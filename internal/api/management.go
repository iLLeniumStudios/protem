package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

const lastModifiedByAnnotation = "protem.dev/last-modified-by"

var reservedLabelPrefixes = []string{
	"protem.dev/",
	"argocd.argoproj.io/",
	"kustomize.toolkit.fluxcd.io/",
	"helm.toolkit.fluxcd.io/",
	"app.kubernetes.io/",
}

func isReservedLabel(key string) bool {
	for _, prefix := range reservedLabelPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func decodeStrict(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func callerName(user UserInfo) string {
	if user.Email != "" {
		return user.Email
	}
	return user.Username
}

func stampModifiedBy(obj metav1.Object, who string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[lastModifiedByAnnotation] = who
	obj.SetAnnotations(annotations)
}

func (s *Server) recordEvent(obj client.Object, reason, message string) {
	if s.Recorder != nil {
		s.Recorder.Event(obj, corev1.EventTypeNormal, reason, message)
	}
}

func (s *Server) writeKubeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case apierrors.IsAlreadyExists(err), apierrors.IsConflict(err):
		status = http.StatusConflict
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		status = http.StatusUnprocessableEntity
	case apierrors.IsNotFound(err):
		status = http.StatusNotFound
	}
	s.writeJSON(w, status, map[string]string{errorKey: err.Error()})
}

func (s *Server) authorizeWrite(ctx context.Context, w http.ResponseWriter, user UserInfo, verb, resource, namespace, name string) bool {
	allowed, err := s.checkSAR(ctx, user, verb, resource, namespace, name)
	if err != nil || !allowed {
		s.writeJSON(w, http.StatusForbidden, map[string]string{errorKey: fmt.Sprintf("Forbidden: user %q may not %s %s", callerName(user), verb, resource)})
		return false
	}
	return true
}

func (s *Server) refuseManaged(w http.ResponseWriter, kind string, obj client.Object) bool {
	owner := managedBy(obj)
	if owner == "" {
		return false
	}
	s.writeJSON(w, http.StatusConflict, map[string]string{
		errorKey: fmt.Sprintf("%s %s is managed by %s; change it there instead", kind, client.ObjectKeyFromObject(obj), owner),
	})
	return true
}

func (s *Server) createProfile(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx, ns := r.Context(), r.PathValue("ns")
	var payload ProfilePayload
	if err := decodeStrict(r, &payload); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: "Invalid request body: " + err.Error()})
		return
	}
	if payload.Name == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: "name is required"})
		return
	}
	if !s.authorizeWrite(ctx, w, user, "create", "accessprofiles", ns, "") {
		return
	}
	who := callerName(user)
	p := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: payload.Name, Annotations: map[string]string{lastModifiedByAnnotation: who}},
		Spec:       payload.Spec,
	}
	if err := s.Client.Create(ctx, p); err != nil {
		s.writeKubeError(w, err)
		return
	}
	s.recordEvent(p, "ProfileCreated", "AccessProfile created by "+who)
	apiLog.Info("AccessProfile created", "namespace", ns, "name", p.Name, "user", who)
	s.writeProfileDetail(ctx, w, http.StatusCreated, p, user)
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx, ns, name := r.Context(), r.PathValue("ns"), r.PathValue("name")
	var payload ProfilePayload
	if err := decodeStrict(r, &payload); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: "Invalid request body: " + err.Error()})
		return
	}
	if payload.ResourceVersion == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: "resourceVersion is required"})
		return
	}
	if !s.authorizeWrite(ctx, w, user, "update", "accessprofiles", ns, name) {
		return
	}
	var p protemv1alpha1.AccessProfile
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &p); err != nil {
		s.writeKubeError(w, err)
		return
	}
	if s.refuseManaged(w, "AccessProfile", &p) {
		return
	}
	// The copy just checked must be the one the caller edited: with a newer version from the caller,
	// the write would carry this copy's labels and skip the ownership check on the current ones.
	if payload.ResourceVersion != p.ResourceVersion {
		s.writeJSON(w, http.StatusConflict, map[string]string{errorKey: "AccessProfile changed since it was loaded; reload and try again"})
		return
	}
	who := callerName(user)
	p.Spec = payload.Spec
	stampModifiedBy(&p, who)
	if err := s.Client.Update(ctx, &p); err != nil {
		s.writeKubeError(w, err)
		return
	}
	s.recordEvent(&p, "ProfileUpdated", "AccessProfile updated by "+who)
	apiLog.Info("AccessProfile updated", "namespace", ns, "name", name, "user", who)
	s.writeProfileDetail(ctx, w, http.StatusOK, &p, user)
}

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx, ns, name := r.Context(), r.PathValue("ns"), r.PathValue("name")
	if !s.authorizeWrite(ctx, w, user, "delete", "accessprofiles", ns, name) {
		return
	}
	var p protemv1alpha1.AccessProfile
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &p); err != nil {
		s.writeKubeError(w, err)
		return
	}
	if s.refuseManaged(w, "AccessProfile", &p) {
		return
	}
	checked := client.Preconditions{UID: &p.UID, ResourceVersion: &p.ResourceVersion}
	if err := s.Client.Delete(ctx, &p, checked); err != nil {
		s.writeKubeError(w, err)
		return
	}
	who := callerName(user)
	s.recordEvent(&p, "ProfileDeleted", "AccessProfile deleted by "+who)
	apiLog.Info("AccessProfile deleted", "namespace", ns, "name", name, "user", who)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) patchCluster(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx, name := r.Context(), r.PathValue("name")
	var payload ClusterSettingsPayload
	if err := decodeStrict(r, &payload); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: "Invalid request body: " + err.Error()})
		return
	}
	if payload.Subject != nil && name == s.HubClusterName {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{
			errorKey: "the hub cluster binds people with the operator's subject template (--subject-template); it cannot be set here",
		})
		return
	}
	if sub := payload.Subject; sub != nil && *sub != "" && !strings.Contains(*sub, "{{.Email}}") && !strings.Contains(*sub, "{{.Username}}") {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{
			errorKey: "subject must contain {{.Email}} or {{.Username}}; a constant subject would map every user to one RBAC identity",
		})
		return
	}
	if payload.Labels != nil {
		for key := range *payload.Labels {
			if isReservedLabel(key) {
				s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: fmt.Sprintf("label %q uses a reserved prefix and cannot be set here", key)})
				return
			}
		}
	}
	if !s.authorizeWrite(ctx, w, user, "update", "targetclusters", "", name) {
		return
	}
	var c protemv1alpha1.TargetCluster
	if err := s.Client.Get(ctx, client.ObjectKey{Name: name}, &c); err != nil {
		s.writeKubeError(w, err)
		return
	}
	if s.refuseManaged(w, "TargetCluster", &c) {
		return
	}
	if payload.Paused != nil {
		c.Spec.Paused = *payload.Paused
	}
	if payload.Subject != nil {
		c.Spec.Subject = *payload.Subject
	}
	if payload.Labels != nil {
		labels := map[string]string{}
		for k, v := range c.Labels {
			if isReservedLabel(k) {
				labels[k] = v
			}
		}
		for k, v := range *payload.Labels {
			labels[k] = v
		}
		c.Labels = labels
	}
	who := callerName(user)
	stampModifiedBy(&c, who)
	if err := s.Client.Update(ctx, &c); err != nil {
		s.writeKubeError(w, err)
		return
	}
	s.recordEvent(&c, "ClusterSettingsChanged", "TargetCluster settings changed by "+who)
	apiLog.Info("TargetCluster settings changed", "name", name, "user", who,
		"paused", payload.Paused != nil, "subject", payload.Subject != nil, "labels", payload.Labels != nil)
	active, err := s.activeGrantCounts(ctx)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, s.clusterView(ctx, &c, active[c.Name], user))
}
