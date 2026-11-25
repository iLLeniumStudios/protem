package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

const (
	argoInstance = "argocd.argoproj.io/instance"
	enabled      = "enabled"
)

// laggingServer reads AccessProfiles as they were before Argo CD adopted them, the way a replica
// with a slow informer cache would, while writes reach the real store.
func laggingServer(t *testing.T) (*Server, client.Client, *protemv1alpha1.AccessProfile) {
	t.Helper()
	stored := testProfile("team-a", "p1", nil, nil)
	c := fake.NewClientBuilder().WithScheme(setupTestScheme(t)).WithObjects(stored).Build()
	var stale protemv1alpha1.AccessProfile
	key := client.ObjectKeyFromObject(stored)
	if err := c.Get(context.Background(), key, &stale); err != nil {
		t.Fatal(err)
	}
	live := stale.DeepCopy()
	live.Labels = map[string]string{argoInstance: "profiles"}
	if err := c.Update(context.Background(), live); err != nil {
		t.Fatal(err)
	}

	lagging := interceptor.NewClient(c, interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if sar, ok := obj.(*authorizationv1.SubjectAccessReview); ok {
				sar.Status.Allowed = true
				return nil
			}
			return c.Create(ctx, obj, opts...)
		},
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if p, ok := obj.(*protemv1alpha1.AccessProfile); ok {
				stale.DeepCopyInto(p)
				return nil
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	s, err := NewServer(lagging, ServerOptions{Addr: testAddr, DevAuth: true, UIManagement: enabled})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s, c, live
}

func TestAStaleReadCannotOverwriteOrDeleteAProfileGitOpsHasAdopted(t *testing.T) {
	for _, tc := range []struct{ name, method string }{{"update", http.MethodPut}, {"delete", http.MethodDelete}} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, live := laggingServer(t)
			w := call(s, tc.method, "/api/namespaces/team-a/profiles/p1", aliceEmail, profileBody("", live.ResourceVersion, "Hijacked"))
			if w.Code != http.StatusConflict {
				t.Errorf("%s through a stale read = %d, want 409: %s", tc.name, w.Code, w.Body.String())
			}
			var after protemv1alpha1.AccessProfile
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(live), &after); err != nil {
				t.Fatalf("profile is gone: %v", err)
			}
			if after.Labels[argoInstance] != "profiles" || after.Spec.DisplayName != live.Spec.DisplayName {
				t.Errorf("stored profile was changed: labels %v, displayName %q", after.Labels, after.Spec.DisplayName)
			}
		})
	}
}

func TestDeletingAProfileRecordsWhoDidIt(t *testing.T) {
	recorder := record.NewFakeRecorder(4)
	s, _ := newMgmtServer(t, allowAll, ServerOptions{UIManagement: enabled, Recorder: recorder}, testProfile("team-a", "p1", nil, nil))
	if w := call(s, http.MethodDelete, "/api/namespaces/team-a/profiles/p1", aliceEmail, ""); w.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d: %s", w.Code, w.Body.String())
	}
	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, "ProfileDeleted") || !strings.Contains(event, aliceEmail) {
			t.Errorf("event = %q, want ProfileDeleted naming %s", event, aliceEmail)
		}
	default:
		t.Error("no event recorded for the deletion")
	}
}

func TestAnEmptySubjectResetsAClusterToTheHubDefault(t *testing.T) {
	cluster := testCluster(nil, nil)
	cluster.Spec.Subject = "oidc:{{.Email}}"
	s, c := newMgmtServer(t, allowAll, ServerOptions{UIManagement: enabled}, cluster)
	if w := call(s, http.MethodPatch, "/api/clusters/c1", aliceEmail, `{"subject":""}`); w.Code != http.StatusOK {
		t.Fatalf("PATCH = %d: %s", w.Code, w.Body.String())
	}
	var after protemv1alpha1.TargetCluster
	if err := c.Get(context.Background(), client.ObjectKey{Name: "c1"}, &after); err != nil || after.Spec.Subject != "" {
		t.Errorf("subject = %q (err %v), want it cleared", after.Spec.Subject, err)
	}
}

func TestTokenClaimsBecomeAUserThatCannotCollideOrClaimSystemGroups(t *testing.T) {
	for _, tc := range []struct {
		name    string
		claims  tokenClaims
		want    UserInfo
		wantErr bool
	}{
		{
			name:   "email and username",
			claims: tokenClaims{Email: "a@x.io", PreferredUsername: "a", Sub: "u1", Groups: []string{"devs"}},
			want:   UserInfo{Email: "a@x.io", Username: "a", Groups: []string{"devs"}},
		},
		{
			name:   "only a subject: the email falls back to it, so two such users never share an RBAC subject",
			claims: tokenClaims{Sub: "u1"},
			want:   UserInfo{Email: "u1", Username: "u1", Groups: []string{}},
		},
		{
			name:   "an email and a subject but no username, as Google issues: the username is the email, not an opaque id",
			claims: tokenClaims{Email: "a@x.io", Sub: "110540357520890786310"},
			want:   UserInfo{Email: "a@x.io", Username: "a@x.io", Groups: []string{}},
		},
		{
			name:   "system: groups from an identity provider are dropped",
			claims: tokenClaims{Email: "a@x.io", Groups: []string{"system:masters", "devs", "system:authenticated"}},
			want:   UserInfo{Email: "a@x.io", Username: "a@x.io", Groups: []string{"devs"}},
		},
		{name: "a system: identity is refused", claims: tokenClaims{Email: "system:admin"}, wantErr: true},
		{name: "a system: username is refused", claims: tokenClaims{Email: "a@x.io", PreferredUsername: "system:kube-controller-manager"}, wantErr: true},
		{name: "no identity at all is refused", claims: tokenClaims{Groups: []string{"devs"}}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.claims.user()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("user() = %+v, want an error", got)
				}
				return
			}
			if err != nil || got.Email != tc.want.Email || got.Username != tc.want.Username || strings.Join(got.Groups, ",") != strings.Join(tc.want.Groups, ",") {
				t.Fatalf("user() = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
