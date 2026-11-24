package controller

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/audit"
)

// auditMark is how many events of one request (told apart from a later one of the same name by UID) were written.
type auditMark struct {
	uid    types.UID
	events int
}

// AuditReconciler writes each audit event of an AccessRequest to Out once it happens, and deletes finished
// requests Retention after their last event.
type AuditReconciler struct {
	client.Client
	// Out receives one JSON object per line.
	Out io.Writer
	// Retention is how long a finished request is kept after its last event; 0 keeps it forever.
	Retention time.Duration
	// Now defaults to time.Now.
	Now func() time.Time

	// written remembers, per request, how many events this process has written. The cached read of
	// status.auditedEvents can lag the write of it, and would otherwise repeat a line.
	written sync.Map
}

type auditLogLine struct {
	Kind string `json:"kind"`
	audit.Event
}

// +kubebuilder:rbac:groups=protem.dev,resources=accessrequests,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=protem.dev,resources=accessrequests/status,verbs=get;patch
// +kubebuilder:rbac:groups=protem.dev,resources=accessgrants,verbs=get;list;watch

func (r *AuditReconciler) Reconcile(ctx context.Context, key ctrl.Request) (ctrl.Result, error) {
	var req protemv1alpha1.AccessRequest
	if err := r.Get(ctx, key.NamespacedName, &req); err != nil {
		if apierrors.IsNotFound(err) {
			r.written.Delete(key.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var grant *protemv1alpha1.AccessGrant
	if req.Status.GrantRef != "" {
		grant = &protemv1alpha1.AccessGrant{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: req.Status.GrantRef}, grant); apierrors.IsNotFound(err) {
			grant = nil
		} else if err != nil {
			return ctrl.Result{}, err
		}
	}
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}

	events := audit.Events(&req, grant, now)
	// Writing before recording the cursor makes delivery at-least-once: a failure in between writes the same
	// events, with the same ids, again on the next reconcile.
	logged := int(req.Status.AuditedEvents)
	if m, ok := r.written.Load(key.NamespacedName); ok && m.(auditMark).uid == req.UID {
		logged = max(logged, m.(auditMark).events)
	}
	if len(events) > logged {
		for _, e := range events[logged:] {
			line, err := json.Marshal(auditLogLine{Kind: "ProtemAudit", Event: e})
			if err != nil {
				return ctrl.Result{}, err
			}
			if _, err := r.Out.Write(append(line, '\n')); err != nil {
				return ctrl.Result{}, err
			}
		}
		base := req.DeepCopy()
		req.Status.AuditedEvents = int32(len(events))
		if err := r.Status().Patch(ctx, &req, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, err
		}
		r.written.Store(key.NamespacedName, auditMark{uid: req.UID, events: len(events)})
	}

	live := grant != nil && grant.Spec.Revocation == nil && now.Before(grant.Spec.ExpiresAt.Time)
	if r.Retention > 0 && finished(&req, live) {
		if wait := events[len(events)-1].Timestamp.Add(r.Retention).Sub(now); wait > 0 {
			return ctrl.Result{RequeueAfter: wait}, nil
		}
		return ctrl.Result{}, client.IgnoreNotFound(r.Delete(ctx, &req, client.PropagationPolicy(metav1.DeletePropagationBackground)))
	}
	if live {
		return ctrl.Result{RequeueAfter: grant.Spec.ExpiresAt.Sub(now) + time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func finished(req *protemv1alpha1.AccessRequest, grantLive bool) bool {
	switch req.Status.Phase {
	case protemv1alpha1.RequestPhaseRejected, protemv1alpha1.RequestPhaseCancelled, protemv1alpha1.RequestPhaseTimedOut:
		return true
	case protemv1alpha1.RequestPhaseApproved:
		return req.Status.GrantRef != "" && !grantLive
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuditReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&protemv1alpha1.AccessRequest{}).
		Owns(&protemv1alpha1.AccessGrant{}).
		Named("audit").
		Complete(r)
}
