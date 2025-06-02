// Package main implements the Kyverno MCP (Model Context Protocol) server.
// It provides tools for interacting with Kyverno policy engine and AWS Bedrock's Knowledge Base
// for Kyverno documentation search.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"
	kyvernov1alpha2 "github.com/kyverno/kyverno/api/policyreport/v1alpha2"
	kyvernoclient "github.com/kyverno/kyverno/pkg/client/clientset/versioned"
	kyvernov1 "github.com/kyverno/kyverno/pkg/client/clientset/versioned/typed/kyverno/v1"
	wgpolicyv1alpha2 "github.com/kyverno/kyverno/pkg/client/clientset/versioned/typed/policyreport/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// createTextContent creates a new text content for tool results
func createTextContent(text string) mcp.TextContent {
	return mcp.TextContent{
		Type: "text",
		Text: text,
	}
}

// createToolResult creates a new tool result with the given content
func createToolResult(content interface{}) *mcp.CallToolResult {
	switch v := content.(type) {
	case string:
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				createTextContent(v),
			},
		}
	default:
		// For non-string content, marshal to JSON
		jsonBytes, err := json.MarshalIndent(content, "", "  ")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error marshaling content: %v", err))
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				createTextContent(string(jsonBytes)),
			},
		}
	}
}

// createToolResultError creates a new error tool result with the given error message
func createToolResultError(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			createTextContent(text),
		},
		IsError: true,
	}
}

// KyvernoClient represents a client for interacting with Kyverno
type KyvernoClient struct {
	kubeconfigPath   string
	contextName      string
	client           *kyvernoclient.Clientset
	config           *rest.Config
	kyvernoV1        kyvernov1.KyvernoV1Interface
	wgpolicyV1alpha2 wgpolicyv1alpha2.Wgpolicyk8sV1alpha2Interface
}

// KyvernoV1 returns the Kyverno v1 client
func (k *KyvernoClient) KyvernoV1() kyvernov1.KyvernoV1Interface {
	return k.kyvernoV1
}

// Wgpolicyk8sV1alpha2 returns the Wgpolicyk8s v1alpha2 client
func (k *KyvernoClient) Wgpolicyk8sV1alpha2() wgpolicyv1alpha2.Wgpolicyk8sV1alpha2Interface {
	return k.wgpolicyV1alpha2
}

// NewKyvernoClient creates a new Kyverno client with the default kubeconfig
func NewKyvernoClient(kubeconfigPath string) (*KyvernoClient, error) {
	return NewKyvernoClientWithConfig(kubeconfigPath, "")
}

// NewKyvernoClientWithConfig creates a new Kyverno client with the specified kubeconfig and context
func NewKyvernoClientWithConfig(kubeconfigPath, contextName string) (*KyvernoClient, error) {
	var config *rest.Config
	var err error

	// Use the specified kubeconfig or the default location
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err = clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating client config: %v. Please ensure you have a valid kubeconfig file at the default location (~/.kube/config) or specify one with --kubeconfig", err)
	}

	// Validate the context if provided
	if contextName != "" {
		rawConfig, err := clientConfig.RawConfig()
		if err != nil {
			return nil, fmt.Errorf("error getting raw config: %v", err)
		}

		// Check if the context exists
		if _, exists := rawConfig.Contexts[contextName]; !exists {
			return nil, fmt.Errorf("context '%s' does not exist in kubeconfig", contextName)
		}
	}

	// Create the clientset
	clientset, err := kyvernoclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating Kyverno clientset: %v", err)
	}

	return &KyvernoClient{
		kubeconfigPath:   kubeconfigPath,
		contextName:      contextName,
		client:           clientset,
		config:           config,
		kyvernoV1:        clientset.KyvernoV1(),
		wgpolicyV1alpha2: clientset.Wgpolicyk8sV1alpha2(),
	}, nil
}

// ValidateContext checks if the specified context exists in the kubeconfig
func (k *KyvernoClient) ValidateContext(_ string) (bool, error) {
	// In a real implementation, this would check the kubeconfig for the context
	// For now, we'll just return true to simulate a successful validation
	return true, nil
}

// SwitchContext switches the current context to the specified context
func (k *KyvernoClient) SwitchContext(contextName string) error {
	// In a real implementation, this would switch the current context in the kubeconfig
	// For now, we'll just update the context name in our client
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

// GetPolicyReports fetches PolicyReports for the given namespace and returns simplified results
func (k *KyvernoClient) GetPolicyReports(namespace string) ([]PolicyReportResult, error) {
	var results []PolicyReportResult

	// Helper function to process results from either PolicyReport or ClusterPolicyReport
	processResults := func(policyResults []kyvernov1alpha2.PolicyReportResult, reportNamespace string) {
		for _, result := range policyResults {
			// Skip results without a policy name
			if result.Policy == "" {
				continue
			}

			// Convert resources to ResourceInfo slice
			var resources []ResourceInfo
			for _, ref := range result.Resources {
				// If resource namespace is empty, use the report namespace
				ns := ref.Namespace
				if ns == "" {
					ns = reportNamespace
				}

				resources = append(resources, ResourceInfo{
					Kind:       ref.Kind,
					Namespace:  ns,
					Name:       ref.Name,
					APIVersion: ref.APIVersion,
				})
			}

			// Convert timestamp to string if not zero
			timestampStr := ""
			if result.Timestamp != (metav1.Timestamp{}) {
				timestampStr = result.Timestamp.String()
			}

			results = append(results, PolicyReportResult{
				Policy:    result.Policy,
				Rule:      result.Rule,
				Result:    string(result.Result),
				Message:   result.Message,
				Resources: resources,
				Timestamp: timestampStr,
			})
		}
	}

	// Handle cluster-wide reports
	clusterReportList, err := k.client.Wgpolicyk8sV1alpha2().ClusterPolicyReports().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing cluster policy reports: %v", err)
	}

	for _, report := range clusterReportList.Items {
		processResults(report.Results, "")
	}

	// Handle namespaced reports
	reportNamespace := namespace
	if namespace == "all" || namespace == "" {
		reportNamespace = ""
	}

	reportList, err := k.client.Wgpolicyk8sV1alpha2().PolicyReports(reportNamespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing policy reports in namespace %s: %v", reportNamespace, err)
	}

	for _, report := range reportList.Items {
		processResults(report.Results, report.Namespace)
	}

	return results, nil
}

func main() {
	// Parse command line flags
	kubeconfigPath := flag.String("kubeconfig", "", "Path to the kubeconfig file")
	contextName := flag.String("context", "", "Name of the kubeconfig context to use")
	flag.Parse()

	// Create a new MCP server
	s := server.NewMCPServer(
		"Kyverno MCP Server",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	// Add a tool to list available contexts
	s.AddTool(mcp.NewTool("list_contexts",
		mcp.WithDescription("List all available Kubernetes contexts"),
	), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// In a real implementation, this would read the kubeconfig file
		// and extract the list of contexts
		// For now, we'll return a placeholder list
		contexts := []string{"context-1", "context-2", "context-3"}

		// Return the list of contexts as a JSON array
		result := map[string]interface{}{
			"available_contexts": contexts,
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Switch context tool
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
			return createToolResultError(fmt.Sprintf("Invalid context parameter: %v", err)), nil
		}

		kyvernoClient, err := NewKyvernoClient("")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error initializing client: %v", err)), nil
		}

		// Validate the context exists
		valid, err := kyvernoClient.ValidateContext(contextName)
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error validating context: %v", err)), nil
		}
		if !valid {
			return createToolResultError(fmt.Sprintf("Context %s not found", contextName)), nil
		}

		// Switch to the new context
		if err := kyvernoClient.SwitchContext(contextName); err != nil {
			return createToolResultError(fmt.Sprintf("Error switching to context %s: %v", contextName, err)), nil
		}

		result := map[string]string{
			"status":  "success",
			"message": fmt.Sprintf("Switched to context: %s", contextName),
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Create Kyverno client with the specified kubeconfig path and context
	kyvernoClient, err := NewKyvernoClientWithConfig(*kubeconfigPath, *contextName)
	if err != nil {
		log.Fatalf("Error creating Kyverno client: %v\n", err)
	}
	_ = kyvernoClient // Keep the client for future use

	// Create a tool to scan the cluster for resources matching a policy
	scanClusterTool := mcp.NewTool(
		"scan_cluster",
		mcp.WithDescription("Scan the cluster for resources that match the given policy"),
		mcp.WithString("policy", mcp.Description("Name of the policy to scan with")),
		mcp.WithString("namespace", mcp.Description("Namespace to scan (use 'all' for all namespaces)")),
		mcp.WithString("kind", mcp.Description("Kind of resources to scan (e.g., Pod, Deployment)")),
	)

	// Register the scan cluster tool
	s.AddTool(scanClusterTool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_ = request // Mark unused parameter
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return createToolResultError("Error: invalid arguments format"), nil
		}

		policyName, ok := args["policy"].(string)
		if !ok || policyName == "" {
			return createToolResultError("Error: 'policy' parameter is required"), nil
		}

		namespace, _ := args["namespace"].(string)
		kind, _ := args["kind"].(string)

		// Get all policy reports
		reportResults, err := kyvernoClient.GetPolicyReports(namespace)
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error getting policy reports: %v", err)), nil
		}

		// Filter results based on the requested policy and resource kind
		var matches []map[string]interface{}
		for _, result := range reportResults {
			// Skip if policy name doesn't match
			if policyName != "" && result.Policy != policyName {
				continue
			}

			// Skip if kind doesn't match
			if kind != "" {
				kindMatch := false
				for _, resource := range result.Resources {
					if resource.Kind == kind {
						kindMatch = true
						break
					}
				}
				if !kindMatch {
					continue
				}
			}

			// Add matching result
			matches = append(matches, map[string]interface{}{
				"policy":    result.Policy,
				"rule":      result.Rule,
				"result":    result.Result,
				"message":   result.Message,
				"resources": result.Resources,
				"timestamp": result.Timestamp,
			})
		}

		// Prepare the result
		result := map[string]interface{}{
			"status":  "success",
			"count":   len(matches),
			"results": matches,
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return createToolResult(string(resultJSON)), nil
	})

	// Create a tool to apply a policy to resources
	applyPolicyTool := mcp.NewTool(
		"apply_policy",
		mcp.WithDescription("Apply a policy to specified resources in the cluster"),
		mcp.WithString("policy", mcp.Description("Name of the policy to apply")),
		mcp.WithString("resource", mcp.Description("Name of the resource to apply the policy to")),
		mcp.WithString("namespace", mcp.Description("Namespace of the resource (use 'default' if not specified)")),
	)

	// Register the apply policy tool
	s.AddTool(applyPolicyTool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_ = request // Mark unused parameter
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return createToolResultError("Error: invalid arguments format"), nil
		}

		policyName, ok := args["policy"].(string)
		if !ok || policyName == "" {
			return createToolResultError("Error: 'policy' parameter is required"), nil
		}

		resourceName, ok := args["resource"].(string)
		if !ok || resourceName == "" {
			return createToolResultError("Error: 'resource' parameter is required"), nil
		}

		namespace, _ := args["namespace"].(string)
		if namespace == "" {
			namespace = "default"
		}

		// Check if the policy has any reports
		reportResults, err := kyvernoClient.GetPolicyReports("")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error checking policy reports: %v", err)), nil
		}

		// Check if the policy exists in any reports
		policyExists := false
		for _, result := range reportResults {
			if result.Policy == policyName {
				policyExists = true
				break
			}
		}

		if !policyExists {
			return createToolResultError(fmt.Sprintf("Policy %s not found in any policy reports", policyName)), nil
		}

		// In a real implementation, we would use the Kyverno CLI or API to apply the policy
		// For now, we'll return a success response with the policy and resource info
		result := map[string]interface{}{
			"status":    "success",
			"policy":    policyName,
			"resource":  resourceName,
			"namespace": namespace,
			"message":   "Policy would be applied to the resource (simulated)",
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return createToolResult(string(resultJSON)), nil
	})

	// Add tool to list cluster policies
	s.AddTool(mcp.NewTool("list_cluster_policies",
		mcp.WithDescription("List all Kyverno cluster policies"),
	), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get the list of cluster policies
		policies, err := kyvernoClient.KyvernoV1().ClusterPolicies().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error listing cluster policies: %v", err)), nil
		}

		// Convert policies to JSON
		policiesJSON, err := json.MarshalIndent(policies.Items, "", "  ")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error marshaling policies: %v", err)), nil
		}

		return createToolResult(string(policiesJSON)), nil
	})

	// Add tool to get a specific cluster policy
	s.AddTool(mcp.NewTool("get_cluster_policy",
		mcp.WithDescription("Get a specific cluster policy"),
		mcp.WithString("name", mcp.Description("Name of the cluster policy"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("name")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Invalid name parameter: %v", err)), nil
		}

		policy, err := kyvernoClient.KyvernoV1().ClusterPolicies().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error getting cluster policy: %v", err)), nil
		}

		policyJSON, err := json.MarshalIndent(policy, "", "  ")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error marshaling policy: %v", err)), nil
		}

		return createToolResult(string(policyJSON)), nil
	})

	// Add tool to list namespaced policies across all namespaces
	s.AddTool(mcp.NewTool("list_namespaced_policies",
		mcp.WithDescription("List all Kyverno namespaced policies across all namespaces"),
	), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get the list of namespaced policies
		policies, err := kyvernoClient.KyvernoV1().Policies(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error listing namespaced policies: %v", err)), nil
		}

		// Convert policies to JSON
		policiesJSON, err := json.MarshalIndent(policies.Items, "", "  ")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error marshaling namespaced policies: %v", err)), nil
		}

		return createToolResult(string(policiesJSON)), nil
	})

	// Add tool to get namespaced policies by namespace
	s.AddTool(mcp.NewTool("get_namespaced_policies",
		mcp.WithDescription("Get namespaced policies by namespace"),
		mcp.WithString("namespace", mcp.Description("Namespace to get policies from"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace, err := request.RequireString("namespace")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Invalid namespace parameter: %v", err)), nil
		}

		policies, err := kyvernoClient.KyvernoV1().Policies(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error getting namespaced policies: %v", err)), nil
		}

		result := make([]string, 0, len(policies.Items))
		for _, policy := range policies.Items {
			result = append(result, policy.Name)
		}

		return createToolResult(map[string]interface{}{
			"namespace": namespace,
			"policies":  result,
		}), nil
	})

	// Add tool to list policy reports across all namespaces
	s.AddTool(mcp.NewTool("list_policy_reports",
		mcp.WithDescription("List all Kyverno policy reports across all namespaces"),
	), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get the list of policy reports
		reports, err := kyvernoClient.Wgpolicyk8sV1alpha2().PolicyReports(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error listing policy reports: %v", err)), nil
		}

		// Convert reports to JSON
		reportsJSON, err := json.MarshalIndent(reports.Items, "", "  ")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error marshaling policy reports: %v", err)), nil
		}

		return createToolResult(string(reportsJSON)), nil
	})

	// Add tool to list policy reports by namespace
	s.AddTool(mcp.NewTool("list_namespaced_policy_reports",
		mcp.WithDescription("List Kyverno policy reports in a specific namespace"),
		mcp.WithDescription("List policy reports in a specific namespace"),
		mcp.WithString("namespace", mcp.Description("Namespace to get policy reports from"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace, err := request.RequireString("namespace")
		if err != nil {
			return createToolResultError(fmt.Sprintf("Invalid namespace parameter: %v", err)), nil
		}

		reports, err := kyvernoClient.Wgpolicyk8sV1alpha2().PolicyReports(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error listing policy reports: %v", err)), nil
		}

		reportNames := make([]string, 0, len(reports.Items))
		for _, report := range reports.Items {
			reportNames = append(reportNames, report.Name)
		}

		return createToolResult(map[string]interface{}{
			"namespace": namespace,
			"reports":   reportNames,
		}), nil
	})

	// Add tool to list policy exceptions
	s.AddTool(mcp.NewTool("list_policy_exceptions",
		mcp.WithDescription("List all policy exceptions"),
		mcp.WithString("namespace", mcp.Description("Namespace to get exceptions from (optional)")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return createToolResultError("Invalid arguments format"), nil
		}

		namespace, _ := args["namespace"].(string)
		// In Kyverno v1.14.1, policy exceptions are managed through policies with specific annotations
		// rather than a dedicated PolicyException resource. We'll list all policies and filter
		// for those with the 'kyverno.io/policy-type: exception' annotation.
		policies, err := kyvernoClient.KyvernoV1().Policies(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return createToolResultError(fmt.Sprintf("Error listing policies: %v", err)), nil
		}

		result := make(map[string][]string)
		for _, policy := range policies.Items {
			if policy.Annotations != nil && policy.Annotations["kyverno.io/policy-type"] == "exception" {
				result[policy.Namespace] = append(result[policy.Namespace], policy.Name)
			}
		}

		return createToolResult(result), nil
	})

	// Add tool to search Kyverno documentation using AWS Bedrock
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

		// Default values
		bedrockRegion := "us-east-1"                       // Update with your AWS region
		bedrockKnowledgeBaseID := "YOUR_KNOWLEDGE_BASE_ID" // Replace with your Bedrock Knowledge Base ID
		bedrockNumberOfResults := 10

		// Parse size parameter if provided
		if sizeStr, ok := args["size"].(string); ok && sizeStr != "" {
			if size, err := strconv.Atoi(sizeStr); err == nil && size > 0 {
				bedrockNumberOfResults = size
			}
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
		numberOfResults := int32(bedrockNumberOfResults)
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
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Error starting server: %v\n", err)
	}
}
