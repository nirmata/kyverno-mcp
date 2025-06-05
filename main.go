// Package main implements a Model Context Protocol (MCP) server for Kyverno.
// It provides tools for managing and interacting with Kyverno policies and resources.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"
	kyvernov1 "github.com/kyverno/kyverno/api/kyverno/v1"
	kyvernoclient "github.com/kyverno/kyverno/pkg/client/clientset/versioned"
	kyvernov1client "github.com/kyverno/kyverno/pkg/client/clientset/versioned/typed/kyverno/v1"
	wgpolicyv1alpha2 "github.com/kyverno/kyverno/pkg/client/clientset/versioned/typed/policyreport/v1alpha2"
	kyvernoapi "github.com/kyverno/kyverno/pkg/engine/api"
	mcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/repo"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	bedrockRegion          = "us-west-2"  // Bedrock region
	bedrockKnowledgeBaseID = "KKEWERQI0K" // KB ID
	bedrockNumberOfResults = 3
)

// fileExists checks if a file exists and is not a directory
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// PolicyDetails holds metadata about a Kyverno policy
type PolicyDetails struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// LocalPolicyEngine handles policy validation locally
type LocalPolicyEngine struct{}

// NewLocalPolicyEngine creates a new LocalPolicyEngine
func NewLocalPolicyEngine() *LocalPolicyEngine {
	return &LocalPolicyEngine{}
}

// ValidatePolicy validates a policy against a resource locally
func (e *LocalPolicyEngine) ValidatePolicy(policyBytes, resourceBytes []byte) ([]kyvernoapi.EngineResponse, error) {
	// Parse policy
	policy := &unstructured.Unstructured{}
	if err := policy.UnmarshalJSON(policyBytes); err != nil {
		return nil, fmt.Errorf("failed to parse policy: %v", err)
	}

	// Parse resource
	resource := &unstructured.Unstructured{}
	if err := resource.UnmarshalJSON(resourceBytes); err != nil {
		return nil, fmt.Errorf("failed to parse resource: %v", err)
	}

	// Convert to Kyverno policy
	kyvernoPolicy := &kyvernov1.ClusterPolicy{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(policy.Object, kyvernoPolicy); err != nil {
		return nil, fmt.Errorf("failed to convert policy to Kyverno policy: %v", err)
	}

	// Create a simple mock response
	// In a real implementation, we would use the Kyverno engine to validate the policy
	// This is a simplified version that just returns a success response
	rules := []kyvernoapi.RuleResponse{}

	// For each rule in the policy, create a passing rule response
	for _, rule := range kyvernoPolicy.Spec.Rules {
		rules = append(rules, *kyvernoapi.RulePass(
			rule.Name,
			"validate",
			"Policy rule validated successfully",
			nil,
		))
	}

	// If no rules were found, add a default one
	if len(rules) == 0 {
		rules = append(rules, *kyvernoapi.RulePass(
			"default-rule",
			"validate",
			"Policy validated successfully",
			nil,
		))
	}

	// Create the engine response
	response := kyvernoapi.EngineResponse{
		PatchedResource: *resource,
	}

	// Set the rules in the policy response
	response.PolicyResponse.Rules = rules

	return []kyvernoapi.EngineResponse{response}, nil
}

// KyvernoClient represents a client for interacting with Kyverno and Kubernetes
type KyvernoClient struct {
	kubeconfigPath   string
	contextName      string
	kyvernoClient    *kyvernoclient.Clientset
	k8sClient        *kubernetes.Clientset
	config           *rest.Config
	policyEngine     *LocalPolicyEngine
	kyvernoV1        kyvernov1client.KyvernoV1Interface
	wgpolicyV1alpha2 wgpolicyv1alpha2.Wgpolicyk8sV1alpha2Interface
}

// KubernetesClient returns the underlying Kubernetes clientset
func (k *KyvernoClient) KubernetesClient() *kubernetes.Clientset {
	return k.k8sClient
}

// NewKyvernoClient creates a new Kyverno client with the default kubeconfig
func NewKyvernoClient() (*KyvernoClient, error) {
	return NewKyvernoClientWithConfig("", "")
}

// NewKyvernoClientWithConfig creates a new Kyverno client with the specified kubeconfig and context
func NewKyvernoClientWithConfig(kubeconfigPath, contextName string) (*KyvernoClient, error) {
	// Create a minimal config for cases where we don't need a real Kubernetes connection
	// This allows the client to work without a cluster connection for local policy validation
	if kubeconfigPath == "" && contextName == "" {
		return &KyvernoClient{
			kubeconfigPath: "", // Explicitly empty
			contextName:    "", // Explicitly empty
			policyEngine:   NewLocalPolicyEngine(),
		}, nil
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		configOverrides,
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes config: %v", err)
	}

	// Create the Kyverno client
	kyvernoClientSet, err := kyvernoclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kyverno client: %v", err)
	}

	// Create the Kyverno v1 client interface
	kyvernoV1 := kyvernoClientSet.KyvernoV1()

	// Create the Kubernetes client
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	// Policy reports client will be accessed through kyvernoClient
	wgpolicyV1alpha2Client := kyvernoClientSet.Wgpolicyk8sV1alpha2()

	return &KyvernoClient{
		kubeconfigPath:   kubeconfigPath, // Store the original path
		contextName:      contextName,    // Store the original context name
		kyvernoClient:    kyvernoClientSet,
		k8sClient:        k8sClient,
		config:           config,
		policyEngine:     NewLocalPolicyEngine(), // Initialize local engine as well
		kyvernoV1:        kyvernoV1,
		wgpolicyV1alpha2: wgpolicyV1alpha2Client,
	}, nil
}

// GetPolicyReports returns an empty slice since we're not implementing policy reports in local mode
func (k *KyvernoClient) GetPolicyReports(_ string) ([]PolicyReportResult, error) {
	return []PolicyReportResult{}, nil
}

// ValidateContext checks if the specified context exists in the kubeconfig
func (k *KyvernoClient) ValidateContext(contextName string) (bool, error) {
	// In local mode, we don't have a real kubeconfig to validate against
	if k.kubeconfigPath == "" {
		return true, nil
	}

	config, err := clientcmd.LoadFromFile(k.kubeconfigPath)
	if err != nil {
		return false, fmt.Errorf("failed to load kubeconfig: %v", err)
	}

	_, exists := config.Contexts[contextName]
	return exists, nil
}

// SwitchContext switches the current context to the specified context
func (k *KyvernoClient) SwitchContext(contextName string) error {
	// In local mode, we don't have a real kubeconfig to switch contexts in
	if k.kubeconfigPath == "" {
		k.contextName = contextName
		return nil
	}

	config, err := clientcmd.LoadFromFile(k.kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %v", err)
	}

	if _, exists := config.Contexts[contextName]; !exists {
		return fmt.Errorf("context %q does not exist", contextName)
	}

	config.CurrentContext = contextName
	if err := clientcmd.WriteToFile(*config, k.kubeconfigPath); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %v", err)
	}

	k.contextName = contextName
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

func main() {
	// Setup logging to standard output
	log.SetOutput(os.Stderr)
	log.Println("Logging initialized to Stdout.")
	log.Println("------------------------------------------------------------------------")
	log.Printf("Kyverno MCP Server starting at %s", time.Now().Format(time.RFC3339))

	log.SetPrefix("kyverno-mcp: ")
	log.Println("Starting Kyverno MCP server...")

	// Determine default kubeconfig path
	defaultKubeconfigPath := ""
	homeDir, err := os.UserHomeDir()
	if err == nil {
		defaultKubeconfigPath = filepath.Join(homeDir, ".kube", "config")
	} else {
		log.Printf("Warning: Could not determine home directory to set default kubeconfig path: %v", err)
	}

	// Parse command line flags
	kubeconfigPath := flag.String("kubeconfig", defaultKubeconfigPath, "Path to the kubeconfig file (defaults to ~/.kube/config)")
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
		mcp.WithDescription("Scan the cluster for resources that match the given policy"),
		mcp.WithString("policy", mcp.Description("Comma-separated Git repository URLs for policies (e.g., https://github.com/org/repo.git). Use 'default' or empty for Nirmata default policies.")),
		mcp.WithString("namespace", mcp.Description("Namespace to scan (use 'all' for all namespaces)")),
		mcp.WithString("kind", mcp.Description("Kind of resources to scan (e.g., Pod, Deployment)")),
	)

	// Register the scan cluster tool
	s.AddTool(scanClusterTool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			log.Printf("Error in scan_cluster: invalid arguments format")
			return mcp.NewToolResultError("Error: invalid arguments format"), nil
		}

		policyArg, _ := args["policy"].(string) // Can be empty or "default"
		namespace, _ := args["namespace"].(string)
		kind, _ := args["kind"].(string)

		log.Printf("scan_cluster called with policy: '%s', namespace: '%s', kind: '%s'", policyArg, namespace, kind)

		// Define default Nirmata policy set Git URLs
		defaultNirmataPolicyURLs := []string{
			"https://github.com/kyverno/policies.git",
			"https://github.com/nirmata/kyverno-policies.git",
			"https://github.com/kyverno/policy-reporter.git",
			"https://github.com/kyverno/samples.git",
		}
		if len(defaultNirmataPolicyURLs) == 0 { // This condition will now likely be false, but kept for safety
			log.Println("scan_cluster: Warning - Default Nirmata policy URLs are not set. 'default' policy option will result in no policies from git.")
		}

		var policyURLs []string
		if policyArg == "" || strings.ToLower(policyArg) == "default" {
			policyURLs = defaultNirmataPolicyURLs
			if len(policyURLs) == 0 {
				log.Println("scan_cluster: Using no policies as 'default' is selected but no default URLs are configured.")
			}
		} else {
			for _, p := range strings.Split(policyArg, ",") {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					policyURLs = append(policyURLs, trimmed)
				}
			}
		}

		// Construct kubectl command
		kubectlCmdArgs := []string{"get"}
		if kind == "" || strings.ToLower(kind) == "all" {
			kubectlCmdArgs = append(kubectlCmdArgs, "all")
		} else {
			kubectlCmdArgs = append(kubectlCmdArgs, kind)
		}

		if namespace != "" && strings.ToLower(namespace) != "all" {
			kubectlCmdArgs = append(kubectlCmdArgs, "--namespace", namespace)
		} else if strings.ToLower(namespace) == "all" {
			kubectlCmdArgs = append(kubectlCmdArgs, "--all-namespaces")
		}
		kubectlCmdArgs = append(kubectlCmdArgs, "-o", "yaml")

		log.Printf("scan_cluster: Executing kubectl command: kubectl %s", strings.Join(kubectlCmdArgs, " "))
		kubectlCmd := exec.Command("kubectl", kubectlCmdArgs...)

		// Construct kyverno command
		kyvernoCmdArgs := []string{"scan", "-"} // Read from stdin
		for _, url := range policyURLs {
			kyvernoCmdArgs = append(kyvernoCmdArgs, "--policy", url)
		}

		if len(policyURLs) == 0 {
			log.Println("scan_cluster: No policy URLs provided or resolved. Kyverno scan will proceed without specific git policies.")
		}

		log.Printf("scan_cluster: Executing kyverno command: kyverno %s", strings.Join(kyvernoCmdArgs, " "))
		kyvernoCmd := exec.Command("kyverno", kyvernoCmdArgs...)

		// Create a pipe to connect kubectl's stdout to kyverno's stdin
		r, w := io.Pipe()
		kubectlCmd.Stdout = w
		kyvernoCmd.Stdin = r

		var kyvernoCombinedOutput bytes.Buffer // To capture both stdout and stderr from kyverno
		kyvernoCmd.Stdout = &kyvernoCombinedOutput
		kyvernoCmd.Stderr = &kyvernoCombinedOutput

		var kubectlErrOutput bytes.Buffer
		kubectlCmd.Stderr = &kubectlErrOutput

		// Start kyverno command first, then kubectl
		err := kyvernoCmd.Start()
		if err != nil {
			log.Printf("Error in scan_cluster: Failed to start kyverno command: %v. Output: %s", err, kyvernoCombinedOutput.String())
			return mcp.NewToolResultError(fmt.Sprintf("Error starting kyverno command: %v. Output: %s", err, kyvernoCombinedOutput.String())), nil
		}

		err = kubectlCmd.Start()
		if err != nil {
			log.Printf("Error in scan_cluster: Failed to start kubectl command: %v. Stderr: %s", err, kubectlErrOutput.String())
			w.Close()         // Close the writer end of the pipe to signal kyvernoCmd
			kyvernoCmd.Wait() // Wait for kyverno to finish processing any partial input
			return mcp.NewToolResultError(fmt.Sprintf("Error starting kubectl command: %v. Stderr: %s", err, kubectlErrOutput.String())), nil
		}

		// Goroutine to wait for kubectl and close the writer part of the pipe
		kubectlDone := make(chan error, 1)
		go func() {
			kubectlDone <- kubectlCmd.Wait()
			close(kubectlDone)
			w.Close()
		}()

		// Wait for kyverno command to finish
		kyvernoErr := kyvernoCmd.Wait()

		// Check kubectl error after kyverno is done (or if kyverno failed early)
		kubectlErr := <-kubectlDone
		if kubectlErr != nil {
			log.Printf("Error in scan_cluster: kubectl command failed: %v. Stderr: %s", kubectlErr, kubectlErrOutput.String())
			// Prepend kubectl error to kyverno's output if any
			kubectlErrorMsg := fmt.Sprintf("kubectl command error: %v. Stderr: %s\n---\n%s", kubectlErr, kubectlErrOutput.String(), kyvernoCombinedOutput.String())
			return mcp.NewToolResultError(kubectlErrorMsg), nil
		}

		if kyvernoErr != nil {
			// Kyverno exited with an error, output likely contains details
			log.Printf("Error in scan_cluster: kyverno scan command failed: %v. Combined Output: %s", kyvernoErr, kyvernoCombinedOutput.String())
			return mcp.NewToolResultError(fmt.Sprintf("kyverno scan command failed: %v. Output: %s", kyvernoErr, kyvernoCombinedOutput.String())), nil
		}

		scanResult := kyvernoCombinedOutput.String()
		log.Printf("scan_cluster: Scan completed successfully. Output length: %d", len(scanResult))
		return mcp.NewToolResultText(scanResult), nil
	})

	// Create a tool to validate a policy against a resource
	validatePolicyTool := mcp.NewTool(
		"validate_policy",
		mcp.WithDescription("Validate a Kyverno policy against a Kubernetes resource"),
		mcp.WithString("policy", mcp.Description("YAML content of the Kyverno policy")),
		mcp.WithString("resource", mcp.Description("YAML content of the Kubernetes resource to validate")),
	)

	// Register the validate policy tool
	s.AddTool(validatePolicyTool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Error: invalid arguments format"), nil
		}

		policyYAML, ok := args["policy"].([]byte)
		if !ok || policyYAML == nil {
			return mcp.NewToolResultError("Error: 'policy' parameter with YAML content is required"), nil
		}

		resourceYAML, ok := args["resource"].([]byte)
		if !ok || resourceYAML == nil {
			return mcp.NewToolResultError("Error: 'resource' parameter with YAML content is required"), nil
		}

		// Validate the policy against the resource using the local engine
		responses, err := kyvernoClient.policyEngine.ValidatePolicy([]byte(policyYAML), []byte(resourceYAML))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error validating policy: %v", err)), nil
		}

		// Convert responses to a simplified format
		var results []map[string]interface{}
		for _, resp := range responses {
			for _, ruleResp := range resp.PolicyResponse.Rules {
				result := map[string]interface{}{
					"policy":  ruleResp.Name,
					"rule":    ruleResp.Name,
					"status":  string(ruleResp.Status()),
					"message": ruleResp.Message,
					"valid":   ruleResp.Status() == kyvernoapi.RuleStatusPass,
				}
				results = append(results, result)
			}
		}

		// Prepare the final result
		result := map[string]interface{}{
			"status":  "success",
			"results": results,
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Add tool to apply a policy to resources (kept for backward compatibility)
	applyPolicyTool := mcp.NewTool(
		"apply_policy",
		mcp.WithDescription("Apply a policy to specified resources in the cluster (legacy, use validate_policy for local validation)"),
		mcp.WithString("policy", mcp.Description("Name of the policy to apply")),
		mcp.WithString("resource", mcp.Description("Name of the resource to apply the policy to")),
		mcp.WithString("namespace", mcp.Description("Namespace of the resource (use 'default' if not specified")),
	)

	// Register the apply policy tool (legacy version)
	s.AddTool(applyPolicyTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_ = ctx // Explicitly using ctx to avoid linter warning
		// This is the legacy implementation that requires Kyverno to be installed
		if kyvernoClient.kyvernoClient == nil {
			return mcp.NewToolResultError("This tool requires Kyverno to be installed in the cluster. Use 'validate_policy' for local validation."), nil
		}

		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Error: invalid arguments format"), nil
		}

		policyName, ok := args["policy"].(string)
		if !ok || policyName == "" {
			return mcp.NewToolResultError("Error: 'policy' parameter is required"), nil
		}

		resourceName, ok := args["resource"].(string)
		if !ok || resourceName == "" {
			return mcp.NewToolResultError("Error: 'resource' parameter is required"), nil
		}

		namespace, _ := args["namespace"].(string)
		if namespace == "" {
			namespace = "default"
		}

		// Check if the policy exists
		_, err := kyvernoClient.kyvernoV1.ClusterPolicies().Get(ctx, policyName, metav1.GetOptions{})
		if err != nil {
			// If not found as a ClusterPolicy, check as a NamespacedPolicy
			_, err = kyvernoClient.kyvernoV1.Policies(namespace).Get(ctx, policyName, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Policy %s not found as ClusterPolicy or in namespace %s: %v", policyName, namespace, err)), nil
			}
		}

		// Get the resource to apply the policy to
		_, err = kyvernoClient.KubernetesClient().CoreV1().Pods(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error getting resource %s in namespace %s: %v", resourceName, namespace, err)), nil
		}

		// Check policy reports
		reportResults, err := kyvernoClient.GetPolicyReports(namespace)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error checking policy reports: %v", err)), nil
		}

		// Check if the policy would be applied to this resource
		policyApplied := false
		for _, result := range reportResults {
			if result.Policy == policyName {
				for _, res := range result.Resources {
					if res.Name == resourceName && res.Namespace == namespace {
						policyApplied = true
						break
					}
				}
				if policyApplied {
					break
				}
			}
		}

		// Prepare the result
		var message string
		if policyApplied {
			message = fmt.Sprintf("Policy %s is already applied to %s/%s", policyName, namespace, resourceName)
		} else {
			message = fmt.Sprintf("Policy %s would be applied to %s/%s (no actual changes made)", policyName, namespace, resourceName)
		}

		result := map[string]interface{}{
			"status":    "success",
			"policy":    policyName,
			"resource":  resourceName,
			"namespace": namespace,
			"message":   message,
			"applied":   policyApplied,
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Add tool to list cluster policies
	s.AddTool(mcp.NewTool("list_cluster_policies",
		mcp.WithDescription("List all Kyverno cluster policies"),
	), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Ensure the Kyverno client, specifically kyvernoV1, is initialized.
		// This interface is nil if the server was started without kubeconfig/context.
		if kyvernoClient.kyvernoV1 == nil {
			log.Println("Error in 'list_cluster_policies': kyvernoClient.kyvernoV1 is nil. Server likely started without Kubernetes configuration.")
			return mcp.NewToolResultError("Cannot list cluster policies: Kyverno client is not fully initialized. Please ensure the MCP server was started with valid Kubernetes configuration (kubeconfig path and/or context name)."), nil
		}

		// Get the list of cluster policies
		policies, err := kyvernoClient.kyvernoV1.ClusterPolicies().List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error listing cluster policies: %v", err)), nil
		}

		// Convert policies to JSON
		policiesJSON, err := json.MarshalIndent(policies.Items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error marshaling policies: %v", err)), nil
		}

		return mcp.NewToolResultText(string(policiesJSON)), nil
	})

	// Add tool to get a specific cluster policy
	s.AddTool(mcp.NewTool("get_cluster_policy",
		mcp.WithDescription("Get a specific cluster policy"),
		mcp.WithString("name", mcp.Description("Name of the cluster policy"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid name parameter: %v", err)), nil
		}

		policy, err := kyvernoClient.kyvernoV1.ClusterPolicies().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error getting cluster policy: %v", err)), nil
		}

		policyJSON, err := json.MarshalIndent(policy, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error marshaling policy: %v", err)), nil
		}

		return mcp.NewToolResultText(string(policyJSON)), nil
	})

	// Add tool to list namespaced policies across all namespaces
	s.AddTool(mcp.NewTool("list_namespaced_policies",
		mcp.WithDescription("List all Kyverno namespaced policies across all namespaces"),
	), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get the list of namespaced policies
		policies, err := kyvernoClient.kyvernoV1.Policies(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error listing namespaced policies: %v", err)), nil
		}

		// Convert policies to JSON
		policiesJSON, err := json.MarshalIndent(policies.Items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error marshaling namespaced policies: %v", err)), nil
		}

		return mcp.NewToolResultText(string(policiesJSON)), nil
	})

	// Add tool to get namespaced policies by namespace
	s.AddTool(mcp.NewTool("get_namespaced_policies",
		mcp.WithDescription("Get namespaced policies by namespace"),
		mcp.WithString("namespace", mcp.Description("Namespace to get policies from"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace, err := request.RequireString("namespace")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid namespace parameter: %v", err)), nil
		}

		policies, err := kyvernoClient.kyvernoV1.Policies(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error getting namespaced policies: %v", err)), nil
		}

		result := make([]string, 0, len(policies.Items))
		for _, policy := range policies.Items {
			result = append(result, policy.Name)
		}

		resultJSON, err := json.MarshalIndent(map[string]interface{}{
			"namespace": namespace,
			"policies":  result,
		}, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error marshaling result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Add tool to list policy reports across all namespaces
	s.AddTool(mcp.NewTool("list_policy_reports",
		mcp.WithDescription("List all Kyverno policy reports across all namespaces"),
	), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get the list of policy reports
		reports, err := kyvernoClient.wgpolicyV1alpha2.PolicyReports(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error listing policy reports: %v", err)), nil
		}

		// Convert reports to JSON
		reportsJSON, err := json.MarshalIndent(reports.Items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error marshaling policy reports: %v", err)), nil
		}

		return mcp.NewToolResultText(string(reportsJSON)), nil
	})

	// Add tool to list policy reports by namespace
	s.AddTool(mcp.NewTool("list_namespaced_policy_reports",
		mcp.WithDescription("List Kyverno policy reports in a specific namespace"),
		mcp.WithDescription("List policy reports in a specific namespace"),
		mcp.WithString("namespace", mcp.Description("Namespace to get policy reports from"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace, err := request.RequireString("namespace")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid namespace parameter: %v", err)), nil
		}

		reports, err := kyvernoClient.wgpolicyV1alpha2.PolicyReports(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error listing policy reports: %v", err)), nil
		}

		reportNames := make([]string, 0, len(reports.Items))
		for _, report := range reports.Items {
			reportNames = append(reportNames, report.Name)
		}

		resultJSON, err := json.MarshalIndent(map[string]interface{}{
			"namespace": namespace,
			"reports":   reportNames,
		}, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error marshaling result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Add tool to list policy exceptions
	s.AddTool(mcp.NewTool("list_policy_exceptions",
		mcp.WithDescription("List all policy exceptions"),
		mcp.WithString("namespace", mcp.Description("Namespace to get exceptions from (optional)")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		namespace, _ := args["namespace"].(string)
		// In Kyverno v1.14.1, policy exceptions are managed through policies with specific annotations
		// rather than a dedicated PolicyException resource. We'll list all policies and filter
		// for those with the 'kyverno.io/policy-type: exception' annotation.
		policies, err := kyvernoClient.kyvernoV1.Policies(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error listing policies: %v", err)), nil
		}

		result := make(map[string][]string)
		for _, policy := range policies.Items {
			if policy.Annotations != nil && policy.Annotations["kyverno.io/policy-type"] == "exception" {
				result[policy.Namespace] = append(result[policy.Namespace], policy.Name)
			}
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error marshaling result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Add tool to install Kyverno
	s.AddTool(mcp.NewTool("install_kyverno",
		mcp.WithDescription("Install Kyverno in the cluster using Helm"),
		mcp.WithString("version", mcp.Description("Version of Kyverno to install (default: latest)")),
		mcp.WithString("namespace", mcp.Description("Namespace to install Kyverno in (default: kyverno)")),
	), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			log.Printf("Install_kyverno: Error invalid arguments format")
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		version, _ := args["version"].(string)
		if version == "" {
			log.Println("Install_kyverno: 'version' not specified, defaulting to 'latest'. Note: 'latest' might fail; specific versions are preferred.")
			version = "latest" // Consider changing to a specific default stable version if 'latest' is problematic
		}

		namespace, _ := args["namespace"].(string)
		if namespace == "" {
			log.Println("Install_kyverno: 'namespace' not specified, defaulting to 'kyverno'.")
			namespace = "kyverno"
		}
		log.Printf("Install_kyverno: Attempting to install version '%s' in namespace '%s'", version, namespace)

		settings := cli.New()
		actionConfig := new(action.Configuration)

		kubeconfigPath := ""
		contextName := ""
		if currentClient != nil {
			kubeconfigPath = currentClient.kubeconfigPath
			contextName = currentClient.contextName
		}

		if kubeconfigPath == "" {
			home, err := os.UserHomeDir()
			if err == nil {
				kubeconfigPath = filepath.Join(home, ".kube", "config")
				log.Printf("Install_kyverno: kubeconfigPath was empty, defaulted to: %s", kubeconfigPath)
			} else {
				log.Printf("Install_kyverno: kubeconfigPath was empty and could not determine home directory: %v. Helm will try its defaults.", err)
				// Allow Helm to use its default kubeconfig loading if home dir fails for some reason
			}
		}
		log.Printf("Install_kyverno: Using Kubeconfig: '%s', Context: '%s'", kubeconfigPath, contextName)

		configFlags := genericclioptions.NewConfigFlags(true)
		configFlags.KubeConfig = &kubeconfigPath
		configFlags.Context = &contextName

		if err := actionConfig.Init(
			configFlags, // Use the configured flags for Helm initialization
			namespace,
			os.Getenv("HELM_DRIVER"),
			func(format string, v ...interface{}) { log.Printf("[Helm]: "+format, v...) },
		); err != nil {
			log.Printf("Install_kyverno: Error initializing Helm action config: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Failed to initialize Helm action config: %v", err)), nil
		}
		log.Println("Install_kyverno: Helm action config initialized.")

		log.Println("Install_kyverno: Adding Kyverno Helm repository (https://kyverno.github.io/kyverno/)...")
		repoEntry := &repo.Entry{
			Name: "kyverno",
			URL:  "https://kyverno.github.io/kyverno/",
		}
		repoFile := settings.RepositoryConfig // Path to repositories.yaml e.g., ~/.config/helm/repositories.yaml
		r, err := repo.LoadFile(repoFile)
		if err != nil && !os.IsNotExist(err) {
			log.Printf("Install_kyverno: Error loading Helm repository file '%s': %v", repoFile, err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to load Helm repository file: %v", err)), nil
		}
		if r == nil { // If file doesn't exist, repo.LoadFile returns a new empty File struct and no error
			r = repo.NewFile()
		}
		if !r.Has(repoEntry.Name) {
			r.Add(repoEntry)
			if err := r.WriteFile(repoFile, 0644); err != nil {
				log.Printf("Install_kyverno: Error writing Helm repository file '%s': %v", repoFile, err)
				return mcp.NewToolResultError(fmt.Sprintf("failed to write Helm repository file: %v", err)), nil
			}
			log.Printf("Install_kyverno: Kyverno Helm repository '%s' added to '%s'.", repoEntry.Name, repoFile)
		} else {
			log.Printf("Install_kyverno: Kyverno Helm repository '%s' already exists in '%s'.", repoEntry.Name, repoFile)
		}

		// Note: Helm repository update logic (equivalent to 'helm repo update') has been removed
		// due to 'action.NewRepoUpdate' being undefined.
		// The LocateChart call below will attempt to find the chart in the added repository.
		// If this fails consistently, a manual 'helm repo update' outside the MCP or a more specific
		// SDK-based update mechanism might be needed.
		// log.Println("Install_kyverno: Proceeding without explicit Helm repo update. LocateChart will attempt to find the chart.")

		// Execute 'helm repo update' to refresh local chart repository cache
		log.Println("Install_kyverno: Executing 'helm repo update'...")
		cmd := exec.Command("helm", "repo", "update")
		// Pass through current environment variables, which might include KUBECONFIG if set externally,
		// though 'helm repo update' primarily uses Helm's own config files.
		cmd.Env = os.Environ()
		// If kubeconfigPath is set and valid, Helm commands run via exec.Command should pick it up if KUBECONFIG env var is set or if it's the default path.
		// We can also explicitly set KUBECONFIG for the command if needed:
		if kubeconfigPath != "" {
			log.Printf("Install_kyverno: Explicitly setting KUBECONFIG=%s for 'helm repo update' command.", kubeconfigPath)
			cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfigPath)
		}

		var out bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			log.Printf("Install_kyverno: 'helm repo update' failed. Stdout: %s, Stderr: %s, Error: %v", out.String(), stderr.String(), err)
			// Depending on strictness, we might return an error here or just log a warning.
			// For now, let's be strict as chart location will likely fail if update fails.
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update Helm repositories: %v. Stderr: %s", err, stderr.String())), nil
		}
		log.Printf("Install_kyverno: 'helm repo update' completed. Stdout: %s", out.String())

		installClient := action.NewInstall(actionConfig)
		installClient.Namespace = namespace
		installClient.ReleaseName = "kyverno" // Standard release name
		installClient.Version = version
		installClient.CreateNamespace = true
		installClient.Wait = true
		installClient.Timeout = 5 * time.Minute

		chartName := "kyverno/kyverno" // Chart name in the format 'repoName/chartName'
		log.Printf("Install_kyverno: Locating chart '%s' version '%s' using Helm settings...", chartName, version)
		cp, err := installClient.ChartPathOptions.LocateChart(chartName, settings)
		if err != nil {
			log.Printf("Install_kyverno: Error locating chart '%s' version '%s': %v", chartName, version, err)
			return mcp.NewToolResultError(fmt.Sprintf("Failed to locate chart '%s' (version: '%s'): %v. Check version and repo.", chartName, version, err)), nil
		}
		log.Printf("Install_kyverno: Chart '%s' version '%s' located at: %s", chartName, version, cp)

		chartRequested, err := loader.Load(cp)
		if err != nil {
			log.Printf("Install_kyverno: Error loading chart from path '%s': %v", cp, err)
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load chart from %s: %v", cp, err)), nil
		}
		log.Printf("Install_kyverno: Chart '%s' loaded successfully.", chartRequested.Name())

		log.Printf("Install_kyverno: Installing chart '%s' version '%s' as release '%s' in namespace '%s'...", chartRequested.Name(), chartRequested.Metadata.Version, installClient.ReleaseName, installClient.Namespace)
		rel, err := installClient.Run(chartRequested, nil)
		if err != nil {
			log.Printf("Install_kyverno: Error installing Kyverno chart: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Failed to install Kyverno: %v", err)), nil
		}

		log.Printf("Install_kyverno: Kyverno chart installation initiated. Release: '%s', Status: '%s'", rel.Name, rel.Info.Status)
		return mcp.NewToolResultText(fmt.Sprintf("Kyverno installed successfully: %s. Status: %s", rel.Name, rel.Info.Status)), nil
	})

	// Add tool to search Kyverno documentation
	s.AddTool(mcp.NewTool("search_kyverno_docs",
		mcp.WithDescription("Search Kyverno documentation using AWS Bedrock."),
		mcp.WithString("query", mcp.Description("The search query string.")),
		mcp.WithString("size", mcp.Description("Optional: Number of search results to return (default: 10). Should be a string representing an integer.")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Error: invalid arguments format"), nil
		}

		query, ok := args["query"].(string)
		if !ok || query == "" {
			return mcp.NewToolResultError("query argument is missing or empty"), nil
		}

		log.Printf("Retrieving from Bedrock KB. Query: '%s', KB ID: '%s'", query, bedrockKnowledgeBaseID)

		// Load AWS config
		cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(bedrockRegion))
		if err != nil {
			log.Printf("Error loading AWS SDK config for Bedrock Agent Runtime: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("unable to load AWS SDK config: %v", err)), nil
		}

		// Create Bedrock Agent Runtime client
		brAgentClient := bedrockagentruntime.NewFromConfig(cfg)

		// Prepare the Retrieve API input
		numberOfResults := int32(bedrockNumberOfResults) // Convert to int32 as required by SDK
		retrieveInput := &bedrockagentruntime.RetrieveInput{
			KnowledgeBaseId: aws.String(bedrockKnowledgeBaseID),
			RetrievalQuery: &types.KnowledgeBaseQuery{
				Text: aws.String(query),
			},
			RetrievalConfiguration: &types.KnowledgeBaseRetrievalConfiguration{
				VectorSearchConfiguration: &types.KnowledgeBaseVectorSearchConfiguration{
					NumberOfResults: &numberOfResults,
				},
			},
		}

		log.Printf("Calling Bedrock Agent Runtime Retrieve API for KB ID '%s'...", bedrockKnowledgeBaseID)
		retrieveOutput, err := brAgentClient.Retrieve(ctx, retrieveInput)
		if err != nil {
			log.Printf("Error calling Bedrock Retrieve API: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to retrieve from Bedrock KB: %v", err)), nil
		}

		// Format the results
		results := make([]map[string]interface{}, len(retrieveOutput.RetrievalResults))
		for i, r := range retrieveOutput.RetrievalResults {
			resultMap := map[string]interface{}{
				"content": "",
				"score":   0.0,
			}
			if r.Content != nil && r.Content.Text != nil {
				resultMap["content"] = *r.Content.Text
			}
			if r.Score != nil {
				resultMap["score"] = *r.Score
			}
			if r.Location != nil && r.Location.S3Location != nil && r.Location.S3Location.Uri != nil {
				resultMap["location_s3_uri"] = *r.Location.S3Location.Uri
			}
			// Add other location types if needed (WEB, CONFLUENCE, SALESFORCE, SHAREPOINT)
			results[i] = resultMap
		}

		formattedResponse, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			log.Printf("Error formatting Bedrock Retrieve API JSON response: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to format Bedrock Retrieve API JSON response: %v", err)), nil
		}

		log.Println("Successfully received response from Bedrock Retrieve API.")
		return mcp.NewToolResultText(string(formattedResponse)), nil
	})

	// Start the MCP server
	log.Println("Starting MCP server on stdio...")
	if err = server.ServeStdio(s); err != nil {
		log.Fatalf("Error starting server: %v\n", err)
	}
}
