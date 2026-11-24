package fleet

import (
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

const (
	envLabel        = "env"
	paymentsNs      = "payments"
	prodUsEast1     = "prod-us-east-1"
	paymentsProfile = "payments-profile"
	pciDssTier      = "pci-dss"
	productionEnv   = "production"
	tierLabel       = "tier"
	regionLabel     = "region"
)

func makeCluster(name string, ready bool, labels map[string]string) protemv1alpha1.TargetCluster {
	condStatus := metav1.ConditionFalse
	if ready {
		condStatus = metav1.ConditionTrue
	}
	return protemv1alpha1.TargetCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: protemv1alpha1.TargetClusterSpec{
			DisplayName: name,
		},
		Status: protemv1alpha1.TargetClusterStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: condStatus,
				},
			},
		},
	}
}

// TC-U-01: Unit test verifying ResolveAndEvaluate correctly evaluates complex label selectors
func TestTC_U_01_ResolveTargetClusters_LabelSelector(t *testing.T) {
	clusters := []protemv1alpha1.TargetCluster{
		makeCluster(prodUsEast1, true, map[string]string{envLabel: productionEnv, tierLabel: pciDssTier, regionLabel: "us-east"}),
		makeCluster("prod-eu-west-1", true, map[string]string{envLabel: productionEnv, tierLabel: pciDssTier, regionLabel: "eu-west"}),
		makeCluster("prod-ap-south-1", true, map[string]string{envLabel: productionEnv, tierLabel: "standard", regionLabel: "ap-south"}),
		makeCluster("staging-us-east-1", true, map[string]string{envLabel: "staging", tierLabel: "standard", regionLabel: "us-east"}),
		makeCluster("prod-disconnected", false, map[string]string{envLabel: productionEnv, tierLabel: pciDssTier}), // Not ready
	}

	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "pci-prod-triage"},
		Spec: protemv1alpha1.AccessProfileSpec{
			DisplayName: "PCI Production Triage",
			Clusters: protemv1alpha1.ClusterTargeting{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						envLabel:  productionEnv,
						tierLabel: pciDssTier,
					},
				},
			},
			Scope:      protemv1alpha1.ProfileScopeNamespaced,
			Namespaces: []string{paymentsNs},
			Duration: protemv1alpha1.ProfileDurationConfig{
				Default: metav1.Duration{Duration: 1 * time.Hour},
				Max:     metav1.Duration{Duration: 4 * time.Hour},
			},
			Approval: protemv1alpha1.ProfileApprovalConfig{
				Rules: []protemv1alpha1.ApprovalRule{
					{
						Name: "default-rule",
						Mode: protemv1alpha1.ApprovalModeAuto,
					},
				},
			},
		},
	}

	req := &protemv1alpha1.AccessRequest{
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: "pci-prod-triage",
		},
	}

	eval, err := ResolveAndEvaluate(profile, req, clusters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"prod-eu-west-1", prodUsEast1}
	if !reflect.DeepEqual(eval.ResolvedClusters, expected) {
		t.Errorf("TC-U-01 Failed: expected %v, got %v", expected, eval.ResolvedClusters)
	}
}

// TC-U-02: Unit test verifying ResolveAndEvaluate rejects requests targeting clusters omitted by profile
func TestTC_U_02_ResolveTargetClusters_WhitelistRejection(t *testing.T) {
	clusters := []protemv1alpha1.TargetCluster{
		makeCluster(prodUsEast1, true, map[string]string{envLabel: productionEnv}),
		makeCluster("prod-eu-west-1", true, map[string]string{envLabel: productionEnv}),
	}

	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "us-only-triage"},
		Spec: protemv1alpha1.AccessProfileSpec{
			DisplayName: "US Only Triage",
			Clusters: protemv1alpha1.ClusterTargeting{
				Names: []string{prodUsEast1}, // Explicit whitelist
			},
			Scope:      protemv1alpha1.ProfileScopeNamespaced,
			Namespaces: []string{paymentsNs},
			Duration: protemv1alpha1.ProfileDurationConfig{
				Default: metav1.Duration{Duration: 1 * time.Hour},
				Max:     metav1.Duration{Duration: 4 * time.Hour},
			},
			Approval: protemv1alpha1.ProfileApprovalConfig{
				Rules: []protemv1alpha1.ApprovalRule{
					{
						Name: "default-rule",
						Mode: protemv1alpha1.ApprovalModeAuto,
					},
				},
			},
		},
	}

	// User requests a cluster outside the whitelist
	req := &protemv1alpha1.AccessRequest{
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: "us-only-triage",
			Clusters:   []string{"prod-eu-west-1"},
		},
	}

	_, err := ResolveAndEvaluate(profile, req, clusters)
	if err == nil {
		t.Fatalf("TC-U-02 Failed: expected error when requesting cluster outside whitelist, got nil")
	}

	expectedErrMsg := `requested cluster "prod-eu-west-1" is not permitted by profile "us-only-triage"`
	if err.Error() != expectedErrMsg {
		t.Errorf("unexpected error message: got %q, expected %q", err.Error(), expectedErrMsg)
	}
}

// TC-U-03: Namespaces subset evaluation
func TestTC_U_03_NamespacesSubset(t *testing.T) {
	clusters := []protemv1alpha1.TargetCluster{
		makeCluster(prodUsEast1, true, map[string]string{envLabel: productionEnv}),
	}

	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: paymentsProfile},
		Spec: protemv1alpha1.AccessProfileSpec{
			DisplayName: "Payments Profile",
			Clusters:    protemv1alpha1.ClusterTargeting{Names: []string{prodUsEast1}},
			Scope:       protemv1alpha1.ProfileScopeNamespaced,
			Namespaces:  []string{paymentsNs, "payments-db"},
			Duration: protemv1alpha1.ProfileDurationConfig{
				Default: metav1.Duration{Duration: 1 * time.Hour},
				Max:     metav1.Duration{Duration: 4 * time.Hour},
			},
			Approval: protemv1alpha1.ProfileApprovalConfig{
				Rules: []protemv1alpha1.ApprovalRule{{Name: "rule1", Mode: protemv1alpha1.ApprovalModeAuto}},
			},
		},
	}

	// Case 1: Valid subset
	reqValid := &protemv1alpha1.AccessRequest{
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: paymentsProfile,
			Namespaces: []string{paymentsNs},
		},
	}
	eval, err := ResolveAndEvaluate(profile, reqValid, clusters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(eval.ResolvedNamespaces, []string{paymentsNs}) {
		t.Errorf("expected [payments], got %v", eval.ResolvedNamespaces)
	}

	// Case 2: Disallowed namespace
	reqInvalid := &protemv1alpha1.AccessRequest{
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: paymentsProfile,
			Namespaces: []string{"kube-system"},
		},
	}
	_, err = ResolveAndEvaluate(profile, reqInvalid, clusters)
	if err == nil {
		t.Fatalf("expected error for disallowed namespace kube-system, got nil")
	}
}

// TC-U-04: Approval rules per cluster (strictest governs, rule maxDuration applied)
func TestTC_U_04_ApprovalRulesPerCluster(t *testing.T) {
	clusters := []protemv1alpha1.TargetCluster{
		makeCluster("dev-cluster", true, map[string]string{envLabel: "dev"}),
		makeCluster("prod-cluster", true, map[string]string{envLabel: "prod"}),
	}

	maxDuration2h := metav1.Duration{Duration: 2 * time.Hour}
	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-profile"},
		Spec: protemv1alpha1.AccessProfileSpec{
			DisplayName: "Fleet Profile",
			Clusters: protemv1alpha1.ClusterTargeting{
				Names: []string{"dev-cluster", "prod-cluster"},
			},
			Scope: protemv1alpha1.ProfileScopeCluster,
			Role:  protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "view"},
			Duration: protemv1alpha1.ProfileDurationConfig{
				Default: metav1.Duration{Duration: 4 * time.Hour},
				Max:     metav1.Duration{Duration: 8 * time.Hour},
			},
			Approval: protemv1alpha1.ProfileApprovalConfig{
				Rules: []protemv1alpha1.ApprovalRule{
					{
						Name: "dev-rule",
						When: &protemv1alpha1.ApprovalRuleWhen{
							ClusterSelector: &metav1.LabelSelector{MatchLabels: map[string]string{envLabel: "dev"}},
						},
						Mode: protemv1alpha1.ApprovalModeAuto,
					},
					{
						Name: "prod-rule",
						When: &protemv1alpha1.ApprovalRuleWhen{
							ClusterSelector: &metav1.LabelSelector{MatchLabels: map[string]string{envLabel: "prod"}},
						},
						Mode:        protemv1alpha1.ApprovalModeManual,
						MaxDuration: &maxDuration2h, // Tighter duration on prod
						Approvers: &protemv1alpha1.ApprovalApprovers{
							Groups: []string{"sre-leads"},
						},
						RequiredApprovals: 1,
					},
				},
			},
		},
	}

	req := &protemv1alpha1.AccessRequest{
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: "fleet-profile",
			Duration:   &metav1.Duration{Duration: 6 * time.Hour},
		},
	}

	eval, err := ResolveAndEvaluate(profile, req, clusters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mixed request has prod cluster -> overall mode must be Manual
	if eval.OverallMode != protemv1alpha1.ApprovalModeManual {
		t.Errorf("expected overall mode Manual, got %v", eval.OverallMode)
	}

	// Duration must be capped by prod-rule maxDuration (2h)
	if eval.ResolvedDuration.Duration != 2*time.Hour {
		t.Errorf("expected duration capped to 2h, got %v", eval.ResolvedDuration.Duration)
	}

	if len(eval.ClusterRules) != 2 {
		t.Fatalf("expected 2 cluster rules, got %d", len(eval.ClusterRules))
	}
}

func TestRuleMatchesRequester(t *testing.T) {
	cluster := makeCluster("c", true, nil)
	requester := func(username, email string, groups ...string) *protemv1alpha1.AccessRequest {
		return &protemv1alpha1.AccessRequest{Spec: protemv1alpha1.AccessRequestSpec{
			Requester: protemv1alpha1.RequesterInfo{Username: username, Email: email, Groups: groups},
		}}
	}
	alice := requester("alice", "alice@company.com", "devs")
	bob := requester("bob", "bob@company.com", "ops")
	carol := requester("carol", "carol@company.com", "sre")
	nobody := requester("", "")
	cases := []struct {
		name string
		when protemv1alpha1.ApprovalRuleWhen
		req  *protemv1alpha1.AccessRequest
		want bool
	}{
		{"no requester condition matches anyone", protemv1alpha1.ApprovalRuleWhen{}, carol, true},
		{"user by username", protemv1alpha1.ApprovalRuleWhen{RequesterUsers: []string{"bob"}}, bob, true},
		{"user by email", protemv1alpha1.ApprovalRuleWhen{RequesterUsers: []string{"bob@company.com"}}, bob, true},
		{"other user", protemv1alpha1.ApprovalRuleWhen{RequesterUsers: []string{"bob"}}, alice, false},
		{"empty user entry matches nobody", protemv1alpha1.ApprovalRuleWhen{RequesterUsers: []string{""}}, nobody, false},
		{"group beside users", protemv1alpha1.ApprovalRuleWhen{RequesterGroups: []string{"devs"}, RequesterUsers: []string{"bob"}}, alice, true},
		{"user beside groups", protemv1alpha1.ApprovalRuleWhen{RequesterGroups: []string{"devs"}, RequesterUsers: []string{"bob"}}, bob, true},
		{"neither group nor user", protemv1alpha1.ApprovalRuleWhen{RequesterGroups: []string{"devs"}, RequesterUsers: []string{"bob"}}, carol, false},
		{"users need a request", protemv1alpha1.ApprovalRuleWhen{RequesterUsers: []string{"bob"}}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := protemv1alpha1.ApprovalRule{Name: "r", Mode: protemv1alpha1.ApprovalModeAuto, When: &tc.when}
			if got := ruleMatches(&rule, &cluster, tc.req); got != tc.want {
				t.Fatalf("ruleMatches = %v, want %v", got, tc.want)
			}
		})
	}
}
