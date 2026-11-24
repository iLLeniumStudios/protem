package fleet

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/approval"
)

// IsClusterReady checks if a TargetCluster is active and not paused.
func IsClusterReady(c *protemv1alpha1.TargetCluster) bool {
	if c.Spec.Paused {
		return false
	}
	// Fail closed if conditions are empty, unless explicitly marked Ready=True
	for _, cond := range c.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// MatchedClusterRequirement captures the approval requirement for one resolved cluster.
type MatchedClusterRequirement struct {
	ClusterName       string
	RuleName          string
	Mode              protemv1alpha1.ApprovalMode
	MaxDuration       time.Duration
	ApproverGroups    []string
	ApproverUsers     []string
	RequiredApprovals int
	AllowSelfApproval bool
}

// FleetEvaluation contains the complete evaluated requirements per resolved cluster.
type FleetEvaluation struct {
	ResolvedClusters   []string
	ResolvedNamespaces []string
	ResolvedDuration   metav1.Duration
	ClusterRules       []MatchedClusterRequirement
	OverallMode        protemv1alpha1.ApprovalMode // Auto only if all clusters match Auto; otherwise Manual
}

// ErrNotReady marks a resolution that failed only because a cluster the profile targets is not
// Ready, which can pass: a hub that just started, an agent that is reconnecting.
var ErrNotReady = errors.New("cluster is not ready")

// EligibleClusters returns the sorted names of the Ready candidate clusters the profile targets.
func EligibleClusters(profile *protemv1alpha1.AccessProfile, candidateClusters []protemv1alpha1.TargetCluster) ([]string, error) {
	ready, _, err := targetedClusters(profile, candidateClusters)
	return ready, err
}

// targetedClusters splits the candidates the profile targets into Ready ones and the rest, sorted.
func targetedClusters(profile *protemv1alpha1.AccessProfile, candidateClusters []protemv1alpha1.TargetCluster) (ready, waiting []string, err error) {
	var match func(*protemv1alpha1.TargetCluster) bool
	switch {
	case profile.Spec.Clusters.Selector != nil:
		selector, err := metav1.LabelSelectorAsSelector(profile.Spec.Clusters.Selector)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid clusterSelector on profile %s: %w", profile.Name, err)
		}
		match = func(c *protemv1alpha1.TargetCluster) bool { return selector.Matches(labels.Set(c.Labels)) }
	case len(profile.Spec.Clusters.Names) > 0:
		match = func(c *protemv1alpha1.TargetCluster) bool {
			return slices.Contains(profile.Spec.Clusters.Names, c.Name)
		}
	default:
		return nil, nil, fmt.Errorf("profile %s has neither clusters.selector nor clusters.names configured", profile.Name)
	}
	for i := range candidateClusters {
		c := &candidateClusters[i]
		switch {
		case !match(c):
		case IsClusterReady(c):
			ready = append(ready, c.Name)
		default:
			waiting = append(waiting, c.Name)
		}
	}
	sort.Strings(ready)
	sort.Strings(waiting)
	return ready, waiting, nil
}

// resolveClusters returns the Ready clusters the profile targets, or the ones of them the request
// names. An error wraps ErrNotReady when the only obstacle is a targeted cluster that is not Ready.
func resolveClusters(
	profile *protemv1alpha1.AccessProfile,
	request *protemv1alpha1.AccessRequest,
	candidateClusters []protemv1alpha1.TargetCluster,
) ([]string, error) {
	eligible, waiting, err := targetedClusters(profile, candidateClusters)
	if err != nil {
		return nil, err
	}
	resolved := eligible
	if request != nil && len(request.Spec.Clusters) > 0 {
		resolved = nil
		for _, requested := range request.Spec.Clusters {
			if slices.Contains(waiting, requested) {
				return nil, fmt.Errorf("requested cluster %q is not ready: %w", requested, ErrNotReady)
			}
			if !slices.Contains(eligible, requested) {
				return nil, fmt.Errorf("requested cluster %q is not permitted by profile %q", requested, profile.Name)
			}
			resolved = append(resolved, requested)
		}
		sort.Strings(resolved)
	}
	if len(resolved) == 0 && len(waiting) > 0 {
		return nil, fmt.Errorf("no cluster of profile %q is ready (%s): %w", profile.Name, strings.Join(waiting, ", "), ErrNotReady)
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("no ready clusters matched for profile %q", profile.Name)
	}
	return resolved, nil
}

// ResolveAndEvaluate resolves target clusters, namespaces, and duration,
// and determines the per-cluster approval requirements.
func ResolveAndEvaluate(
	profile *protemv1alpha1.AccessProfile,
	request *protemv1alpha1.AccessRequest,
	candidateClusters []protemv1alpha1.TargetCluster,
) (*FleetEvaluation, error) {
	if profile == nil {
		return nil, fmt.Errorf("access profile is required")
	}

	// 1-3. Ready clusters the profile targets, narrowed to the ones the request names
	resolvedClusters, err := resolveClusters(profile, request, candidateClusters)
	if err != nil {
		return nil, err
	}

	// 4. Resolve namespaces subset
	var resolvedNamespaces []string
	if profile.Spec.Scope == protemv1alpha1.ProfileScopeNamespaced {
		profileNsSet := make(map[string]bool)
		for _, ns := range profile.Spec.Namespaces {
			profileNsSet[ns] = true
		}

		if request != nil && len(request.Spec.Namespaces) > 0 {
			for _, reqNs := range request.Spec.Namespaces {
				if !profileNsSet[reqNs] {
					return nil, fmt.Errorf("requested namespace %q is not permitted by profile %q", reqNs, profile.Name)
				}
				resolvedNamespaces = append(resolvedNamespaces, reqNs)
			}
		} else {
			resolvedNamespaces = append(resolvedNamespaces, profile.Spec.Namespaces...)
		}
		sort.Strings(resolvedNamespaces)
	}

	// 5. Compute base duration: min(request, profile max)
	effectiveDuration := profile.Spec.Duration.Default.Duration
	if request != nil && request.Spec.Duration != nil && request.Spec.Duration.Duration > 0 {
		effectiveDuration = request.Spec.Duration.Duration
	}
	if profile.Spec.Duration.Max.Duration > 0 && effectiveDuration > profile.Spec.Duration.Max.Duration {
		effectiveDuration = profile.Spec.Duration.Max.Duration
	}

	// 6. Match approval rules per resolved cluster (first matching rule wins)
	clusterLookup := make(map[string]protemv1alpha1.TargetCluster)
	for _, c := range candidateClusters {
		clusterLookup[c.Name] = c
	}

	var clusterReqs []MatchedClusterRequirement
	overallMode := protemv1alpha1.ApprovalModeAuto

	for _, clusterName := range resolvedClusters {
		cluster := clusterLookup[clusterName]
		matched := false

		for _, rule := range profile.Spec.Approval.Rules {
			if ruleMatches(&rule, &cluster, request) {
				matched = true
				req := MatchedClusterRequirement{
					ClusterName:       clusterName,
					RuleName:          rule.Name,
					Mode:              rule.Mode,
					RequiredApprovals: rule.RequiredApprovals,
					AllowSelfApproval: rule.AllowSelfApproval,
				}
				if req.RequiredApprovals <= 0 {
					req.RequiredApprovals = 1
				}
				if rule.Approvers != nil {
					req.ApproverGroups = rule.Approvers.Groups
					req.ApproverUsers = rule.Approvers.Users
				}
				if rule.MaxDuration != nil && rule.MaxDuration.Duration > 0 {
					req.MaxDuration = rule.MaxDuration.Duration
					if req.MaxDuration < effectiveDuration {
						effectiveDuration = req.MaxDuration
					}
				}
				if rule.Mode == protemv1alpha1.ApprovalModeManual {
					overallMode = protemv1alpha1.ApprovalModeManual
				}
				clusterReqs = append(clusterReqs, req)
				break
			}
		}

		if !matched {
			return nil, fmt.Errorf("cluster %q matched no approval rules in profile %q", clusterName, profile.Name)
		}
	}

	return &FleetEvaluation{
		ResolvedClusters:   resolvedClusters,
		ResolvedNamespaces: resolvedNamespaces,
		ResolvedDuration:   metav1.Duration{Duration: effectiveDuration},
		ClusterRules:       clusterReqs,
		OverallMode:        overallMode,
	}, nil
}

func ruleMatches(rule *protemv1alpha1.ApprovalRule, cluster *protemv1alpha1.TargetCluster, request *protemv1alpha1.AccessRequest) bool {
	if rule.When == nil {
		return true
	}

	// 1. ClusterSelector
	if rule.When.ClusterSelector != nil {
		sel, err := metav1.LabelSelectorAsSelector(rule.When.ClusterSelector)
		if err != nil || !sel.Matches(labels.Set(cluster.Labels)) {
			return false
		}
	}

	// 2. RequesterGroups / RequesterUsers
	if len(rule.When.RequesterGroups) > 0 || len(rule.When.RequesterUsers) > 0 {
		return request != nil && approval.Named(approval.Person(request.Spec.Requester), rule.When.RequesterGroups, rule.When.RequesterUsers)
	}

	return true
}
