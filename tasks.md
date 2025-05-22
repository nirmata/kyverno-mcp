# Kyverno MCP Server MVP - Step-by-Step Implementation Plan (K8s API Resources)

This plan breaks down the MVP development into small, testable tasks, focusing on loading resources via the Kubernetes API as per the current architecture.

## Phase 0: Project Setup & Core MCP Types
**Goal**: Establish the project structure and define core data types for MCP communication, reflecting API-based resource queries.

### Task 0.1: Initialize Go Module & Basic Directory Structure

**Start**: Empty directory.

**Action**:
- Run `go mod init <your_module_path>` (e.g., `github.com/username/kyverno-mcp-server`).
- Create the initial directory structure: `cmd/kyverno-mcp-server`, `pkg/mcp/types`, `pkg/kyverno`, `pkg/utils`, `pkg/config`.
- Create empty `.go` files (e.g., `main.go`, `apply.go`, `engine.go`, `policy_loader.go`, `resource_loader.go`, `k8s.go`, `fs.go`) in their respective directories.

**End**: Project initialized with `go.mod`, `go.sum`, and basic folder/file structure.

**Test**: Verify directory structure. `go build ./...` should pass.

### Task 0.2: Define MCP ApplyRequest, ResourceQuery, & ApplyResponse Types

**Start**: `pkg/mcp/types/apply.go` is empty.

**Action**: Implement the Go structs for `ResourceQuery`, `ApplyRequest`, `ApplyResponse`, `PolicyApplicationResult`, `ResourceInfo`, and `RuleResult` in `pkg/mcp/types/apply.go` as defined in Section 6.1 of the architecture document. Ensure `ApplyRequest` includes `ResourceQueries` and `KubeconfigPath`, and omits `ResourcePaths` (for K8s resources) and `Cluster`.

**End**: `pkg/mcp/types/apply.go` contains complete type definitions.

**Test**: `go build ./pkg/mcp/types/...` compiles. Manually review struct definitions.

## Phase 1: Kubernetes API Utilities (pkg/utils/k8s.go)
**Goal**: Implement essential utilities for interacting with the Kubernetes API.

### Task 1.1: Implement GetDynamicClient Utility

**Start**: `pkg/utils/k8s.go` is empty or has placeholders.

**Action**:
- Implement `GetDynamicClient(kubeconfigPath string) (dynamic.Interface, error)` in `pkg/utils/k8s.go`.
- If `kubeconfigPath` is empty, use `rest.InClusterConfig()`.
- If `kubeconfigPath` is provided, use `clientcmd.BuildConfigFromFlags("", kubeconfigPath)`.
- Create and return a `dynamic.NewForConfig(config)`. Handle all errors.
- Add imports: `k8s.io/client-go/dynamic`, `k8s.io/client-go/tools/clientcmd`, `k8s.io/client-go/rest`.

**End**: `GetDynamicClient` can create a client for in-cluster or out-of-cluster scenarios.

**Test**:
- Write `k8s_test.go`.
- Test case 1 (In-cluster mock): Mock `rest.InClusterConfig()` to return a dummy config. Verify `dynamic.NewForConfig` is called.
- Test case 2 (Out-of-cluster): Create a dummy kubeconfig file. Call `GetDynamicClient` with its path. Verify a client is attempted to be created. Clean up dummy kubeconfig.
- Test case 3: Invalid kubeconfig path, verify error.

### Task 1.2: Implement GVKToGVR Utility (Simplified Initial Version)

**Start**: `pkg/utils/k8s.go` has `GetDynamicClient`.

**Action**:
- Implement `GVKToGVR(apiVersion, kind string) (schema.GroupVersionResource, error)` in `pkg/utils/k8s.go`.
- For this initial MVP task, implement a simplified version that uses a hardcoded map for common GVKs to GVRs (e.g., Pod, Deployment, ConfigMap, Service).
- Return an error if the GVK is not in the hardcoded map. Add a TODO comment to implement full RESTMapper-based discovery later.
- Add imports: `k8s.io/apimachinery/pkg/runtime/schema`, `fmt`.

**End**: `GVKToGVR` can map a few common GVKs.

**Test**:
- Extend `k8s_test.go`.
- Test case 1: Call with `"v1"`, `"Pod"`. Verify correct GVR (`{Group: "", Version: "v1", Resource: "pods"}`).
- Test case 2: Call with `"apps/v1"`, `"Deployment"`. Verify correct GVR.
- Test case 3: Call with an unmapped GVK. Verify error.
- Test case 4: Call with invalid apiVersion string. Verify error.

## Phase 2: Resource Loader (Kubernetes API - pkg/kyverno/resource_loader.go)
**Goal**: Implement loading of Kubernetes resources from the API using the utilities from Phase 1.

### Task 2.1: Define ResourceLoader Interface and apiResourceLoader Struct

**Start**: `pkg/kyverno/resource_loader.go` is mostly empty.

**Action**:
- Define the `ResourceLoader` interface with `Load(ctx context.Context, queries []types.ResourceQuery, kubeconfigPath string) ([]*unstructured.Unstructured, error)`.
- Define the `apiResourceLoader` struct (it might not need fields for now).
- Implement `NewAPIResourceLoader()` constructor.
- Add imports: `context`, project types, `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`.

**End**: Interface and struct definitions are complete.

**Test**: `go build ./pkg/kyverno/...` compiles.

### Task 2.2: Implement apiResourceLoader.Load() - Client and GVR Setup

**Start**: `apiResourceLoader.Load()` is a stub.

**Action**:
- In `Load()`:
  - Initialize an empty `allResources []*unstructured.Unstructured`.
  - Call `utils_k8s.GetDynamicClient(kubeconfigPath)` to get the dynamic client. Handle errors.
  - Start a loop through queries.
  - Inside the loop, call `utils_k8s.GVKToGVR(query.APIVersion, query.Kind)` for each query. Handle errors.
- Add imports: project `utils/k8s`, `fmt`, `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`, `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"`, `k8s.io/client-go/dynamic`.

**End**: `Load()` can initialize a client and map GVKs to GVRs for each query.

**Test**:
- Write `resource_loader_test.go`.
- Mock `utils_k8s.GetDynamicClient` to return a mock `dynamic.Interface`.
- Mock `utils_k8s.GVKToGVR`.
- Test case 1: Call `Load` with a valid query. Verify `GetDynamicClient` and `GVKToGVR` are called with expected arguments.
- Test case 2: `GetDynamicClient` returns error. Verify `Load` returns error.
- Test case 3: `GVKToGVR` returns error. Verify `Load` returns error.

### Task 2.3: Implement apiResourceLoader.Load() - Fetch Single Named Resource

**Start**: `Load()` sets up client and GVR.

**Action**:
- Inside the queries loop in `Load()`:
  - Determine `resourceInterface` (namespaced or cluster-scoped) using `dynamicClient.Resource(gvr)`.
  - If `query.Name` is not empty:
    - Call `resourceInterface.Get(ctx, query.Name, metav1.GetOptions{})`.
    - If successful, append the result to `allResources`.
    - Handle errors from `Get` (e.g., NotFound could be logged and skipped, or returned as an error based on desired strictness for MVP - let's error out for now if a specific named resource is not found as per arch doc).

**End**: `Load()` can fetch a single specified resource by name.

**Test**:
- Extend `resource_loader_test.go`.
- Mock `dynamicClient.Resource(gvr).Namespace(ns).Get(...)` or `dynamicClient.Resource(gvr).Get(...)`.
- Test case 1: Query for a named resource. Mock `Get` to return a sample `unstructured.Unstructured`. Verify it's added to results.
- Test case 2: Mock `Get` to return a `k8serrors.NewNotFound` error. Verify `Load` returns an error.
- Test case 3: Mock `Get` to return a different error. Verify `Load` returns an error.

### Task 2.4: Implement apiResourceLoader.Load() - List Resources by Label Selector

**Start**: `Load()` can fetch named resources.

**Action**:
- Inside the queries loop, in the else block (if `query.Name` is empty):
  - Create `metav1.ListOptions{}`. If `query.LabelSelector` is not empty, set `listOptions.LabelSelector`.
  - Call `resourceInterface.List(ctx, listOptions)`.
  - Iterate through `resourceList.Items` and append copies to `allResources`.
  - Handle errors from `List`.

**End**: `Load()` can list resources based on kind and label selector.

**Test**:
- Extend `resource_loader_test.go`.
- Mock `dynamicClient.Resource(gvr).Namespace(ns).List(...)` or `dynamicClient.Resource(gvr).List(...)`.
- Test case 1: Query with a label selector. Mock `List` to return a list with one or more items. Verify items are added.
- Test case 2: Query without a label selector (list all). Mock `List` to return items. Verify.
- Test case 3: Mock `List` to return an error. Verify `Load` returns an error.

## Phase 3: Policy Loader (Local Files - pkg/kyverno/policy_loader.go)
**Goal**: Implement loading of Kyverno policies from local file paths (as per architecture section 6.5 note for MVP).

### Task 3.1: Define PolicyLoader Interface and localPolicyLoader Struct

**Start**: `pkg/kyverno/policy_loader.go` is mostly empty.

**Action**:
- Define the `PolicyLoader` interface with `Load(policyPaths []string, gitBranch string) ([]kyvernov1.PolicyInterface, error)`.
- Define `localPolicyLoader` struct.
- Implement `NewLocalPolicyLoader()` constructor.
- Add imports: `github.com/kyverno/kyverno/api/kyverno/v1`.

**End**: Interface and struct definitions for local policy loading are complete.

**Test**: `go build ./pkg/kyverno/...` compiles.

### Task 3.2: Implement localPolicyLoader.Load() for Local Policy Files/Directories

**Start**: `localPolicyLoader.Load()` is a stub.

**Action**:
- Implement the logic to iterate through `policyPaths`.
- Use Kyverno's `github.com/kyverno/kyverno/cmd/cli/kubectl-kyverno/utils/common.LoadPolicies(nil, "", policyPaths, "")` to load policies from the given paths.
- Aggregate and return all loaded `kyvernov1.PolicyInterface` objects. Handle errors.

**End**: `Load()` can load policies from local file paths/directories.

**Test**:
- Write `policy_loader_test.go`.
- Create test policy YAML files (e.g., a simple validate policy).
- Test case 1: Load a single policy file. Verify the `PolicyInterface` object.
- Test case 2: Load multiple policy files from a directory. Verify all are loaded.
- Test case 3: Test with an invalid path. Verify error.
- Test case 4: Test with a malformed policy YAML. Verify error.

## Phase 4: Kyverno Engine Wrapper (pkg/kyverno/engine.go)
**Goal**: Set up the Kyverno engine wrapper to use the new resource loader and handle server-side kubeconfig.

### Task 4.1: Update Engine Interface and kyvernoEngine Struct & Constructor

**Start**: `pkg/kyverno/engine.go` might have old definitions.

**Action**:
- Ensure `Engine` interface is `ApplyPolicies(ctx context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error)`.
- Update `kyvernoEngine` struct to hold `policyLoader PolicyLoader`, `resourceLoader ResourceLoader`, and `serverKubeconfigPath string`.
- Update `NewKyvernoEngine(pl PolicyLoader, rl ResourceLoader, serverKubeconfigPath string) Engine` constructor to accept and store these.

**End**: Engine definition updated.

**Test**: `go build ./pkg/kyverno/...` compiles.

### Task 4.2: Implement kyvernoEngine.ApplyPolicies() - Kubeconfig Logic & Loader Calls

**Start**: `kyvernoEngine.ApplyPolicies()` is a stub or has old logic.

**Action**:
- In `ApplyPolicies()`:
  - Create an initial `ApplyResponse` with `RequestID: req.RequestID`.
  - Call `ke.policyLoader.Load(req.PolicyPaths, "")`. Handle and return error in `resp.Error`.
  - Determine `kubeconfigForLoad`: if `req.KubeconfigPath` is not empty, use it; otherwise, use `ke.serverKubeconfigPath`.
  - Call `ke.resourceLoader.Load(ctx, req.ResourceQueries, kubeconfigForLoad)`. Handle and return error in `resp.Error`.
  - For now, if policies and resources are loaded successfully, log them (or their counts) and return resp with empty Results.

**End**: `ApplyPolicies` correctly determines kubeconfig and calls new loaders.

**Test**:
- Write/Update `engine_test.go`.
- Mock `PolicyLoader` and `ResourceLoader`.
- Test case 1: `req.KubeconfigPath` is provided. Verify `resourceLoader.Load` is called with `req.KubeconfigPath`.
- Test case 2: `req.KubeconfigPath` is empty. Verify `resourceLoader.Load` is called with `serverKubeconfigPath`.
- Test case 3: Mocks return valid policies/resources. Verify response without error.
- Test case 4: Mock `PolicyLoader` returns error. Verify error in response.
- Test case 5: Mock `ResourceLoader` returns error. Verify error in response.

## Phase 5: MCP Server & Handler (pkg/mcp/*, cmd/*)
**Goal**: Set up the MCP server, handler, and command-line interface for kubeconfig.

### Task 5.1: Define ApplyService Struct and Constructor

**Start**: `pkg/mcp/apply_handler.go` (or `handler.go`) might be empty.

**Action**: Define `ApplyService` struct (embedding MCP base service) holding `kyverno.Engine`. Implement `NewApplyService`.

**End**: `ApplyService` defined.

**Test**: `go build ./pkg/mcp/...` compiles.

### Task 5.2: Implement ApplyService.ProcessApplyRequest() (Stubbed Call to Engine)

**Start**: `ProcessApplyRequest` method is missing or hardcodes response.

**Action**: Implement `ProcessApplyRequest(ctx context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error)`. Call `s.KyvernoEngine.ApplyPolicies(ctx, req)`. Return its response/error.

**End**: `ProcessApplyRequest` delegates to `kyvernoEngine`.

**Test**:
- Write/Update `apply_handler_test.go`.
- Mock `kyverno.Engine`.
- Configure `ApplyPolicies` to return a specific response/error.
- Verify `ProcessApplyRequest` returns these.

### Task 5.3: Update main.go for --kubeconfig Flag and Server Initialization

**Start**: `cmd/kyverno-mcp-server/main.go` is empty or has old init.

**Action**:
- In `main()`:
  - Use the flag package to define a `--kubeconfig` string flag (optional).
  - Parse flags.
  - Initialize `localPolicyLoader` and `apiResourceLoader`.
  - Initialize `kyvernoEngine` using `NewKyvernoEngine`, passing the value of the `--kubeconfig` flag as `serverKubeconfigPath`.
  - Initialize `ApplyService` with the engine.
  - Setup and start the mark3labs/mcp-go server, registering `ApplyService`.

**End**: `main.go` handles `--kubeconfig` and starts server with correctly configured engine.

**Test**:
- Run `go run ./cmd/kyverno-mcp-server/main.go --kubeconfig=path/to/dummy.yaml`. Verify server starts.
- Run `go run ./cmd/kyverno-mcp-server/main.go`. Verify server starts (for in-cluster scenario).
- (Manual E2E if ready): Send a request. Verify logs show the correct kubeconfig path being considered by the engine.

## Phase 6: Full Kyverno Engine Logic & Integration
**Goal**: Implement the core policy application logic within `kyvernoEngine.ApplyPolicies()`.

### Task 6.1: kyvernoEngine.ApplyPolicies() - UserInfo and ValuesFile Processing

**Action**: Load UserInfo from `req.UserInfoPath`, parse `req.ValuesFileContent`.

**Test**: Unit tests for these specific functionalities.

### Task 6.2: kyvernoEngine.ApplyPolicies() - Policy Context Creation

**Action**: Create `engine.ContextInterface` for each resource, adding variables, UserInfo, resource.

**Test**: Code review, integration testing later.

### Task 6.3: kyvernoEngine.ApplyPolicies() - Iterating and Applying Policies (Validate)

**Action**: Loop policies/resources, call `policyEngine.Validate()`, transform `engine.PolicyResponse` to `types.PolicyApplicationResult`.

**Test**: Unit tests with mock engine calls or simple real engine calls.

### Task 6.4: kyvernoEngine.ApplyPolicies() - Iterating and Applying Policies (Mutate)

**Action**: Call `policyEngine.Mutate()`, transform response, marshal patched resource. Ensure mutated resource is used by subsequent rules for that resource.

**Test**: Unit tests.

### Task 6.5: kyvernoEngine.ApplyPolicies() - Consolidate and Refine Response Mapping

**Action**: Ensure full and accurate mapping from `engine.PolicyResponse` / `engine.RuleResponse` to MCP types.

**Test**: Refine previous tests.

## Phase 7: End-to-End Testing & Refinement
**Goal**: Perform end-to-end tests with a real/mocked K8s API and refine the MVP.

### Task 7.1: Setup E2E Test Environment (Kind/Minikube or Mocked K8s API)

**Action**: Decide on E2E testing strategy. For initial tests, a Kind cluster is good. Ensure test K8s cluster has some sample resources (Deployments, Pods).

**End**: Test environment ready.

### Task 7.2: Comprehensive E2E Test Case 1 (Validation Against K8s Resources)

**Start**: All components integrated, E2E environment ready.

**Action**:
- Prepare local policy files (e.g., validate a label exists on Deployments).
- Deploy sample resources to your test K8s cluster.
- Construct an `ApplyRequest`:
  - `PolicyPaths` pointing to local policy files.
  - `ResourceQueries` to select relevant resources from your K8s cluster (e.g., all Deployments in a namespace).
  - `KubeconfigPath` (if MCP server is out-of-cluster) or let server use in-cluster config.
  - `Validate: true`, `Mutate: false`.
- Start MCP server (pointing to test K8s cluster if out-of-cluster).
- Send `ApplyRequest` using a simple MCP client.
- Inspect `ApplyResponse`. Verify results based on policies and actual cluster resources.

**End**: Successful E2E validation against live K8s API resources.

### Task 7.3: Comprehensive E2E Test Case 2 (Mutation Against K8s Resources)

**Action**: Similar to 7.2, but use a mutation policy. Verify `PatchedResource` and, if possible, check if the resource in the K8s cluster would have been patched (the apply command is dry-run like, it doesn't write back to cluster). The `PatchedResource` in response is key.

**End**: Successful E2E mutation test.

### Task 7.4: Error Handling and Edge Case Testing (K8s API)

**Action**:
- Test with `ResourceQueries` for non-existent GVKs (test `GVKToGVR` error path).
- Test with `ResourceQueries` for kinds/resources the server's SA/kubeconfig doesn't have RBAC for. Verify K8s API errors are propagated.
- Test server running in-cluster (if possible in test setup) vs. out-of-cluster with `--kubeconfig`.
- Test invalid `kubeconfigPath` in `ApplyRequest` or server flag.

**End**: Robustness for K8s API interactions improved.

This revised plan aligns with the architecture focusing on Kubernetes API for resource loading.