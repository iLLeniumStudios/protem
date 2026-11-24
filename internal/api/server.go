package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	authorizationv1 "k8s.io/api/authorization/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/approval"
	"github.com/iLLeniumStudios/protem/internal/binding"
	"github.com/iLLeniumStudios/protem/internal/fleet"
	"github.com/iLLeniumStudios/protem/web"
)

const errorKey = "error"

var apiLog = logf.Log.WithName("api-server")

// ServerOptions configures the Protem API server.
type ServerOptions struct {
	Addr            string
	DevAuth         bool
	OIDCIssuerURL   string
	OIDCClientID    string
	SubjectTemplate string
	// UIManagement is "readonly" (the default when empty) or "enabled".
	UIManagement string
	SignOutURL   string
	Recorder     record.EventRecorder
	// HubClusterName is the TargetCluster that stands for the hub itself; it has no agent.
	HubClusterName string
	// HubAPIURL is the hub's Kubernetes API as agents reach it; join tokens are refused without it.
	HubAPIURL string
	// HubCAData is the certificate authority agents trust for HubAPIURL; may be empty.
	HubCAData      []byte
	AgentImage     string
	AgentManifests string
	JoinTokenTTL   time.Duration
}

// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Server provides the REST API for the Protem web UI and CLI.
type Server struct {
	Client          client.Client
	Mux             *http.ServeMux
	Addr            string
	DevAuth         bool
	SubjectTemplate string
	Authenticator   *Authenticator
	Management      bool
	SignOutURL      string
	Recorder        record.EventRecorder
	HubClusterName  string
	HubAPIURL       string
	HubCAData       []byte
	AgentImage      string
	AgentManifests  string
	JoinTokenTTL    time.Duration
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false // e.g. ":8080" binds to all interfaces (0.0.0.0), NOT loopback!
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost"
}

// NewServer initializes the API server routes.
func NewServer(k8sClient client.Client, opts ...ServerOptions) (*Server, error) {
	var opt ServerOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	listenAddr := opt.Addr
	if listenAddr == "" {
		listenAddr = ":8080"
	}

	// C1: Refuse dev-auth unless explicitly bound to loopback
	if opt.DevAuth && !isLoopback(listenAddr) {
		return nil, fmt.Errorf("refusing to enable dev-auth on non-loopback address %q; dev-auth can only bind to 127.0.0.1, localhost, or [::1]", listenAddr)
	}

	var management bool
	switch opt.UIManagement {
	case "", "readonly":
	case "enabled":
		management = true
	default:
		return nil, fmt.Errorf("invalid ui-management mode %q; use readonly or enabled", opt.UIManagement)
	}

	joinTTL := cmp.Or(opt.JoinTokenTTL, DefaultJoinTokenTTL)
	if joinTTL < 10*time.Minute || joinTTL > time.Hour {
		return nil, fmt.Errorf("invalid --join-token-ttl %v; use a value from 10m to 1h", opt.JoinTokenTTL)
	}

	var verifier TokenVerifier
	if opt.OIDCIssuerURL != "" && opt.OIDCClientID != "" {
		provider, err := oidc.NewProvider(context.Background(), opt.OIDCIssuerURL)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OIDC provider %q: %w", opt.OIDCIssuerURL, err)
		}
		verifier = provider.Verifier(&oidc.Config{ClientID: opt.OIDCClientID})
	}

	auth := NewAuthenticator(opt.DevAuth, verifier)

	s := &Server{
		Client:          k8sClient,
		Mux:             http.NewServeMux(),
		Addr:            listenAddr,
		DevAuth:         opt.DevAuth,
		SubjectTemplate: opt.SubjectTemplate,
		Authenticator:   auth,
		Management:      management,
		SignOutURL:      opt.SignOutURL,
		Recorder:        opt.Recorder,
		HubClusterName:  cmp.Or(opt.HubClusterName, "local"),
		HubAPIURL:       opt.HubAPIURL,
		HubCAData:       opt.HubCAData,
		AgentImage:      cmp.Or(opt.AgentImage, DefaultAgentImage),
		AgentManifests:  cmp.Or(opt.AgentManifests, DefaultAgentManifests),
		JoinTokenTTL:    joinTTL,
	}
	s.registerRoutes()
	return s, nil
}

// Start implements manager.Runnable to run alongside controllers in manager.
func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.Addr,
		Handler: s.Handler(),
	}
	apiLog.Info("Starting Protem web UI & REST API server", "addr", s.Addr, "devAuth", s.DevAuth)

	go func() {
		<-ctx.Done()
		apiLog.Info("Shutting down Protem web UI & REST API server")
		_ = srv.Shutdown(context.Background())
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Handler is everything the server serves: the routes behind the cross-site request checks.
func (s *Server) Handler() http.Handler {
	return s.cors(requireJSONWrites(s.Mux))
}

const maxBodyBytes = 1 << 20

// requireJSONWrites refuses state-changing requests that are not JSON. A page on another site can make a
// browser post a form with the user's session cookie, but cannot set this content type without a CORS
// preflight, which the server does not grant.
func requireJSONWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				_ = json.NewEncoder(w).Encode(map[string]string{errorKey: "Content-Type must be application/json"})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")
}

// NeedLeaderElection returns false so the web UI runs on all manager replicas.
func (s *Server) NeedLeaderElection() bool {
	return false
}

func (s *Server) registerRoutes() {
	decide := func(kind protemv1alpha1.ApprovalDecisionType) http.HandlerFunc {
		return s.authed(func(w http.ResponseWriter, r *http.Request, user UserInfo) {
			s.decideRequest(w, r, user, kind)
		})
	}
	s.Mux.HandleFunc("GET /api/me", s.authed(s.handleMe))
	s.Mux.HandleFunc("GET /api/clusters", s.authed(s.handleClusters))
	s.Mux.HandleFunc("GET /api/profiles", s.authed(s.handleProfiles))
	s.Mux.HandleFunc("GET /api/requests", s.authed(s.handleGetRequests))
	s.Mux.HandleFunc("GET /api/namespaces/{ns}/requests", s.authed(s.handleGetRequests))
	s.Mux.HandleFunc("GET /api/namespaces/{ns}/requests/{name}", s.authed(s.handleGetRequest))
	s.Mux.HandleFunc("POST /api/namespaces/{ns}/requests", s.authed(s.handleCreateRequest))
	s.Mux.HandleFunc("POST /api/namespaces/{ns}/requests/{name}/approve", decide(protemv1alpha1.ApprovalDecisionApproved))
	s.Mux.HandleFunc("POST /api/namespaces/{ns}/requests/{name}/reject", decide(protemv1alpha1.ApprovalDecisionRejected))
	s.Mux.HandleFunc("POST /api/namespaces/{ns}/requests/{name}/cancel", decide(protemv1alpha1.ApprovalDecisionCancelled))
	s.Mux.HandleFunc("GET /api/grants", s.authed(s.handleGrants))
	s.Mux.HandleFunc("GET /api/namespaces/{ns}/grants/{name}", s.authed(s.handleGetGrant))
	s.Mux.HandleFunc("POST /api/namespaces/{ns}/grants/{name}/revoke", s.authed(s.revokeGrant))
	s.Mux.HandleFunc("GET /api/stats", s.authed(s.handleStats))
	s.Mux.HandleFunc("GET /api/audit", s.authed(s.handleAudit))
	s.Mux.HandleFunc("GET /api/capabilities", s.authed(s.handleCapabilities))
	s.Mux.HandleFunc("GET /api/namespaces/{ns}/profiles/{name}", s.authed(s.handleProfileDetail))
	s.Mux.HandleFunc("GET /api/namespaces/{ns}/profiles/{name}/permissions", s.authed(s.handleProfilePermissions))
	s.Mux.HandleFunc("GET /api/namespaces/{ns}/grants/{name}/permissions", s.authed(s.handleGrantPermissions))
	if s.Management {
		s.Mux.HandleFunc("POST /api/namespaces/{ns}/profiles", s.authed(s.createProfile))
		s.Mux.HandleFunc("PUT /api/namespaces/{ns}/profiles/{name}", s.authed(s.updateProfile))
		s.Mux.HandleFunc("DELETE /api/namespaces/{ns}/profiles/{name}", s.authed(s.deleteProfile))
		s.Mux.HandleFunc("PATCH /api/clusters/{name}", s.authed(s.patchCluster))
		s.Mux.HandleFunc("POST /api/clusters", s.authed(s.addCluster))
		s.Mux.HandleFunc("POST /api/clusters/{name}/join", s.authed(s.joinCluster))
		s.Mux.HandleFunc("POST /api/clusters/{name}/reissue", s.authed(s.reissueCluster))
		s.Mux.HandleFunc("DELETE /api/clusters/{name}", s.authed(s.deleteCluster))
	}
	s.Mux.HandleFunc("/api/", s.handleAPINotFound)
	s.Mux.Handle("/", web.Handler())
}

func (s *Server) handleAPINotFound(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusNotFound, map[string]string{errorKey: "Endpoint not found"})
}

func (s *Server) authed(h func(http.ResponseWriter, *http.Request, UserInfo)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.Authenticator.Authenticate(r)
		if err != nil {
			s.writeJSON(w, http.StatusUnauthorized, map[string]string{errorKey: err.Error()})
			return
		}
		h(w, r, user)
	}
}

// cors lets a UI dev server on another loopback port call the API; outside dev-auth the UI is same-origin.
func (s *Server) cors(next http.Handler) http.Handler {
	if !s.DevAuth {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); isLoopbackOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Test-User")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		apiLog.Error(err, "failed to write json response")
	}
}

func (s *Server) checkSAR(ctx context.Context, user UserInfo, verb, resource, namespace, name string) (bool, error) {
	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   binding.FormatSubject(s.SubjectTemplate, protemv1alpha1.RequesterInfo{Email: user.Email, Username: user.Username}),
			Groups: user.Groups,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      verb,
				Group:     "protem.dev",
				Resource:  resource,
				Name:      name,
			},
		},
	}
	if err := s.Client.Create(ctx, sar); err != nil {
		return false, err
	}
	return sar.Status.Allowed, nil
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, user UserInfo) {
	s.writeJSON(w, http.StatusOK, MeView{UserInfo: user, DevAuth: s.DevAuth, SignOutURL: s.SignOutURL})
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx := r.Context()
	allowed := func(verb, resource, namespace string) bool {
		ok, _ := s.checkSAR(ctx, user, verb, resource, namespace, "")
		return ok
	}
	caps := CapabilitiesView{
		Management:        s.Management,
		ProfileNamespaces: []string{},
		ViewFleet:         allowed("list", "targetclusters", ""),
		ViewAll:           allowed("get", "accessrequests", ""),
	}
	if s.Management {
		caps.ManageClusters = allowed("update", "targetclusters", "")
		caps.AddClusters = allowed("create", "targetclusters", "")
		if allowed("create", "accessprofiles", "") {
			caps.ProfileNamespaces = []string{"*"}
		} else {
			var profiles protemv1alpha1.AccessProfileList
			if err := s.Client.List(ctx, &profiles); err != nil {
				s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
				return
			}
			seen := map[string]bool{}
			for _, p := range profiles.Items {
				if !seen[p.Namespace] {
					seen[p.Namespace] = true
					if allowed("update", "accessprofiles", p.Namespace) {
						caps.ProfileNamespaces = append(caps.ProfileNamespaces, p.Namespace)
					}
				}
			}
			sort.Strings(caps.ProfileNamespaces)
		}
	}
	s.writeJSON(w, http.StatusOK, caps)
}

func (s *Server) activeGrantCounts(ctx context.Context) (map[string]int, error) {
	var grants protemv1alpha1.AccessGrantList
	if err := s.Client.List(ctx, &grants); err != nil {
		return nil, err
	}
	active := map[string]int{}
	for i := range grants.Items {
		if g := &grants.Items[i]; g.Status.Phase == protemv1alpha1.GrantPhaseActive {
			for _, c := range g.Spec.Clusters {
				active[c]++
			}
		}
	}
	return active, nil
}

// clusterView reports the agent's Lease renewal as the heartbeat: the controller writes
// status.lastHeartbeat only when readiness changes.
func (s *Server) clusterView(ctx context.Context, c *protemv1alpha1.TargetCluster, activeGrants int) ClusterView {
	view := toClusterView(c, activeGrants)
	var lease coordinationv1.Lease
	key := client.ObjectKey{Namespace: fleet.ClusterNamespace(c.Name), Name: fleet.AgentLeaseName}
	if err := s.Client.Get(ctx, key, &lease); err == nil {
		if lease.Spec.RenewTime != nil {
			if renewed := lease.Spec.RenewTime.Time; view.LastHeartbeat == nil || renewed.After(*view.LastHeartbeat) {
				view.LastHeartbeat = &renewed
			}
		}
		for _, role := range strings.Split(lease.Annotations[fleet.AllowedRolesAnnotation], ",") {
			if role = strings.TrimSpace(role); role != "" {
				view.AllowedRoles = append(view.AllowedRoles, role)
			}
		}
		view.AllowedRolesPartial = len(view.AllowedRoles) > 0 && lease.Annotations[fleet.AllowedRolesPartialAnnotation] == "true"
	}
	switch {
	case c.Name == s.HubClusterName:
		view.Agent = "none"
	case view.LastHeartbeat == nil:
		view.Agent = "waiting"
	case meta.IsStatusConditionTrue(c.Status.Conditions, "Ready"):
		view.Agent = "connected"
	default:
		view.Agent = "lost"
	}
	return view
}

func (s *Server) handleClusters(w http.ResponseWriter, r *http.Request, _ UserInfo) {
	ctx := r.Context()

	var list protemv1alpha1.TargetClusterList
	if err := s.Client.List(ctx, &list); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	active, err := s.activeGrantCounts(ctx)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	slices.SortFunc(list.Items, func(a, b protemv1alpha1.TargetCluster) int { return byName(&a, &b) })
	views := make([]ClusterView, 0, len(list.Items))
	for i := range list.Items {
		views = append(views, s.clusterView(ctx, &list.Items[i], active[list.Items[i].Name]))
	}
	s.writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx := r.Context()

	var list protemv1alpha1.AccessProfileList
	if err := s.Client.List(ctx, &list); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	var spokes protemv1alpha1.TargetClusterList
	if err := s.Client.List(ctx, &spokes); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}

	person := toPerson(user)
	slices.SortFunc(list.Items, func(a, b protemv1alpha1.AccessProfile) int { return byName(&a, &b) })
	views := make([]ProfileView, 0, len(list.Items))
	for i := range list.Items {
		views = append(views, toProfileView(&list.Items[i], person, spokes.Items))
	}

	s.writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleGetRequests(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx, q := r.Context(), r.URL.Query()
	targetNamespace := r.PathValue("ns")
	state, stateErr := choiceFilter(q, "state", "open", "decided")
	mine, mineErr := boolFilter(q, "mine")
	approver, approverErr := boolFilter(q, "approver")
	if err := errors.Join(stateErr, mineErr, approverErr); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: err.Error()})
		return
	}

	var list protemv1alpha1.AccessRequestList
	var listOpts []client.ListOption
	if targetNamespace != "" {
		listOpts = append(listOpts, client.InNamespace(targetNamespace))
	}

	if err := s.Client.List(ctx, &list, listOpts...); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}

	// Check if user is an auditor or cluster admin with SAR get permission
	hasGlobalRead, _ := s.checkSAR(ctx, user, "get", "accessrequests", targetNamespace, "")

	person := toPerson(user)
	slices.SortFunc(list.Items, func(a, b protemv1alpha1.AccessRequest) int { return newestFirst(&a, &b) })
	matching := make([]*protemv1alpha1.AccessRequest, 0, len(list.Items))
	for i := range list.Items {
		req := &list.Items[i]
		switch {
		case !hasGlobalRead && !approval.CanView(person, req),
			state != "" && (requestPhase(req) == string(protemv1alpha1.RequestPhasePending)) != (state == "open"),
			mine && !approval.SamePerson(person, approval.Person(req.Spec.Requester)),
			approver && !isNamedApprover(person, req):
			continue
		}
		matching = append(matching, req)
	}
	page, total, err := paginate(q, matching)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: err.Error()})
		return
	}

	now, flagged := metav1.Now(), s.flaggedRules(ctx)
	views := make([]RequestView, 0, len(page))
	for _, req := range page {
		views = append(views, toRequestView(req, person, now, flagged(req)))
	}
	s.writeJSON(w, http.StatusOK, RequestPage{Items: views, Total: total})
}

func isNamedApprover(p approval.Person, req *protemv1alpha1.AccessRequest) bool {
	withoutRequester := *req
	withoutRequester.Spec.Requester = protemv1alpha1.RequesterInfo{}
	return approval.CanView(p, &withoutRequester)
}

func (s *Server) handleGetRequest(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx := r.Context()
	var req protemv1alpha1.AccessRequest
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: r.PathValue("ns"), Name: r.PathValue("name")}, &req); err != nil {
		s.writeKubeError(w, err)
		return
	}
	person := toPerson(user)
	if !approval.CanView(person, &req) {
		allowed, err := s.checkSAR(ctx, user, "get", "accessrequests", req.Namespace, "")
		if err != nil || !allowed {
			s.writeJSON(w, http.StatusForbidden, map[string]string{errorKey: fmt.Sprintf("Forbidden: user %q may not read request %s/%s", user.Email, req.Namespace, req.Name)})
			return
		}
	}
	s.writeJSON(w, http.StatusOK, toRequestView(&req, person, metav1.Now(), s.flaggedRules(ctx)(&req)))
}

func (s *Server) handleCreateRequest(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx := r.Context()
	namespace := r.PathValue("ns")

	var payload CreateRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: "Invalid request body"})
		return
	}

	if payload.ProfileRef == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: "profileRef is required"})
		return
	}

	var dur *metav1.Duration
	if payload.Duration != "" {
		if parsed, err := time.ParseDuration(payload.Duration); err == nil {
			dur = &metav1.Duration{Duration: parsed}
		}
	}

	accessReq := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("req-%d-", time.Now().UnixMilli()),
			Namespace:    namespace,
		},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: payload.ProfileRef,
			Requester: protemv1alpha1.RequesterInfo{
				Username: user.Username,
				Email:    user.Email,
				Groups:   user.Groups,
			},
			Namespaces: payload.Namespaces,
			Clusters:   payload.Clusters,
			Duration:   dur,
			Justification: protemv1alpha1.RequestJustification{
				Reason: payload.Reason,
				Ticket: payload.Ticket,
			},
		},
	}

	// The controller makes the same check and has the last word; this one answers at once.
	var requests protemv1alpha1.AccessRequestList
	var grants protemv1alpha1.AccessGrantList
	if err := errors.Join(s.Client.List(ctx, &requests, client.InNamespace(namespace)), s.Client.List(ctx, &grants, client.InNamespace(namespace))); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	if held := approval.Held(accessReq, payload.Clusters, requests.Items, grants.Items, time.Now()); held != "" {
		s.writeJSON(w, http.StatusConflict, map[string]string{errorKey: held})
		return
	}

	if err := s.Client.Create(ctx, accessReq); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}

	s.writeJSON(w, http.StatusCreated, toRequestView(accessReq, toPerson(user), metav1.Now(), s.flaggedRules(ctx)(accessReq)))
}

func toPerson(user UserInfo) approval.Person {
	return approval.Person{Username: user.Username, Email: user.Email, Groups: user.Groups}
}

func (s *Server) decideRequest(w http.ResponseWriter, r *http.Request, user UserInfo, kind protemv1alpha1.ApprovalDecisionType) {
	ctx := r.Context()
	ns, name := r.PathValue("ns"), r.PathValue("name")

	var payload struct {
		Comment string `json:"comment"`
		Reason  string `json:"reason"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}
	comment := payload.Reason
	if kind == protemv1alpha1.ApprovalDecisionApproved {
		comment = payload.Comment
	}

	person, now := toPerson(user), metav1.Now()
	var targetReq protemv1alpha1.AccessRequest
	// The controllers write this status too (phase changes, the audit cursor). A decision that loses
	// that race is judged again on a fresh read; the pause lets the cached read catch up.
	for attempt := 1; ; attempt++ {
		if done := s.tryDecide(ctx, w, user, person, kind, comment, now, client.ObjectKey{Namespace: ns, Name: name}, &targetReq, attempt == 5); done {
			return
		}
		time.Sleep(time.Duration(attempt) * 20 * time.Millisecond)
	}
}

// tryDecide records the decision once. It reports false only when the write lost a race and may be retried;
// in every other case it has written the response.
func (s *Server) tryDecide(ctx context.Context, w http.ResponseWriter, user UserInfo, person approval.Person, kind protemv1alpha1.ApprovalDecisionType,
	comment string, now metav1.Time, key client.ObjectKey, targetReq *protemv1alpha1.AccessRequest, last bool) bool {
	if err := s.Client.Get(ctx, key, targetReq); err != nil {
		s.writeJSON(w, http.StatusNotFound, map[string]string{errorKey: "AccessRequest not found"})
		return true
	}

	decisions, err := approval.Decide(person, targetReq, kind, comment, now)
	switch {
	case errors.Is(err, approval.ErrNotPending):
		s.writeJSON(w, http.StatusBadRequest, map[string]string{
			errorKey: fmt.Sprintf("Cannot record %s decision on request in %s phase; request must be in Pending phase", kind, targetReq.Status.Phase),
		})
		return true
	case errors.Is(err, approval.ErrUnresolved):
		s.writeJSON(w, http.StatusConflict, map[string]string{
			errorKey: "Request is awaiting requirement resolution by controller",
		})
		return true
	case errors.Is(err, approval.ErrNotEligible):
		s.writeJSON(w, http.StatusForbidden, map[string]string{
			errorKey: fmt.Sprintf("Forbidden: user %q is not eligible to record a %s decision on this request", user.Email, kind),
		})
		return true
	case errors.Is(err, approval.ErrAlreadyApproved):
		s.writeJSON(w, http.StatusConflict, map[string]string{
			errorKey: "You have already approved this request",
		})
		return true
	case err != nil:
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return true
	}

	targetReq.Status.Approvals = append(targetReq.Status.Approvals, decisions...)
	switch err := s.Client.Status().Update(ctx, targetReq); {
	case apierrors.IsConflict(err) && !last:
		return false
	case apierrors.IsConflict(err):
		s.writeJSON(w, http.StatusConflict, map[string]string{errorKey: "The request changed while your decision was being recorded. Try again."})
	case err != nil:
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
	default:
		s.writeJSON(w, http.StatusOK, toRequestView(targetReq, person, now, s.flaggedRules(ctx)(targetReq)))
	}
	return true
}

// grantVisible is the one rule for who may see a grant; req is the grant's request, nil when it no longer exists.
func grantVisible(p approval.Person, mayGetGrants bool, g *protemv1alpha1.AccessGrant, req *protemv1alpha1.AccessRequest) bool {
	return mayGetGrants || approval.SamePerson(p, approval.Person(g.Spec.Requester)) || (req != nil && isNamedApprover(p, req))
}

func (s *Server) canSeeGrant(ctx context.Context, user UserInfo, g *protemv1alpha1.AccessGrant) (bool, error) {
	mayGetGrants, _ := s.checkSAR(ctx, user, "get", "accessgrants", g.Namespace, "")
	var req *protemv1alpha1.AccessRequest
	if g.Spec.RequestRef != "" {
		var found protemv1alpha1.AccessRequest
		switch err := s.Client.Get(ctx, client.ObjectKey{Namespace: g.Namespace, Name: g.Spec.RequestRef}, &found); {
		case err == nil:
			req = &found
		case !apierrors.IsNotFound(err):
			return false, err
		}
	}
	return grantVisible(toPerson(user), mayGetGrants, g, req), nil
}

func grantEnded(g *protemv1alpha1.AccessGrant) bool {
	return g.Status.Phase == protemv1alpha1.GrantPhaseExpired || g.Status.Phase == protemv1alpha1.GrantPhaseRevoked
}

func (s *Server) handleGrants(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx, q := r.Context(), r.URL.Query()
	state, stateErr := choiceFilter(q, "state", "active", "ended")
	mine, mineErr := boolFilter(q, "mine")
	if err := errors.Join(stateErr, mineErr); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: err.Error()})
		return
	}

	var list protemv1alpha1.AccessGrantList
	var requests protemv1alpha1.AccessRequestList
	if err := errors.Join(s.Client.List(ctx, &list), s.Client.List(ctx, &requests)); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	requestByKey := make(map[client.ObjectKey]*protemv1alpha1.AccessRequest, len(requests.Items))
	for i := range requests.Items {
		requestByKey[client.ObjectKeyFromObject(&requests.Items[i])] = &requests.Items[i]
	}

	hasGlobalRead, _ := s.checkSAR(ctx, user, "get", "accessgrants", "", "")

	person := toPerson(user)
	visible := make([]*protemv1alpha1.AccessGrant, 0, len(list.Items))
	for i := range list.Items {
		g := &list.Items[i]
		req := requestByKey[client.ObjectKey{Namespace: g.Namespace, Name: g.Spec.RequestRef}]
		switch {
		case !grantVisible(person, hasGlobalRead, g, req),
			state != "" && grantEnded(g) != (state == "ended"),
			mine && !approval.SamePerson(person, approval.Person(g.Spec.Requester)):
			continue
		}
		visible = append(visible, g)
	}

	slices.SortFunc(visible, func(a, b *protemv1alpha1.AccessGrant) int {
		active := func(g *protemv1alpha1.AccessGrant) int {
			if g.Status.Phase == protemv1alpha1.GrantPhaseActive {
				return 0
			}
			return 1
		}
		return cmp.Or(cmp.Compare(active(a), active(b)), newestFirst(a, b))
	})
	page, total, err := paginate(q, visible)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: err.Error()})
		return
	}

	now := time.Now()
	views := make([]GrantView, 0, len(page))
	canUpdate := map[string]bool{}
	for _, g := range page {
		if _, ok := canUpdate[g.Namespace]; !ok && !approval.SamePerson(person, approval.Person(g.Spec.Requester)) {
			canUpdate[g.Namespace], _ = s.checkSAR(ctx, user, "update", "accessgrants", g.Namespace, "")
		}
		views = append(views, toGrantView(g, person, canUpdate[g.Namespace], now))
	}

	s.writeJSON(w, http.StatusOK, GrantPage{Items: views, Total: total})
}

func (s *Server) handleGetGrant(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx := r.Context()
	var g protemv1alpha1.AccessGrant
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: r.PathValue("ns"), Name: r.PathValue("name")}, &g); err != nil {
		s.writeKubeError(w, err)
		return
	}
	if !s.mayReadGrant(ctx, w, user, &g) {
		return
	}
	person := toPerson(user)
	mayRevokeAny := false
	if !approval.SamePerson(person, approval.Person(g.Spec.Requester)) {
		mayRevokeAny, _ = s.checkSAR(ctx, user, "update", "accessgrants", g.Namespace, g.Name)
	}
	s.writeJSON(w, http.StatusOK, toGrantView(&g, person, mayRevokeAny, time.Now()))
}

func (s *Server) mayReadGrant(ctx context.Context, w http.ResponseWriter, user UserInfo, g *protemv1alpha1.AccessGrant) bool {
	visible, err := s.canSeeGrant(ctx, user, g)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return false
	}
	if !visible {
		s.writeJSON(w, http.StatusForbidden, map[string]string{errorKey: fmt.Sprintf("Forbidden: user %q may not read grant %s/%s", user.Email, g.Namespace, g.Name)})
	}
	return visible
}

func (s *Server) revokeGrant(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx := r.Context()
	ns, name := r.PathValue("ns"), r.PathValue("name")

	var reason string
	if r.Body != nil {
		var payload struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		reason = payload.Reason
	}

	var grant protemv1alpha1.AccessGrant
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &grant); err != nil {
		s.writeJSON(w, http.StatusNotFound, map[string]string{errorKey: "AccessGrant not found"})
		return
	}

	// H5 Authorization check on revoke:
	// 1. Caller is original requester: always allowed to revoke early
	// 2. Otherwise: requires SAR update permission on accessgrants in this namespace
	if !approval.SamePerson(toPerson(user), approval.Person(grant.Spec.Requester)) {
		allowed, err := s.checkSAR(ctx, user, "update", "accessgrants", grant.Namespace, grant.Name)
		if err != nil || !allowed {
			s.writeJSON(w, http.StatusForbidden, map[string]string{
				errorKey: fmt.Sprintf("Forbidden: user %q is not authorized to revoke grant for %q", user.Email, grant.Spec.Requester.Email),
			})
			return
		}
	}
	if grant.Spec.Revocation != nil || !time.Now().Before(grant.Spec.ExpiresAt.Time) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: "Grant is already revoked or expired"})
		return
	}

	grant.Spec.Revocation = &protemv1alpha1.RevocationDetails{
		By:     user.Email,
		At:     metav1.Now(),
		Reason: reason,
	}

	if err := s.Client.Update(ctx, &grant); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}

	s.writeJSON(w, http.StatusOK, toGrantView(&grant, toPerson(user), true, time.Now()))
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request, _ UserInfo) {
	ctx := r.Context()

	var grantList protemv1alpha1.AccessGrantList
	var reqList protemv1alpha1.AccessRequestList
	var profList protemv1alpha1.AccessProfileList
	var clusterList protemv1alpha1.TargetClusterList

	_ = s.Client.List(ctx, &grantList)
	_ = s.Client.List(ctx, &reqList)
	_ = s.Client.List(ctx, &profList)
	_ = s.Client.List(ctx, &clusterList)

	activeGrants := 0
	for _, g := range grantList.Items {
		if g.Status.Phase == protemv1alpha1.GrantPhaseActive && time.Now().Before(g.Spec.ExpiresAt.Time) {
			activeGrants++
		}
	}

	pendingRequests := 0
	for _, r := range reqList.Items {
		if requestPhase(&r) == string(protemv1alpha1.RequestPhasePending) {
			pendingRequests++
		}
	}

	s.writeJSON(w, http.StatusOK, StatsView{
		ActiveGrants:      activeGrants,
		PendingRequests:   pendingRequests,
		TotalProfiles:     len(profList.Items),
		TotalRequests:     len(reqList.Items),
		ConnectedClusters: len(clusterList.Items),
	})
}
