package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

const (
	DefaultAgentImage     = "ghcr.io/illeniumstudios/protem-agent:latest"
	DefaultAgentManifests = "https://github.com/iLLeniumStudios/protem//config/agent/spoke?ref=main"
	DefaultJoinTokenTTL   = 15 * time.Minute

	maxClusterNameLength = 40
	joinCommand          = "kubectl apply -k ."
)

func (s *Server) newClusterError(p AddClusterPayload) string {
	switch {
	case len(validation.IsDNS1123Label(p.Name)) > 0 || len(p.Name) > maxClusterNameLength:
		return fmt.Sprintf("name %q is not valid; use at most %d lowercase letters, digits and '-', starting and ending with a letter or digit",
			p.Name, maxClusterNameLength)
	case p.Name == s.HubClusterName:
		return fmt.Sprintf("name %q is the hub's own name; choose another", p.Name)
	case strings.TrimSpace(p.DisplayName) == "":
		return "displayName is required; give the cluster a name people will recognise"
	}
	for key := range p.Labels {
		if isReservedLabel(key) {
			return fmt.Sprintf("label %q uses a reserved prefix and cannot be set here", key)
		}
	}
	return ""
}

func (s *Server) addCluster(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx := r.Context()
	var payload AddClusterPayload
	if err := decodeStrict(r, &payload); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: "Invalid request body: " + err.Error()})
		return
	}
	if msg := s.newClusterError(payload); msg != "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: msg})
		return
	}
	if !s.authorizeWrite(ctx, w, user, "create", "targetclusters", "", "") {
		return
	}
	who := callerName(user)
	c := &protemv1alpha1.TargetCluster{
		ObjectMeta: metav1.ObjectMeta{Name: payload.Name, Labels: payload.Labels, Annotations: map[string]string{lastModifiedByAnnotation: who}},
		Spec:       protemv1alpha1.TargetClusterSpec{DisplayName: payload.DisplayName, Description: payload.Description},
	}
	if err := s.Client.Create(ctx, c); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.writeJSON(w, http.StatusConflict, map[string]string{errorKey: fmt.Sprintf("TargetCluster %q already exists; choose another name", payload.Name)})
			return
		}
		s.writeKubeError(w, err)
		return
	}
	s.recordEvent(c, "ClusterAdded", "TargetCluster added by "+who)
	apiLog.Info("TargetCluster added", "name", c.Name, "user", who)
	s.writeJSON(w, http.StatusCreated, s.clusterView(ctx, c, 0))
}

func (s *Server) getCluster(ctx context.Context, w http.ResponseWriter, name string, c *protemv1alpha1.TargetCluster) bool {
	err := s.Client.Get(ctx, client.ObjectKey{Name: name}, c)
	switch {
	case apierrors.IsNotFound(err):
		s.writeJSON(w, http.StatusNotFound, map[string]string{errorKey: fmt.Sprintf("TargetCluster %q does not exist; reload the list of clusters", name)})
	case err != nil:
		s.writeKubeError(w, err)
	default:
		return true
	}
	return false
}

func (s *Server) joinCluster(w http.ResponseWriter, r *http.Request, user UserInfo) {
	s.issueCredentials(w, r, user, false)
}

func (s *Server) reissueCluster(w http.ResponseWriter, r *http.Request, user UserInfo) {
	s.issueCredentials(w, r, user, true)
}

func (s *Server) issueCredentials(w http.ResponseWriter, r *http.Request, user UserInfo, reissue bool) {
	ctx, name := r.Context(), r.PathValue("name")
	if !s.authorizeWrite(ctx, w, user, "update", "targetclusters", "", name) {
		return
	}
	var c protemv1alpha1.TargetCluster
	if !s.getCluster(ctx, w, name, &c) {
		return
	}
	if c.Name == s.HubClusterName {
		s.writeJSON(w, http.StatusConflict, map[string]string{errorKey: fmt.Sprintf("TargetCluster %q is the hub itself; it needs no agent", name)})
		return
	}
	if s.HubAPIURL == "" {
		s.writeJSON(w, http.StatusConflict, map[string]string{
			errorKey: "agents cannot be connected because the hub's API address is not configured; start the operator with --hub-api-url",
		})
		return
	}
	internalError := func(err error) {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
	}
	if err := fleet.EnsureAgentIdentity(ctx, s.Client, name); err != nil {
		internalError(err)
		return
	}
	if reissue {
		if err := fleet.ReplaceAgentAccount(ctx, s.Client, name); err != nil {
			internalError(err)
			return
		}
	}
	who := callerName(user)
	patch := client.MergeFrom(c.DeepCopy())
	stampModifiedBy(&c, who)
	if err := s.Client.Patch(ctx, &c, patch); err != nil {
		s.writeKubeError(w, err)
		return
	}

	seconds := int64(s.JoinTokenTTL / time.Second)
	token := &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &seconds}}
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: fleet.ClusterNamespace(name), Name: fleet.AgentName}}
	if err := s.Client.SubResource("token").Create(ctx, sa, token); err != nil {
		internalError(fmt.Errorf("request a join token for ServiceAccount %s/%s: %w", sa.Namespace, sa.Name, err))
		return
	}
	kubeconfig, err := s.joinKubeconfig(token.Status.Token)
	if err != nil {
		internalError(err)
		return
	}
	expires := token.Status.ExpirationTimestamp.Time
	if reissue {
		s.recordEvent(&c, "CredentialsReissued", "Agent credentials reissued by "+who)
		apiLog.Info("Agent credentials reissued", "name", name, "user", who, "expiresAt", expires)
	} else {
		s.recordEvent(&c, "JoinTokenIssued", "Join token issued by "+who)
		apiLog.Info("Join token issued", "name", name, "user", who, "expiresAt", expires)
	}
	s.writeJSON(w, http.StatusOK, JoinView{
		Cluster:       name,
		ExpiresAt:     expires,
		Kubeconfig:    string(kubeconfig),
		Kustomization: s.joinKustomization(name),
		Command:       joinCommand,
	})
}

func (s *Server) joinKubeconfig(token string) ([]byte, error) {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["hub"] = &clientcmdapi.Cluster{Server: s.HubAPIURL, CertificateAuthorityData: s.HubCAData}
	cfg.AuthInfos[fleet.AgentName] = &clientcmdapi.AuthInfo{Token: token}
	cfg.Contexts["hub"] = &clientcmdapi.Context{Cluster: "hub", AuthInfo: fleet.AgentName}
	cfg.CurrentContext = "hub"
	return clientcmd.Write(*cfg)
}

func splitImage(image string) (name, tag string) {
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		return image[:i], image[i+1:]
	}
	return image, "latest"
}

// yamlString quotes s only where YAML would otherwise read it as something other than this string, such as
// the tag 1.10 as a number.
func yamlString(s string) string {
	var v any
	if err := yaml.Unmarshal([]byte(s), &v); err == nil && v == s {
		return s
	}
	return strconv.Quote(s)
}

func (s *Server) joinKustomization(cluster string) string {
	name, tag := splitImage(s.AgentImage)
	return fmt.Sprintf(`resources:
- %s
images:
- name: protem-agent
  newName: %s
  newTag: %s
configMapGenerator:
- name: protem-agent
  namespace: protem-system
  literals:
  - %s
secretGenerator:
- name: protem-agent-hub
  namespace: protem-system
  options:
    disableNameSuffixHash: true
  files:
  - kubeconfig=hub.kubeconfig
`, yamlString(s.AgentManifests), yamlString(name), yamlString(tag), yamlString("cluster-name="+cluster))
}

func (s *Server) deleteCluster(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx, name := r.Context(), r.PathValue("name")
	if !s.authorizeWrite(ctx, w, user, "delete", "targetclusters", "", name) {
		return
	}
	var c protemv1alpha1.TargetCluster
	if !s.getCluster(ctx, w, name, &c) {
		return
	}
	if s.refuseManaged(w, "TargetCluster", &c) {
		return
	}
	active, err := s.activeGrantCounts(ctx)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	if n := active[name]; n > 0 {
		s.writeJSON(w, http.StatusConflict, map[string]string{
			errorKey: fmt.Sprintf("%d active grants still target TargetCluster %q; revoke them first", n, name),
		})
		return
	}
	checked := client.Preconditions{UID: &c.UID, ResourceVersion: &c.ResourceVersion}
	if err := s.Client.Delete(ctx, &c, checked); err != nil {
		s.writeKubeError(w, err)
		return
	}
	who := callerName(user)
	s.recordEvent(&c, "ClusterRemoved", "TargetCluster removed by "+who)
	apiLog.Info("TargetCluster removed", "name", name, "user", who)
	w.WriteHeader(http.StatusNoContent)
}
