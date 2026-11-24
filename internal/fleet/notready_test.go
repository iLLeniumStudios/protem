package fleet

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// A cluster the profile targets but that is not Ready yet (a hub that just started, an agent that
// is reconnecting) is a different answer from a cluster the profile does not allow.
func TestResolveTellsNotReadyFromNotPermitted(t *testing.T) {
	prod := map[string]string{envLabel: productionEnv}
	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "prod-view"},
		Spec: protemv1alpha1.AccessProfileSpec{
			Scope:    protemv1alpha1.ProfileScopeCluster,
			Clusters: protemv1alpha1.ClusterTargeting{Selector: &metav1.LabelSelector{MatchLabels: prod}},
			Approval: protemv1alpha1.ProfileApprovalConfig{Rules: []protemv1alpha1.ApprovalRule{{Name: "auto", Mode: protemv1alpha1.ApprovalModeAuto}}},
		},
	}
	asking := func(clusters ...string) *protemv1alpha1.AccessRequest {
		return &protemv1alpha1.AccessRequest{Spec: protemv1alpha1.AccessRequestSpec{Clusters: clusters}}
	}
	starting := makeCluster("prod-eu", false, prod)
	ready := makeCluster("prod-us", true, prod)
	other := makeCluster("dev-1", true, map[string]string{envLabel: "dev"})

	for _, tc := range []struct {
		name     string
		request  *protemv1alpha1.AccessRequest
		clusters []protemv1alpha1.TargetCluster
		notReady bool
	}{
		{"the requested cluster is targeted but not ready", asking("prod-eu"), []protemv1alpha1.TargetCluster{starting, ready}, true},
		{"no targeted cluster is ready", asking(), []protemv1alpha1.TargetCluster{starting, other}, true},
		{"the requested cluster is not targeted", asking("dev-1"), []protemv1alpha1.TargetCluster{ready, other}, false},
		{"the requested cluster does not exist", asking("nowhere"), []protemv1alpha1.TargetCluster{ready}, false},
		{"nothing is targeted", asking(), []protemv1alpha1.TargetCluster{other}, false},
	} {
		_, err := ResolveAndEvaluate(profile, tc.request, tc.clusters)
		if err == nil {
			t.Errorf("%s: expected an error", tc.name)
			continue
		}
		if got := errors.Is(err, ErrNotReady); got != tc.notReady {
			t.Errorf("%s: errors.Is(err, ErrNotReady) = %v, want %v (%v)", tc.name, got, tc.notReady, err)
		}
	}
}
