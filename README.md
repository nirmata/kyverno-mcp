# Kyverno MCP Server

A Model Context Protocol (MCP) server that provides Kyverno policy management capabilities through a standardized interface. This server allows AI assistants to interact with Kyverno policies in a Kubernetes cluster.

## Features

- **Kubernetes Context Management**: List and switch between different Kubernetes contexts
- **Cluster Scanning**: Scan the cluster for resources that match specific Kyverno policies
- **Policy Management**: Apply Kyverno policies to specific resources in the cluster
- **Policy Inspection**: List and inspect cluster and namespaced policies
- **Policy Reports**: View policy reports across namespaces
- **Policy Exceptions**: List policy exceptions
- **Debug Mode**: Enable detailed logging for troubleshooting

## Prerequisites

- Go 1.16 or higher
- Kubernetes cluster with Kyverno installed
- `kubectl` configured with access to your cluster
- Kyverno CLI (optional, for local testing)

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/nirmata/go-kyverno-mcp
   cd go-kyverno-mcp
   ```

2. Build the binary:
   ```bash
   go build -o kyverno-mcp main.go
   ```

## Usage

### Starting the Server

```bash
# Basic usage (uses default kubeconfig at ~/.kube/config and current context)
./kyverno-mcp

# With custom kubeconfig and context
./kyverno-mcp --kubeconfig=/path/to/kubeconfig --context=my-cluster
```

## Command Line Flags

- `--kubeconfig` (string): Path to the kubeconfig file (default: "" - uses default location ~/.kube/config)
- `--context` (string): Kubernetes context to use (default: "" - uses current context)

### Examples

Start with a specific kubeconfig and context:

```bash
./kyverno-mcp --kubeconfig=~/.kube/config --context=my-cluster
```

Enable debug logging:

```bash
./kyverno-mcp --debug
```

## Available Tools

### 1. List Kubernetes Contexts

List all available Kubernetes contexts.

**Example Request:**
```json
{
  "tool": "list_contexts"
}
```

### 2. Switch Kubernetes Context

Switch to a different Kubernetes context.

**Parameters:**
- `context` (string, required): Name of the context to switch to

**Example Request:**
```json
{
  "tool": "switch_context",
  "context": "my-cluster-context"
}
```

### 3. Scan Cluster

Scan the cluster for resources that match a specific Kyverno policy.

**Parameters:**
- `policy` (string, required): Name of the policy to scan with
- `namespace` (string, required): Namespace to scan (use 'all' for all namespaces)
- `kind` (string, required): Kind of resources to scan (e.g., Pod, Deployment)

**Example Request:**
```json
{
  "tool": "scan_cluster",
  "policy": "validate-pod-labels",
  "namespace": "default",
  "kind": "Pod"
}
```

### 4. Apply Policy

Apply a Kyverno policy to a specific resource in the cluster.

**Parameters:**
- `policy` (string, required): Name of the policy to apply
- `resource` (string, required): Name of the resource to apply the policy to
- `namespace` (string, required): Namespace of the resource

**Example Request:**
```json
{
  "tool": "apply_policy",
  "policy": "validate-pod-labels",
  "resource": "my-pod",
  "namespace": "default"
}
```

### 5. List Cluster Policies

List all Kyverno cluster policies.

**Example Request:**
```json
{
  "tool": "list_cluster_policies"
}
```

### 6. Get Cluster Policy

Get a specific cluster policy.

**Parameters:**
- `name` (string, required): Name of the cluster policy

**Example Request:**
```json
{
  "tool": "get_cluster_policy",
  "name": "disallow-host-namespaces"
}
```

### 7. List Namespaced Policies

List all Kyverno namespaced policies across all namespaces.

**Example Request:**
```json
{
  "tool": "list_namespaced_policies"
}
```

### 8. Get Namespaced Policies by Namespace

Get namespaced policies in a specific namespace.

**Parameters:**
- `namespace` (string, required): Namespace to get policies from

**Example Request:**
```json
{
  "tool": "get_namespaced_policies",
  "namespace": "default"
}
```

### 9. List Policy Reports

List all Kyverno policy reports across all namespaces.

**Example Request:**
```json
{
  "tool": "list_policy_reports"
}
```

### 10. List Namespaced Policy Reports

List policy reports in a specific namespace.

**Parameters:**
- `namespace` (string, required): Namespace to get policy reports from

**Example Request:**
```json
{
  "tool": "list_namespaced_policy_reports",
  "namespace": "default"
}
```

### 11. List Policy Exceptions

List all policy exceptions across all namespaces.

**Parameters:**
- `namespace` (string, optional): Namespace to filter exceptions

**Example Request:**
```json
{
  "tool": "list_policy_exceptions",
  "namespace": "default"
}
```

## Integration with Claude AI

To use this MCP server with Claude AI, update your Claude desktop configuration:

```json
{
  "mcpServers": {
    "kyverno": {
      "command": "/path/to/kyverno-mcp",
      "args": [
        "--kubeconfig=/path/to/your/kubeconfig",
        "--context=your-cluster-context",
        "--debug"
      ]
    }
  }
}
```

## Development

### Building

```bash
go build -o kyverno-mcp main.go
```

### Testing

Run unit tests:
```bash
go test -v ./...
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Acknowledgements

- [Kyverno](https://kyverno.io/)
- [Model Context Protocol](https://modelcontextprotocol.io/)
- [Kubernetes](https://kubernetes.io/)
