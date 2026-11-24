/*
Copyright 2026 The Protem Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/api"
	"github.com/iLLeniumStudios/protem/internal/controller"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(protemv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// flagsFromEnv sets each flag --some-name from PROTEM_SOME_NAME. Call it before parsing so the
// command line wins: OLM's Subscription can set environment variables on the operator, not arguments.
func flagsFromEnv(fs *flag.FlagSet, lookup func(string) (string, bool)) error {
	var err error
	fs.VisitAll(func(f *flag.Flag) {
		name := "PROTEM_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if value, ok := lookup(name); ok && err == nil {
			if setErr := fs.Set(f.Name, value); setErr != nil {
				err = fmt.Errorf("%s=%q: %w", name, value, setErr)
			}
		}
	})
	return err
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var apiAddr string
	var devAuth bool
	var oidcIssuerURL string
	var oidcClientID string
	var hubClusterName string
	var subjectTemplate string
	var uiManagement string
	var signOutURL string
	var historyRetention time.Duration
	var hubAPIURL, agentImage, agentManifests string
	var joinTokenTTL time.Duration
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&apiAddr, "api-bind-address", ":8080", "The address the Protem web UI & REST API server binds to.")
	flag.BoolVar(&devAuth, "dev-auth", false,
		"Enable development personas via X-Test-User (refuses to start unless bound to loopback)")
	flag.StringVar(&oidcIssuerURL, "oidc-issuer-url", "", "OIDC Issuer URL (e.g. http://localhost:8085/realms/protem)")
	flag.StringVar(&oidcClientID, "oidc-client-id", "", "OIDC Client ID (e.g. protem)")
	flag.StringVar(&hubClusterName, "hub-cluster-name", "local", "Cluster name of the hub in the fleet")
	flag.StringVar(&subjectTemplate, "subject-template", "{{.Email}}",
		"Template for mapping user to RBAC subject (e.g. {{.Email}} or oidc:{{.Email}})")
	flag.StringVar(&uiManagement, "ui-management", "readonly",
		"readonly, or enabled to let the UI create, edit and delete AccessProfiles and add, edit and remove "+
			"TargetClusters (requires config/rbac/ui_management_role.yaml)")
	flag.StringVar(&hubAPIURL, "hub-api-url", "",
		"The hub's Kubernetes API URL as agents on target clusters reach it; required to connect agents from the UI")
	flag.StringVar(&agentImage, "agent-image", api.DefaultAgentImage, "The agent image the UI's install files use")
	flag.StringVar(&agentManifests, "agent-manifests", api.DefaultAgentManifests,
		"The kustomize location of the agent manifests the UI's install files use")
	flag.DurationVar(&joinTokenTTL, "join-token-ttl", api.DefaultJoinTokenTTL,
		"How long a join token issued in the UI works, from 10m to 1h")
	flag.StringVar(&signOutURL, "sign-out-url", "",
		"URL the UI opens to sign out (e.g. /oauth2/sign_out); empty hides the link")
	flag.DurationVar(&historyRetention, "history-retention", 0,
		"Delete finished requests, and their grants, this long after their last audit event. "+
			"0 keeps them forever; otherwise at least 1m.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	if err := flagsFromEnv(flag.CommandLine, os.LookupEnv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	// A just-issued grant can be missing from the cache for a moment, and a request whose grant is
	// missing counts as finished. A minute keeps that moment from ever deleting a live grant.
	if historyRetention != 0 && historyRetention < time.Minute {
		setupLog.Error(nil, "--history-retention must be 0 or at least 1m", "history-retention", historyRetention)
		os.Exit(1)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher, webhookCertWatcher *certwatcher.CertWatcher

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		var err error
		webhookCertWatcher, err = certwatcher.New(
			filepath.Join(webhookCertPath, webhookCertName),
			filepath.Join(webhookCertPath, webhookCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
			os.Exit(1)
		}

		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
			config.GetCertificate = webhookCertWatcher.GetCertificate
		})
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: webhookTLSOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "to initialize metrics certificate watcher", "error", err)
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	restConfig := ctrl.GetConfigOrDie()
	hubCA := restConfig.CAData
	if len(hubCA) == 0 && restConfig.CAFile != "" {
		var err error
		if hubCA, err = os.ReadFile(restConfig.CAFile); err != nil {
			setupLog.Error(err, "unable to read the hub's certificate authority", "file", restConfig.CAFile)
			os.Exit(1)
		}
	}
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "55bdcc32.protem.dev",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&controller.AccessProfileReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AccessProfile")
		os.Exit(1)
	}
	if err := (&controller.AccessRequestReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AccessRequest")
		os.Exit(1)
	}
	if err := (&controller.AccessGrantReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		HubClusterName:  hubClusterName,
		SubjectTemplate: subjectTemplate,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AccessGrant")
		os.Exit(1)
	}
	if err := (&controller.TargetClusterReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		HubClusterName: hubClusterName,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TargetCluster")
		os.Exit(1)
	}
	// Audit lines go to stdout and nothing else does: the operator's own logs go to stderr.
	if err := (&controller.AuditReconciler{
		Client:    mgr.GetClient(),
		Out:       os.Stdout,
		Retention: historyRetention,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Audit")
		os.Exit(1)
	}

	// Register Protem web UI & REST API server
	apiServer, err := api.NewServer(mgr.GetClient(), api.ServerOptions{
		Addr:            apiAddr,
		DevAuth:         devAuth,
		OIDCIssuerURL:   oidcIssuerURL,
		OIDCClientID:    oidcClientID,
		SubjectTemplate: subjectTemplate,
		UIManagement:    uiManagement,
		SignOutURL:      signOutURL,
		Recorder:        mgr.GetEventRecorderFor("protem-api"),
		HubClusterName:  hubClusterName,
		HubAPIURL:       hubAPIURL,
		HubCAData:       hubCA,
		AgentImage:      agentImage,
		AgentManifests:  agentManifests,
		JoinTokenTTL:    joinTokenTTL,
	})
	if err != nil {
		setupLog.Error(err, "unable to initialize api server")
		os.Exit(1)
	}
	if err := mgr.Add(apiServer); err != nil {
		setupLog.Error(err, "unable to add api server to manager")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := mgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher to manager")
			os.Exit(1)
		}
	}

	if webhookCertWatcher != nil {
		setupLog.Info("Adding webhook certificate watcher to manager")
		if err := mgr.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "unable to add webhook certificate watcher to manager")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
