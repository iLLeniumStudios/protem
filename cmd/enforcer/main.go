package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/enforcer"
)

var (
	scheme = runtime.NewScheme()
	log    = ctrl.Log.WithName("agent-main")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(protemv1alpha1.AddToScheme(scheme))
}

func main() {
	var clusterName string
	var targetKubeconfig string
	var targetContext string
	var hubKubeconfig string
	var hubContext string
	var subjectTemplate string
	var syncInterval time.Duration
	var hubTokenTTL time.Duration
	var hubTokenSecret string

	flag.StringVar(&clusterName, "cluster-name", "", "Name of this cluster in the Protem fleet (required)")
	flag.StringVar(&targetKubeconfig, "target-kubeconfig", "",
		"Path to target cluster kubeconfig (defaults to standard kubeconfig)")
	flag.StringVar(&targetContext, "target-context", "", "Override kubectl context to connect to the target cluster")
	flag.StringVar(&hubKubeconfig, "hub-kubeconfig", "", "Path to hub kubeconfig (defaults to standard kubeconfig)")
	flag.StringVar(&hubContext, "hub-context", "", "Override kubectl context to connect to hub cluster")
	flag.StringVar(&subjectTemplate, "subject-template", "{{.Email}}", "Template for mapping user to RBAC subject")
	flag.DurationVar(&syncInterval, "sync-interval", 3*time.Second, "Polling and sync interval with Hub")
	flag.DurationVar(&hubTokenTTL, "hub-token-ttl", 24*time.Hour,
		"Lifetime of each hub token the agent requests; it renews after a third of it (minimum 10m)")
	flag.StringVar(&hubTokenSecret, "hub-token-secret", "",
		"<namespace>/<name> of a Secret on the target cluster that stores the renewed hub token (empty: memory only)")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if clusterName == "" {
		log.Error(nil, "--cluster-name flag is required")
		os.Exit(1)
	}
	// Without either flag the hub client would fall back to the same config as the target cluster
	// (in a pod: this cluster's own service account) and the agent would treat the target as the hub.
	if hubKubeconfig == "" && hubContext == "" {
		log.Error(nil, "one of --hub-kubeconfig or --hub-context is required")
		os.Exit(1)
	}
	if hubTokenTTL < enforcer.MinHubTokenTTL {
		log.Error(nil, "--hub-token-ttl must be at least 10m", "hubTokenTTL", hubTokenTTL.String())
		os.Exit(1)
	}
	var tokenSecret client.ObjectKey
	if hubTokenSecret != "" {
		namespace, name, ok := strings.Cut(hubTokenSecret, "/")
		if !ok || namespace == "" || name == "" || strings.Contains(name, "/") {
			log.Error(nil, "--hub-token-secret must be <namespace>/<name>", "hubTokenSecret", hubTokenSecret)
			os.Exit(1)
		}
		tokenSecret = client.ObjectKey{Namespace: namespace, Name: name}
	}

	log.Info("Initializing Protem agent", "cluster", clusterName,
		"targetContext", targetContext, "hubContext", hubContext)

	ctx := ctrl.SetupSignalHandler()

	// 1. Target cluster client config
	targetRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if targetKubeconfig != "" {
		targetRules.ExplicitPath = targetKubeconfig
	}
	targetOverrides := &clientcmd.ConfigOverrides{}
	if targetContext != "" {
		targetOverrides.CurrentContext = targetContext
	}
	targetCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(targetRules, targetOverrides).ClientConfig()
	if err != nil {
		log.Error(err, "Failed to load kubeconfig for target cluster")
		os.Exit(1)
	}

	targetClient, err := client.New(targetCfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "Failed to create Kubernetes client for target cluster")
		os.Exit(1)
	}

	// 2. Hub Client Config
	hubRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if hubKubeconfig != "" {
		hubRules.ExplicitPath = hubKubeconfig
	}
	hubOverrides := &clientcmd.ConfigOverrides{}
	if hubContext != "" {
		hubOverrides.CurrentContext = hubContext
	}
	hubCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(hubRules, hubOverrides).ClientConfig()
	if err != nil {
		log.Error(err, "Failed to load kubeconfig for hub cluster")
		os.Exit(1)
	}

	tokenRenewer, err := enforcer.NewHubTokenRenewer(ctx, hubCfg, targetClient, tokenSecret, hubTokenTTL)
	if err != nil {
		log.Error(err, "Failed to set up hub token renewal")
		os.Exit(1)
	}

	hubClient, err := client.New(hubCfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "Failed to create Kubernetes client for hub cluster")
		os.Exit(1)
	}
	if tokenRenewer != nil {
		go tokenRenewer.Run(ctx, hubClient)
	}

	agent := enforcer.NewAgent(enforcer.AgentConfig{
		ClusterName:     clusterName,
		HubClient:       hubClient,
		SpokeClient:     targetClient,
		SubjectTemplate: subjectTemplate,
		SyncInterval:    syncInterval,
	})

	if err := agent.Start(ctx); err != nil {
		log.Error(err, "Agent encountered fatal error")
		os.Exit(1)
	}
}
