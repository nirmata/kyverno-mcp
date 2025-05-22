# Kyverno MCP Server Architecture

## 1. Overview
This document outlines the architecture for a Management Control Plane (MCP) server for Kyverno. The primary goal of this server is to expose Kyverno's policy application capabilities over MCP, allowing clients to apply Kyverno policies to a given set of Kubernetes resources and receive the results. This is analogous to the Kyverno CLI's apply command.

The server will be implemented in Golang, utilizing the mark3labs/mcp-go library for MCP communication.

## 2. Goals
- Provide an MCP endpoint to apply Kyverno policies to Kubernetes resources.
- Support loading policies from various sources (e.g., local file paths, potentially URLs in the future).
- Support loading Kubernetes resources exclusively via the Kubernetes API.
- The server must be able to run in-cluster (authenticating via a service account).
- The server must be able to run out-of-cluster (authenticating via a kubeconfig file specified by a parameter).
- Allow specification of common kyverno apply parameters like valuesFile, userInfo, etc.
- Return detailed results of policy application, including validations, mutations, and errors.
- Leverage Kyverno's core libraries for policy evaluation.

## 3. Non-Goals (for initial version)
- Full real-time policy enforcement or admission control (this is the domain of the main Kyverno deployment).
- Loading Kubernetes resources directly from local files or URLs via the MCP request. (Resources are always fetched from the K8s API based on query parameters).
- Support for all Kyverno CLI features initially (e.g., complex Git interactions, interactive prompts).
- Advanced UI or dashboard (the focus is on the MCP backend).
- Generating policies (focus is on applying existing policies).

## 4. High-Level Architecture Diagram
```
+-------------------+      MCP (ApplyRequest)      +------------------------+
|    MCP Client     | <--------------------------> |  Kyverno MCP Server    |
| (e.g., CLI, UI)   |                              |  (Golang, mcp-go)      |
+-------------------+      MCP (ApplyResponse)     +------------------------+
                                                         |         ^
                                                         | Uses    | Interacts with
                                                         V         |
                                           +---------------------------+  K8s API
                                           |   Kyverno Core Libraries  | <---------> Kubernetes Cluster
                                           | (Policy Engine, Loaders)  |
                                           +---------------------------+
```

## 5. File and Folder Structure
A suggested project structure:

```
kyverno-mcp-server/
├── cmd/
│   └── kyverno-mcp-server/
│       └── main.go                 // Entry point, server initialization, DI, CLI flags (e.g., --kubeconfig)
├── pkg/
│   ├── mcp/                        // MCP specific components
│   │   ├── server.go               // MCP server setup, lifecycle management using mcp-go
│   │   ├── handler.go              // Core MCP message handler registration and dispatching
│   │   ├── types/                  // MCP message type definitions (Go structs)
│   │   │   └── apply.go            // Defines ApplyRequest and ApplyResponse
│   │   └── apply_handler.go        // Specific handler implementation for ApplyRequest
│   │
│   ├── kyverno/                    // Kyverno interaction logic
│   │   ├── engine.go               // Interface and implementation for Kyverno's apply logic
│   │   ├── policy_loader.go        // Logic to load/fetch policies from various sources
│   │   └── resource_loader.go      // Logic to load/fetch resources from the Kubernetes API
│   │
│   ├── config/
│   │   └── config.go               // Configuration management (server settings, paths, etc.)
│   │
│   └── utils/                      // Common utility functions
│       ├── fs.go                   // Filesystem utilities (e.g., for UserInfoPath)
│       └── k8s.go                  // Kubernetes client setup and API interaction utilities
│
├── internal/                         // (Optional) Private application and library code not intended for external use
│   └── ...
│
├── api/                              // (Optional) If using Protobuf for MCP message definitions
│   └── v1/
│       └── apply.proto             // .proto definition for ApplyRequest/ApplyResponse
│
├── go.mod
├── go.sum
└── README.md
```

## 6. Key Packages and Interfaces

### 6.1. pkg/mcp/types/apply.go
Defines the Go structs for MCP messages.

```go
package types

import (
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResourceQuery defines how to select resources from the Kubernetes API.
type ResourceQuery struct {
    APIVersion    string            `json:"apiVersion"`              // e.g., "apps/v1", "v1"
    Kind          string            `json:"kind"`                    // e.g., "Deployment", "Pod", "ConfigMap"
    Namespace     string            `json:"namespace,omitempty"`     // Specific namespace. If empty for namespaced kinds, implies all namespaces permitted by RBAC. For cluster-scoped kinds, this should be empty.
    Name          string            `json:"name,omitempty"`          // Specific resource name. If empty, selects all resources matching other criteria.
    LabelSelector string            `json:"labelSelector,omitempty"` // e.g., "app=my-app,env=prod"
}

// ApplyRequest mirrors the parameters for a Kyverno apply command,
// adapted for MCP and Kubernetes API-based resource loading.
type ApplyRequest struct {
    RequestID         string            `json:"requestID,omitempty"`
    PolicyPaths       []string          `json:"policyPaths,omitempty"`       // Paths/URLs to policy YAMLs (loaded by MCP server)
    ResourceQueries   []ResourceQuery   `json:"resourceQueries"`           // Specifies which resources to fetch from the K8s API
    KubeconfigPath    string            `json:"kubeconfigPath,omitempty"`  // Path to kubeconfig file (for out-of-cluster mode, typically passed by client if server allows overriding its own config)
    UserInfoPath      string            `json:"userInfoPath,omitempty"`      // Path to user info YAML
    ValuesFileContent string            `json:"valuesFileContent,omitempty"` // YAML content of values file
    Mutate            bool              `json:"mutate,omitempty"`
    Validate          bool              `json:"validate,omitempty"`
    ContextEntries    map[string]string `json:"contextEntries,omitempty"`    // Additional context entries
}

// ApplyResponse contains the results of applying policies.
type ApplyResponse struct {
    RequestID        string                      `json:"requestID,omitempty"`
    Results          []PolicyApplicationResult   `json:"results,omitempty"`
    Error            string                      `json:"error,omitempty"`
}

// PolicyApplicationResult defines the structure for a single policy application outcome.
type PolicyApplicationResult struct {
    PolicyName              string       `json:"policyName"`
    Resource                ResourceInfo `json:"resource"`
    Rules                   []RuleResult `json:"rules"`
    ValidationFailureAction string       `json:"validationFailureAction,omitempty"`
    PatchedResource         string       `json:"patchedResource,omitempty"` // YAML string of the resource if mutated
}

// ResourceInfo identifies a Kubernetes resource.
type ResourceInfo struct {
    APIVersion string `json:"apiVersion"`
    Kind       string `json:"kind"`
    Namespace  string `json:"namespace,omitempty"`
    Name       string `json:"name"`
}

// RuleResult describes the outcome of a single rule application.
type RuleResult struct {
    Name    string `json:"name"`
    Type    string `json:"type"`
    Message string `json:"message"`
    Status  string `json:"status"`
}
```

### 6.2. pkg/mcp/server.go
(No change in description from previous versions)

### 6.3. pkg/mcp/handler.go (or apply_handler.go)
(No change in Go code structure from previous versions, but the req *types.ApplyRequest will have the updated fields)

### 6.4. pkg/kyverno/engine.go
Interface and implementation for the core Kyverno logic.

```go
package kyverno

import (
    "context"
    "your_project_path/pkg/mcp/types"
    // Kyverno imports
    // kyvernov1 "[github.com/kyverno/kyverno/api/kyverno/v1](https://github.com/kyverno/kyverno/api/kyverno/v1)"
)

// Engine defines the interface for applying Kyverno policies.
type Engine interface {
    ApplyPolicies(ctx context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error)
}

// kyvernoEngine implements the Engine interface.
type kyvernoEngine struct {
    policyLoader   PolicyLoader
    resourceLoader ResourceLoader
    // Default kubeconfig path for the engine, can be overridden by req.KubeconfigPath
    // This might be set at engine initialization from a server-wide config or CLI flag.
    serverKubeconfigPath string
}

// NewEngine creates a new Kyverno engine.
func NewKyvernoEngine(pl PolicyLoader, rl ResourceLoader, serverKubeconfigPath string) Engine {
    return &kyvernoEngine{
        policyLoader:         pl,
        resourceLoader:       rl,
        serverKubeconfigPath: serverKubeconfigPath,
    }
}

// ApplyPolicies orchestrates the policy application process.
func (ke *kyvernoEngine) ApplyPolicies(ctx context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error) {
    // TODO: Implement the logic:
    // 1. Load policies using ke.policyLoader based on req.PolicyPaths.
    //
    // 2. Determine Kubeconfig: Use req.KubeconfigPath if provided, otherwise use ke.serverKubeconfigPath.
    //    This allows a client request to specify a kubeconfig, or the server uses its default.
    //    If both are empty, it implies in-cluster configuration.
    kubeconfigForLoad := req.KubeconfigPath
    if kubeconfigForLoad == "" {
        kubeconfigForLoad = ke.serverKubeconfigPath
    }
    //
    // 3. Load resources using ke.resourceLoader based on req.ResourceQueries and the determined kubeconfigForLoad.
    //    - This involves using a Kubernetes dynamic client.
    //    - Handle errors (API connection, parsing GVK, resources not found, RBAC issues).
    //
    // 4. Load UserInfo if req.UserInfoPath is provided.
    //
    // 5. Prepare Kyverno engine context.
    //
    // 6. Invoke Kyverno's core policy engine.
    //
    // 7. Transform Kyverno's `engine.PolicyResponse` into `types.PolicyApplicationResult`.
    //
    // 8. Construct the final `types.ApplyResponse`.

    resp := &types.ApplyResponse{RequestID: req.RequestID}

    // policies, err := ke.policyLoader.Load(req.PolicyPaths, "") // Assuming no gitBranch for now
    // if err != nil { ... }

    // resources, err := ke.resourceLoader.Load(ctx, req.ResourceQueries, kubeconfigForLoad)
    // if err != nil { ... }

    // ... rest of the logic ...
    return resp, nil
}
```

### 6.5. pkg/kyverno/policy_loader.go
(No change in description or Go code structure from the original version focusing on local file paths for policies. Git/Kustomize support would be future enhancements as per previous iterations if desired.)

```go
package kyverno

import (
    kyvernov1 "[github.com/kyverno/kyverno/api/kyverno/v1](https://github.com/kyverno/kyverno/api/kyverno/v1)"
    // "[github.com/kyverno/kyverno/cmd/cli/kubectl-kyverno/utils](https://github.com/kyverno/kyverno/cmd/cli/kubectl-kyverno/utils)"
)

type PolicyLoader interface {
    Load(policyPaths []string, gitBranch string) ([]kyvernov1.PolicyInterface, error)
}

type localPolicyLoader struct{}

func NewLocalPolicyLoader() PolicyLoader {
    return &localPolicyLoader{}
}

func (l *localPolicyLoader) Load(policyPaths []string, gitBranch string) ([]kyvernov1.PolicyInterface, error) {
    // TODO: Implement logic as per original design (local files/dirs for policies)
    var policies []kyvernov1.PolicyInterface
    return policies, nil
}
```

### 6.6. pkg/kyverno/resource_loader.go
```go
package kyverno

import (
    "context"
    "fmt"
    "your_project_path/pkg/mcp/types"
    "your_project_path/pkg/utils/k8s" // For K8s client setup

    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/client-go/dynamic"
)

// ResourceLoader defines the interface for loading Kubernetes resources from the API.
type ResourceLoader interface {
    // Load fetches resources from the Kubernetes API based on the provided queries.
    // kubeconfigPath is used for out-of-cluster, if empty, in-cluster config is assumed.
    Load(ctx context.Context, queries []types.ResourceQuery, kubeconfigPath string) ([]*unstructured.Unstructured, error)
}

// apiResourceLoader implements ResourceLoader for Kubernetes API.
type apiResourceLoader struct{}

// NewAPIResourceLoader creates a new API resource loader.
func NewAPIResourceLoader() ResourceLoader {
    return &apiResourceLoader{}
}

func (arl *apiResourceLoader) Load(ctx context.Context, queries []types.ResourceQuery, kubeconfigPath string) ([]*unstructured.Unstructured, error) {
    var allResources []*unstructured.Unstructured

    // 1. Get Kubernetes dynamic client
    //    utils.k8s.GetDynamicClient(kubeconfigPath) should handle in-cluster vs out-of-cluster.
    dynamicClient, err := k8s.GetDynamicClient(kubeconfigPath)
    if err != nil {
        return nil, fmt.Errorf("failed to get Kubernetes dynamic client: %w", err)
    }

    for _, query := range queries {
        gvr, err := k8s.GVKToGVR(query.APIVersion, query.Kind) // Utility to map GVK to GVR
        if err != nil {
            return nil, fmt.Errorf("failed to map GVK to GVR for %s/%s: %w", query.APIVersion, query.Kind, err)
        }

        var resourceInterface dynamic.ResourceInterface
        if query.Namespace != "" {
            resourceInterface = dynamicClient.Resource(gvr).Namespace(query.Namespace)
        } else {
            // For cluster-scoped resources or when querying all namespaces (requires appropriate RBAC)
            resourceInterface = dynamicClient.Resource(gvr)
        }

        if query.Name != "" {
            // Fetch a single named resource
            resource, err := resourceInterface.Get(ctx, query.Name, metav1.GetOptions{})
            if err != nil {
                // Consider how to handle "NotFound" errors - skip or error out?
                // For now, let's error out if a specific resource is not found.
                return nil, fmt.Errorf("failed to get resource %s/%s (Kind: %s, APIVersion: %s): %w", query.Namespace, query.Name, query.Kind, query.APIVersion, err)
            }
            allResources = append(allResources, resource)
        } else {
            // List resources based on label selector
            listOptions := metav1.ListOptions{}
            if query.LabelSelector != "" {
                listOptions.LabelSelector = query.LabelSelector
            }
            resourceList, err := resourceInterface.List(ctx, listOptions)
            if err != nil {
                return nil, fmt.Errorf("failed to list resources for GVR %s (Namespace: '%s', Selector: '%s'): %w", gvr.String(), query.Namespace, query.LabelSelector, err)
            }
            for i := range resourceList.Items {
                allResources = append(allResources, &resourceList.Items[i])
            }
        }
    }

    return allResources, nil
}
```

### 6.7. pkg/utils/k8s.go (Conceptual Content)
```go
package k8s

import (
    "k8s.io/client-go/dynamic"
    "k8s.io/client-go/kubernetes" // For RESTMapper
    "k8s.io/client-go/restmapper" // For GVK to GVR mapping
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/apimachinery/pkg/runtime/schema"
    // "sigs.k8s.io/controller-runtime/pkg/client/config" // Alternative for getting config
)

// GetDynamicClient returns a dynamic Kubernetes client.
// If kubeconfigPath is empty, it attempts to use in-cluster configuration.
func GetDynamicClient(kubeconfigPath string) (dynamic.Interface, error) {
    // TODO: Implement logic:
    // 1. Load *rest.Config:
    //    If kubeconfigPath is provided, use clientcmd.BuildConfigFromFlags("", kubeconfigPath).
    //    Else, use rest.InClusterConfig().
    // 2. Create dynamic client: dynamic.NewForConfig(config).
    // Handle errors.
    return nil, nil // Placeholder
}

// GetRESTMapper returns a RESTMapper.
func GetRESTMapper(kubeconfigPath string) (meta.RESTMapper, error) {
    // TODO: Implement logic similar to GetDynamicClient for config, then:
    // 1. Create a discovery client: discovery.NewDiscoveryClientForConfig(config)
    // 2. Use restmapper.NewDeferredDiscoveryRESTMapper(memory걍discovery.NewMemCacheClient(discoveryClient))
    // Or for a simpler approach if only a few GVKs are known, use a manually populated mapper.
    // However, a dynamic mapper is more robust.
    // A kubernetes.Clientset can also provide a discovery client.
    // config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath) // or InClusterConfig()
    // clientset, err := kubernetes.NewForConfig(config)
    // discoveryClient := clientset.Discovery()
    // return restmapper.NewDeferredDiscoveryRESTMapper(memorydiscovery.NewMemCacheClient(discoveryClient)), nil
    return nil, nil // Placeholder
}

// GVKToGVR maps GroupVersionKind to GroupVersionResource.
// This requires a RESTMapper.
func GVKToGVR(apiVersion, kind string) (schema.GroupVersionResource, error) {
    // TODO: Implement logic:
    // 1. Parse apiVersion into Group and Version.
    // 2. Create schema.GroupVersionKind.
    // 3. Get a RESTMapper (e.g., by calling a helper that gets it based on kubeconfig context).
    // 4. Use mapper.RESTMapping(gvk.GroupKind(), gvk.Version).
    // 5. Return mapping.Resource.
    // Handle errors.
    // For MVP, this might be hardcoded for common types if a full RESTMapper is too complex initially.
    // However, a proper RESTMapper is needed for arbitrary GVKs.
    gv, err := schema.ParseGroupVersion(apiVersion)
    if err != nil {
        return schema.GroupVersionResource{}, err
    }
    gvk := schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: kind}
    
    // This is a simplified placeholder. A real implementation needs a RESTMapper.
    // For example, for "apps/v1/Deployment":
    if gvk.Group == "apps" && gvk.Version == "v1" && gvk.Kind == "Deployment" {
        return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, nil
    }
    if gvk.Group == "" && gvk.Version == "v1" && gvk.Kind == "Pod" {
        return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, nil
    }
    // ... add more common types or implement full RESTMapper lookup
    return schema.GroupVersionResource{}, fmt.Errorf("GVK %s not mapped to GVR (implement RESTMapper)", gvk.String())
}
```

## 7. Component Breakdown

### cmd/kyverno-mcp-server/main.go:
- Parses command-line arguments, including a `--kubeconfig` flag (optional, for out-of-cluster).
- Initializes dependencies. The kyvernoEngine will be initialized with the kubeconfig path from the flag.
- Creates and starts the MCP server.

### pkg/mcp/server.go: 
(No change in description)

### pkg/mcp/apply_handler.go: 
(No change in description)

### pkg/kyverno/engine.go:
- Orchestrates policy application.
- Determines the effective kubeconfig path (request-specific or server default) to pass to the ResourceLoader.
- Uses PolicyLoader for policies and the updated ResourceLoader for resources from K8s API.

### pkg/kyverno/policy_loader.go: 
(No change in core responsibility for loading policies from paths)

### pkg/kyverno/resource_loader.go:
- New Role: Responsible for fetching Kubernetes resources directly from the Kubernetes API.
- Uses a Kubernetes dynamic client.
- Handles in-cluster and out-of-cluster client configuration based on kubeconfigPath.
- Fetches resources based on ResourceQuery criteria.

### pkg/config/config.go:
- May store the server's default kubeconfig path if provided via CLI flag.

### pkg/utils/k8s.go:
- Enhanced Role: Provides crucial utilities for Kubernetes client creation (dynamic client, potentially RESTMapper for GVK-to-GVR conversion). 
- Handles logic for in-cluster vs. out-of-cluster configuration.

## 8. Workflow (ApplyRequest)
1. Client Request: An MCP client constructs an ApplyRequest (including PolicyPaths, ResourceQueries, and optionally KubeconfigPath) and sends it.
2. MCP Server Receives: Routes to ApplyService.
3. Handler Invocation: ApplyService.ProcessApplyRequest is called.
4. Policy Loading: The kyvernoEngine (via PolicyLoader) loads policies from req.PolicyPaths.
5. Determine Kubeconfig: The kyvernoEngine determines the effective kubeconfig path (request-specific req.KubeconfigPath, server default, or empty for in-cluster).
6. Resource Loading (K8s API): The kyvernoEngine (via ResourceLoader) uses the determined kubeconfig to connect to the Kubernetes API and fetches resources matching req.ResourceQueries.
7. Context Preparation: User info, values, and context variables are prepared.
8. Kyverno Engine Execution: Policies are applied to the fetched resources.
9. Response Transformation: Engine responses are transformed.
10. MCP Response Construction: ApplyResponse is assembled.
11. Client Receives Response: The server sends the ApplyResponse.

## 9. API (MCP Interactions)
(No change in description, but ApplyRequest structure has changed as noted in 6.1)

## 10. Error Handling and Logging
(No change in description, but error handling for K8s API interactions is critical)

## 11. Implementation Details & Kyverno Integration

### Kyverno Dependencies: 
(No change in list)

### Kubernetes Client Libraries:
- `k8s.io/client-go/dynamic` for interacting with arbitrary resource types.
- `k8s.io/client-go/tools/clientcmd` for loading kubeconfig files.
- `k8s.io/client-go/rest` for in-cluster configuration.
- `k8s.io/apimachinery/pkg/runtime/schema` for GVK/GVR.
- `k8s.io/client-go/discovery` and `k8s.io/client-go/restmapper` for GVK to GVR mapping.

### Resource Loading: 
The ResourceLoader must robustly handle Kubernetes API interactions, including:
- Authentication (in-cluster service account or kubeconfig)
- GVK to GVR mapping (using a RESTMapper)
- Fetching single or multiple resources

### Configuration: 
The main.go in cmd/kyverno-mcp-server should use the flag package to accept a `--kubeconfig` command-line argument. This path (or an empty string if not provided for in-cluster) should be passed when initializing the kyvernoEngine.

### RBAC: 
The service account (if in-cluster) or the user in kubeconfig (if out-of-cluster) needs appropriate RBAC permissions (get, list, watch) for the resources specified in ResourceQueries across the target namespaces.

## 12. Future Considerations
- Advanced Resource Queries: Support more complex query mechanisms if needed.
- Policy Loading from Git/Kustomize: Re-introduce if policies also need to be sourced dynamically (as per previous iterations of this document).
- Caching: Implement caching for K8s API responses (e.g., GVR mappings, or even resources if appropriate for the use case, though apply usually implies fresh data).
- Security: (No change)
- Metrics and Tracing: (No change)

This architecture now centralizes resource acquisition through the Kubernetes API, providing flexibility for different deployment scenarios of the MCP server.