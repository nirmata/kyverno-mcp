# Kyverno MCP Server

A Model Context Protocol (MCP) server that provides Kyverno policy management capabilities through a standardized interface. This server allows AI assistants to interact with Kyverno policies in a Kubernetes cluster.

## Prerequisites

- Go 1.24 or higher (for building the binary)

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/nirmata/kyverno-mcp
   cd kyverno-mcp
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
        "--kubeconfig=/path/to/your/kubeconfig"
      ]
    }
  }
}
```

## Command Line Flags

- `--kubeconfig` (string): Path to the kubeconfig file (defaults to the value of $KUBECONFIG, or ~/.kube/config if unset)

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
  "context": "your-cluster-name"
}
```

### 3. Scan Cluster

Scan cluster resources using curated Kyverno policy sets, or policies from Git repositories, or files.

**Parameters:**
- `policySets` (string, optional): Policy set name (`pod-security`, `rbac-best-practices`, `kubernetes-best-practices`, `all`), or Git repository URLs, or file paths
- `namespace` (string, optional): Namespace to apply policies to (default: `default`)
- `gitBranch` (string, optional): Branch to use when `policySets` is a Git repo URL (default: `main`)

**Example Request:**
```json
{
  "tool": "apply_policies",
  "policySets": "all",
  "namespace": "default",
  "gitBranch": "main"
}
```
