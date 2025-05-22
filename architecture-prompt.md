I am building an MCP server for Kyverno (https://kyverno.io/).

The initial use case I want to support is applying a set of policies to resource that are in an existing cluster or namespace. 
This is similar to the Kyverno CLI `apply` command documented here: https://kyverno.io/docs/kyverno-cli/usage/apply/.


## Policy Loader

Only policy sets from Git repositories are supported. Limit the support to public repositories for now. 
An example is this pod security policy set: https://github.com/kyverno/policies/tree/main/pod-security. 
Kustomize files should be supported for policy sets, and the implementation should render the YAMLs when 
a `kustomization.yaml` file is found.

## Resource Loader

Resources should be loaded using the Kubernetes API. The server should handle running in cluster, with a service account, or outside of a cluster. When running outside a cluster a `--kubeconfig` parameter is used to pass the Kubernetes credentials.

## Implementation

I would like to use Golang and the https://github.com/mark3labs/mcp-go library.

I like how the FluxCD project has implemented their MCP server. You can use that as a reference. 
The code is available at: https://github.com/controlplaneio-fluxcd/flux-operator/tree/main/cmd/mcp.

Give me the full architecture:
- File + folder structure
- Key packages and interfaces
- What each component does
- All other details for implementation

Format this entire document in markdown.
