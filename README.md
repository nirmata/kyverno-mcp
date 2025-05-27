# Kyverno MCP Server

A Model Context Protocol (MCP) server that provides Kyverno policy management capabilities through a standardized interface. This server allows AI assistants to interact with Kyverno policies in a Kubernetes cluster.

## Features

- **Scan Cluster**: Scan the cluster for resources that match specific Kyverno policies
- **Apply Policy**: Apply Kyverno policies to specific resources in the cluster
- **Kubernetes Context Support**: Work with different Kubernetes clusters using context switching
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
   cd kyverno-mcp
   ```

2. Build the binary:
   ```bash
   go build -o kyverno-mcp main.go
   ```

## Usage

### Starting the Server

```bash
./kyverno-mcp --context=<your-k8s-context> --debug
```

### Available Tools

#### 1. Scan Cluster

Scan the cluster for resources that match a specific Kyverno policy.

**Parameters:**
- `policy` (string, required): Name of the policy to scan with
- `namespace` (string, required): Namespace to scan (use 'all' for all namespaces)
- `kind` (string, required): Kind of resources to scan (e.g., Pod, Deployment)

**Example Request:**
```json
{
  "policy": "validate-pod-labels",
  "namespace": "default",
  "kind": "Pod"
}
```

#### 2. Apply Policy

Apply a Kyverno policy to a specific resource in the cluster.

**Parameters:**
- `policy` (string, required): Name of the policy to apply
- `resource` (string, required): Name of the resource to apply the policy to
- `namespace` (string, required): Namespace of the resource

**Example Request:**
```json
{
  "policy": "validate-pod-labels",
  "resource": "my-pod",
  "namespace": "default"
}
```

## Configuration

### Command Line Flags

- `--context`: Kubernetes context to use (default: current context)
- `--debug`: Enable debug logging (default: false)

## Integration with Claude AI

To use this MCP server with Claude AI, update your Claude desktop configuration:

```json
{
  "mcpServers": {
    "kyverno": {
      "command": "/path/to/kyverno-mcp",
      "args": [
        "--context=minikube",
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
