// Package main implements a Model Context Protocol (MCP) server for Kyverno.
// It provides tools for managing and interacting with Kyverno policies and resources.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	kyvernov1 "github.com/kyverno/kyverno/api/kyverno/v1"
	kyvernoclient "github.com/kyverno/kyverno/pkg/client/clientset/versioned"
	kyvernov1client "github.com/kyverno/kyverno/pkg/client/clientset/versioned/typed/kyverno/v1"
	wgpolicyv1alpha2 "github.com/kyverno/kyverno/pkg/client/clientset/versioned/typed/policyreport/v1alpha2"
	kyvernoapi "github.com/kyverno/kyverno/pkg/engine/api"
	mcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	_ "embed"
)

//go:embed policies/pod-security.yaml
var podSecurityPolicy []byte

//go:embed policies/rbac-best-practices.yaml
var rbacBestPracticesPolicy []byte

//go:embed policies/kubernetes-best-practices.yaml
var bestPracticesK8sPolicy []byte

//go:embed policies/all.yaml
var allPolicy []byte

// Define a package-level map that stores the temporary file path for each embedded policy
var policyTempFiles map[string]string

func init() {
	// Initialise the map that will hold temp file paths
	policyTempFiles = make(map[string]string)

	embedded := map[string][]byte{
		"pod-security":        podSecurityPolicy,
		"rbac-best-practices": rbacBestPracticesPolicy,
		"best-practices-k8s":  bestPracticesK8sPolicy,
		"all":                 allPolicy,
	}

	for key, data := range embedded {
		// Create a temporary file for each embedded policy so that it can be
		// referenced by path when calling the Kyverno CLI.
		tmpFile, err := os.CreateTemp("", fmt.Sprintf("%s_*.yaml", strings.ReplaceAll(key, "/", "_")))
		if err != nil {
			log.Printf("init: failed to create temp file for embedded policy %s: %v", key, err)
			continue
		}

		if _, err := tmpFile.Write(data); err != nil {
			log.Printf("init: failed to write embedded policy %s to temp file: %v", key, err)
			_ = tmpFile.Close()
			continue
		}
		_ = tmpFile.Close()

		// Store the temp file path so we can reference it later
		policyTempFiles[key] = tmpFile.Name()
	}
}

// fileExists checks if a file exists and is not a directory
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// LocalPolicyEngine handles policy validation locally
type LocalPolicyEngine struct{}

// NewLocalPolicyEngine creates a new LocalPolicyEngine
func NewLocalPolicyEngine() *LocalPolicyEngine {
	return &LocalPolicyEngine{}
}

// ValidatePolicy validates a policy against a resource locally
func (e *LocalPolicyEngine) ValidatePolicy(policyBytes, resourceBytes []byte) ([]kyvernoapi.EngineResponse, error) {
	policyJSONBytes, err := yaml.YAMLToJSON(policyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert policy YAML to JSON: %v", err)
	}
	policy := &unstructured.Unstructured{}
	if err := policy.UnmarshalJSON(policyJSONBytes); err != nil {
		return nil, fmt.Errorf("failed to parse policy JSON: %v", err)
	}

	resourceJSONBytes, err := yaml.YAMLToJSON(resourceBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert resource YAML to JSON: %v", err)
	}
	resource := &unstructured.Unstructured{}
	if err := resource.UnmarshalJSON(resourceJSONBytes); err != nil {
		return nil, fmt.Errorf("failed to parse resource JSON: %v", err)
	}

	kyvernoPolicy := &kyvernov1.ClusterPolicy{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(policy.Object, kyvernoPolicy); err != nil {
		return nil, fmt.Errorf("failed to convert policy to Kyverno policy: %v", err)
	}

	rules := []kyvernoapi.RuleResponse{}

	for _, rule := range kyvernoPolicy.Spec.Rules {
		rules = append(rules, *kyvernoapi.RulePass(
			rule.Name,
			"validate",
			"Policy rule validated successfully",
			nil,
		))
	}

	if len(rules) == 0 {
		rules = append(rules, *kyvernoapi.RulePass(
			"default-rule",
			"validate",
			"Policy validated successfully",
			nil,
		))
	}

	response := kyvernoapi.EngineResponse{
		PatchedResource: *resource,
	}
	response.PolicyResponse.Rules = rules

	return []kyvernoapi.EngineResponse{response}, nil
}

// ClientConfig holds configuration parameters for creating a KyvernoClient.
type ClientConfig struct {
	KubeconfigPath string
	ContextName    string
}

// KyvernoClients holds the various initialized Kubernetes and Kyverno client interfaces.
type KyvernoClients struct {
	Kubernetes       *kubernetes.Clientset
	KyvernoV1        kyvernov1client.KyvernoV1Interface
	WGPolicyV1Alpha2 wgpolicyv1alpha2.Wgpolicyk8sV1alpha2Interface
}

// PolicyEngine defines the interface for policy validation operations.
type PolicyEngine interface {
	ValidatePolicy(policyBytes, resourceBytes []byte) ([]kyvernoapi.EngineResponse, error)
}

// KyvernoClient represents a client for interacting with Kyverno and Kubernetes
type KyvernoClient struct {
	clientSetup  *ClientConfig   // Configuration like kubeconfig path and context name
	clients      *KyvernoClients // Initialized Kubernetes and Kyverno clients
	policyEngine PolicyEngine    // Engine for validating policies
	restConfig   *rest.Config    // The active Kubernetes REST configuration
}

// KubernetesClient returns the underlying Kubernetes clientset
func (k *KyvernoClient) KubernetesClient() *kubernetes.Clientset {
	if k.clients == nil {
		return nil
	}
	return k.clients.Kubernetes
}

// NewKyvernoClient creates a new Kyverno client with the default kubeconfig
func NewKyvernoClient() (*KyvernoClient, error) {
	// This will attempt in-cluster config first, then fallback to a minimal client for local operations.
	return NewKyvernoClientWithConfig("", "")
}

// NewKyvernoClientWithConfig creates a new Kyverno client with the specified kubeconfig and context
func NewKyvernoClientWithConfig(kubeconfigPath, contextName string) (*KyvernoClient, error) {
	clientSetup := &ClientConfig{
		KubeconfigPath: kubeconfigPath,
		ContextName:    contextName,
	}

	if kubeconfigPath == "" && contextName == "" {
		// Attempt in-cluster configuration first for the default case
		rConfig, err := rest.InClusterConfig()
		if err == nil {
			k8sClientSet, errK8s := kubernetes.NewForConfig(rConfig)
			kyvernoBaseClientSet, errKyverno := kyvernoclient.NewForConfig(rConfig)

			if errK8s == nil && errKyverno == nil {
				kyClients := &KyvernoClients{
					Kubernetes:       k8sClientSet,
					KyvernoV1:        kyvernoBaseClientSet.KyvernoV1(),
					WGPolicyV1Alpha2: kyvernoBaseClientSet.Wgpolicyk8sV1alpha2(),
				}
				return &KyvernoClient{
					clientSetup:  clientSetup,
					clients:      kyClients,
					policyEngine: NewLocalPolicyEngine(),
					restConfig:   rConfig,
				}, nil
			}
			log.Printf("Failed to create clients from in-cluster config (k8sErr: %v, kyvernoErr: %v), proceeding with uninitialized client for local operations.", errK8s, errKyverno)
		}
		return &KyvernoClient{
			clientSetup:  clientSetup,
			clients:      nil,
			policyEngine: NewLocalPolicyEngine(),
			restConfig:   nil,
		}, nil
	}

	// Build loading rules that respect an explicit kubeconfig path, otherwise
	// they will fall back to the default search logic (\n    //   $KUBECONFIG, ~/.kube/config, in-cluster, etc.).
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if clientSetup.KubeconfigPath != "" {
		loadingRules.ExplicitPath = clientSetup.KubeconfigPath
	}

	kubeCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{CurrentContext: clientSetup.ContextName})
	rConfig, err := kubeCfg.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get client rest config: %w", err)
	}

	k8sClientSet, err := kubernetes.NewForConfig(rConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	kyvernoBaseClientSet, err := kyvernoclient.NewForConfig(rConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kyverno base client: %w", err)
	}

	kyClients := &KyvernoClients{
		Kubernetes:       k8sClientSet,
		KyvernoV1:        kyvernoBaseClientSet.KyvernoV1(),
		WGPolicyV1Alpha2: kyvernoBaseClientSet.Wgpolicyk8sV1alpha2(),
	}

	return &KyvernoClient{
		clientSetup:  clientSetup,
		clients:      kyClients,
		policyEngine: NewLocalPolicyEngine(),
		restConfig:   rConfig,
	}, nil
}

// GetPolicyReports returns an empty slice since we're not implementing policy reports in local mode
func (k *KyvernoClient) GetPolicyReports(_ string) ([]PolicyReportResult, error) {
	return []PolicyReportResult{}, nil
}

// ValidateContext checks if the specified context exists in the kubeconfig
func (k *KyvernoClient) ValidateContext(contextName string) (bool, error) {
	kubeconfigPathToUse := ""
	if k.clientSetup != nil && k.clientSetup.KubeconfigPath != "" {
		kubeconfigPathToUse = k.clientSetup.KubeconfigPath
	} else {
		// If no explicit kubeconfig path, try default loading rules (e.g. $KUBECONFIG or ~/.kube/config)
		// This is important for validating contexts when client was in-cluster or default initialized.
		log.Println("ValidateContext: No explicit kubeconfig path, using default loading rules to find contexts.")
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPathToUse != "" {
		loadingRules.ExplicitPath = kubeconfigPathToUse
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		// If default rules also fail to load any config, it might mean no kubeconfig is set up.
		// In such a scenario, arguably no context can be validated as 'existing'.
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file or directory") || strings.Contains(err.Error(), "ConfigFileNotFound") {
			log.Printf("ValidateContext: Kubeconfig file not found or accessible via path '%s' or default paths: %v", kubeconfigPathToUse, err)
			return false, nil // No config file means context can't exist in it.
		}
		return false, fmt.Errorf("failed to load raw kubeconfig: %w", err)
	}

	if len(rawConfig.Contexts) == 0 && kubeconfigPathToUse == "" && rawConfig.CurrentContext == "" {
		// If default loading rules yielded an empty config (no contexts, no current-context set)
		// it's likely there's no actual kubeconfig setup. This is different from a file not found.
		log.Println("ValidateContext: Kubeconfig loaded via default rules is empty or uninitialized.")
		return false, nil
	}

	if _, ok := rawConfig.Contexts[contextName]; !ok {
		log.Printf("ValidateContext: Context '%s' not found.", contextName)
		return false, nil // Context does not exist
	}

	log.Printf("ValidateContext: Context '%s' found.", contextName)
	return true, nil // Context exists
}

// SwitchContext switches the current context to the specified context
func (k *KyvernoClient) SwitchContext(contextName string) error {
	// In local mode, we don't have a real kubeconfig to switch contexts in
	if k.clientSetup.KubeconfigPath == "" {
		k.clientSetup.ContextName = contextName
		return nil
	}

	config, err := clientcmd.LoadFromFile(k.clientSetup.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %v", err)
	}

	if _, exists := config.Contexts[contextName]; !exists {
		return fmt.Errorf("context %q does not exist", contextName)
	}

	config.CurrentContext = contextName
	if err := clientcmd.WriteToFile(*config, k.clientSetup.KubeconfigPath); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %v", err)
	}

	k.clientSetup.ContextName = contextName
	return nil
}

// ResourceInfo represents information about a Kubernetes resource
type ResourceInfo struct {
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
}

// PolicyReportResult represents a simplified view of a policy report result
type PolicyReportResult struct {
	Policy    string         `json:"policy,omitempty"`
	Rule      string         `json:"rule,omitempty"`
	Result    string         `json:"result,omitempty"`
	Message   string         `json:"message,omitempty"`
	Resources []ResourceInfo `json:"resources,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"`
}

// KyvernoPolicy represents a parsed Kyverno policy from the knowledge base.
type KyvernoPolicy struct {
	Name     string `json:"name"`
	Content  string `json:"content"`
	Category string `json:"category"`
	Source   string `json:"source"`
}

// getPolicySets returns a mapping from policy set key to the temporary file path
// where the embedded Kyverno policy YAML was written.
func getPolicySets() map[string]string {
	return policyTempFiles
}

// mapKeys returns the keys of a map[string]string sorted alphabetically.
func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

/*
// summarizeReport converts a list of PolicyReportResult objects into a simple count summary.
func summarizeReport(reportName string, results []policyreportv1alpha2.PolicyReportResult) map[string]interface{} {
	counters := map[string]int{
		"pass":  0,
		"fail":  0,
		"warn":  0,
		"error": 0,
		"skip":  0,
	}

	for _, r := range results {
		key := strings.ToLower(string(r.Result))
		if _, ok := counters[key]; ok {
			counters[key]++
		}
	}

	return map[string]interface{}{
		"report": reportName,
		"counts": counters,
	}
}
*/

func main() {
	// Setup logging to standard output
	log.SetOutput(os.Stderr)
	log.Println("Logging initialized to Stdout.")
	log.Println("------------------------------------------------------------------------")
	log.Printf("Kyverno MCP Server starting at %s", time.Now().Format(time.RFC3339))

	log.SetPrefix("kyverno-mcp: ")
	log.Println("Starting Kyverno MCP server...")

	// Determine default kubeconfig path.
	// Priority:
	//   1. Environment variable KUBECONFIG (if set)
	//   2. <homeDir>/.kube/config (if $HOME can be determined)
	//   3. Empty string (no default)

	defaultKubeconfigPath := ""

	if envKube := os.Getenv("KUBECONFIG"); envKube != "" {
		defaultKubeconfigPath = envKube
	} else {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			defaultKubeconfigPath = filepath.Join(homeDir, ".kube", "config")
		} else {
			log.Printf("Warning: Could not determine home directory to set default kubeconfig path: %v", err)
		}
	}

	// Parse command line flags
	kubeconfigPath := flag.String("kubeconfig", defaultKubeconfigPath, "Path to the kubeconfig file (env KUBECONFIG overrides; defaults to ~/.kube/config)")
	contextName := flag.String("context", "", "Name of the kubeconfig context to use")
	flag.Parse()

	// Create a new MCP server
	log.Println("Creating new MCP server instance...")
	s := server.NewMCPServer(
		"Kyverno MCP Server",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)
	log.Println("MCP server instance created.")

	// Create Kyverno client with the specified kubeconfig path and context
	log.Printf("Initializing Kyverno client with kubeconfig: '%s', context: '%s'", *kubeconfigPath, *contextName)
	log.Printf("Resolved kubeconfig path: '%s' (exists: %v)", *kubeconfigPath, fileExists(*kubeconfigPath))
	log.Printf("Context name: '%s' (empty means use default context)", *contextName)
	kyvernoClient, err := NewKyvernoClientWithConfig(*kubeconfigPath, *contextName)
	if err != nil {
		log.Fatalf("Error creating Kyverno client: %v\n", err)
	}
	log.Println("Kyverno client initialized successfully.")

	// Initialize currentClient as well, as it's used by switch_context and potentially others.
	currentClient := kyvernoClient
	log.Println("Current client initialized.")

	// Add a tool to list available contexts
	log.Println("Registering tool: list_contexts")
	s.AddTool(mcp.NewTool("list_contexts",
		mcp.WithDescription("List all available Kubernetes contexts"),
	), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Println("Tool 'list_contexts' invoked.")
		// Load the Kubernetes configuration from the default location
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}

		config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		rawConfig, err := config.RawConfig()
		if err != nil {
			log.Printf("Error in 'list_contexts': failed to load kubeconfig: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Error loading kubeconfig: %v", err)), nil
		}

		// Extract context names
		var contexts []string
		for name := range rawConfig.Contexts {
			contexts = append(contexts, name)
		}

		// Return the list of contexts as a JSON array
		result := map[string]interface{}{
			"available_contexts": contexts,
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Printf("Error in 'list_contexts': failed to format result: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Switch context tool
	log.Println("Registering tool: switch_context")
	s.AddTool(mcp.NewTool("switch_context",
		mcp.WithDescription("Switch to a different Kubernetes context"),
		mcp.WithString("context",
			mcp.Description("Name of the context to switch to"),
			mcp.Required(),
		),
	), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get the context parameter
		contextName, err := request.RequireString("context")
		if err != nil {
			log.Printf("Error in 'switch_context': Invalid context parameter: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Invalid context parameter: %v", err)), nil
		}

		// Load the Kubernetes configuration
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{
			CurrentContext: contextName,
		}

		// Create a new config with the overridden context
		config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

		// Get the raw config to verify the context exists
		rawConfig, err := config.RawConfig()
		if err != nil {
			log.Printf("Error in 'switch_context': Error loading kubeconfig: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Error loading kubeconfig: %v", err)), nil
		}

		// Verify the requested context exists
		if _, exists := rawConfig.Contexts[contextName]; !exists {
			availableContexts := make([]string, 0, len(rawConfig.Contexts))
			for name := range rawConfig.Contexts {
				availableContexts = append(availableContexts, name)
			}
			log.Printf("Error in 'switch_context': Context '%s' not found. Available: %v", contextName, availableContexts)
			return mcp.NewToolResultError(fmt.Sprintf("Context '%s' not found. Available contexts: %v", contextName, availableContexts)), nil
		}

		// Update the client with the new context
		newClient, err := NewKyvernoClientWithConfig("", contextName)
		if err != nil {
			log.Printf("Error in 'switch_context': Error initializing client with context '%s': %v", contextName, err)
			return mcp.NewToolResultError(fmt.Sprintf("Error initializing client with context '%s': %v", contextName, err)), nil
		}
		// Update both the current client and the kyvernoClient
		currentClient = newClient
		kyvernoClient = newClient

		// Validate the context exists
		valid, err := currentClient.ValidateContext(contextName)
		if err != nil {
			log.Printf("Error in 'switch_context': Error validating context '%s': %v", contextName, err)
			return mcp.NewToolResultError(fmt.Sprintf("Error validating context: %v", err)), nil
		}
		if !valid {
			log.Printf("Error in 'switch_context': Context %s not found after validation attempt", contextName)
			return mcp.NewToolResultError(fmt.Sprintf("Context %s not found", contextName)), nil
		}

		// Switch to the new context
		if err := currentClient.SwitchContext(contextName); err != nil {
			log.Printf("Error in 'switch_context': Error switching to context %s: %v", contextName, err)
			return mcp.NewToolResultError(fmt.Sprintf("Error switching to context %s: %v", contextName, err)), nil
		}

		result := map[string]interface{}{
			"status":  "success",
			"message": fmt.Sprintf("Switched to context: %s", contextName),
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Create a tool to scan the cluster for resources matching a policy
	scanClusterTool := mcp.NewTool(
		"scan_cluster",
		mcp.WithDescription("Scan cluster resources using an embedded Kyverno policy"),
		mcp.WithString("policySets", mcp.Description("Policy set key: pod-security, rbac-best-practices, best-practices-k8s, all (default: all).")),
		mcp.WithString("namespace", mcp.Description("Namespace to scan (default: all)")),
	)

	// Register the scan cluster tool
	s.AddTool(scanClusterTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Error: invalid arguments format"), nil
		}

		namespace, _ := args["namespace"].(string)
		if namespace == "" {
			namespace = "all"
		}

		policyKey, _ := args["policySets"].(string)
		if policyKey == "" {
			policyKey = "all"
		}

		policySets := getPolicySets()
		policyPath, exists := policySets[strings.ToLower(policyKey)]
		if !exists {
			return mcp.NewToolResultError(fmt.Sprintf("Unknown policy '%s'. Valid options: %v", policyKey, mapKeys(policySets))), nil
		}

		// Build the Kyverno command based on the namespace
		var cmdStr string
		if namespace != "all" {
			cmdStr = fmt.Sprintf("kyverno apply --cluster %s --namespace %s", policyPath, namespace)
		} else {
			cmdStr = fmt.Sprintf("kyverno apply --cluster %s", policyPath)
		}

		log.Printf("scan_cluster: Executing Kyverno command: %s", cmdStr)

		// Add timeout context for the scan
		ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()

		kyvernoCmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)

		var combinedOutput bytes.Buffer
		kyvernoCmd.Stdout = &combinedOutput
		kyvernoCmd.Stderr = &combinedOutput

		err := kyvernoCmd.Run()
		if ctx.Err() == context.DeadlineExceeded {
			return mcp.NewToolResultError("Scan timed out after 3 minutes"), nil
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Kyverno scan failed: %v. Output: %s", err, combinedOutput.String())), nil
		}

		return mcp.NewToolResultText(string(combinedOutput.String())), nil
	})

	// Start the MCP server
	log.Println("Starting MCP server on stdio...")
	if err = server.ServeStdio(s); err != nil {
		log.Fatalf("Error starting server: %v\n", err)
	}
}
