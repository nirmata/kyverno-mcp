# Kyverno MCP Server

A Model Context Protocol (MCP) server that provides Kyverno policy management capabilities through a standardized interface. This server allows AI assistants to interact with Kyverno policies in a Kubernetes cluster.

## Prerequisites

- Go 1.16 or higher
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
# Basic usage – uses the default kubeconfig at ~/.kube/config and the current context
./kyverno-mcp

# With an explicit kubeconfig file
./kyverno-mcp --kubeconfig=/path/to/kubeconfig

# The server always starts in the kubeconfig's current context; use the `switch_context` tool to change it at runtime.
```

### Using it with an MCP Client (Claude Desktop, Amazon Q, Cursor, etc.)

```json
{
  "mcpServers": {
    "kyverno": {
      "command": "/path/to/kyverno-mcp",
      "args": [
        "--kubeconfig=/path/to/your/kubeconfig",
        "--awsconfig=/path/to/your/awsconfig",
      ]
    }
  }
}
```

## Command Line Flags

- `--kubeconfig` (string): Path to the kubeconfig file (defaults to the value of $KUBECONFIG, or ~/.kube/config if unset)
- `--awsconfig` (string): Path to the AWS config file (defaults to the value of $AWS_CONFIG_FILE, or ~/.aws/config if unset)
- `--awsprofile` (string): AWS profile to use (defaults to the current profile)

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

Scan the cluster using embedded Kyverno policy sets.

**Parameters:**
- `policySets` (string, optional): Policy set key: `pod-security`, `rbac-best-practices`, `best-practices-k8s`, or `all` (default: `all`)
- `namespace` (string, optional): Namespace to scan (default: `all`)

**Example Request:**
```json
{
  "tool": "scan_cluster",
  "policySets": "all",
  "namespace": "default"
}
```
