# Kyverno MCP Server

THIS IS A WORK-IN-PROGRESS.

A Model Context Protocol (MCP) server that provides Kyverno policy application as a service. This server allows you to apply Kyverno policies to Kubernetes resources using a simple HTTP API.

## Prerequisites

Before you begin, ensure you have the following installed on your system:

- Go 1.24.1 or later
- Kubernetes cluster (EKS, Minikube, or any other Kubernetes distribution)
- `kubectl` configured to communicate with your cluster
- AWS CLI (if using EKS)
- Docker (if using Minikube)

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/kyverno/go-kyverno-mcp.git
   cd go-kyverno-mcp
   ```

2. Build the server:
   ```bash
   go build -o kyverno-mcp-server ./cmd/kyverno-mcp-server
   ```

## Configuration

The server can be configured using command-line flags:

- `--kubeconfig`: Path to the kubeconfig file (default: uses in-cluster config)
- `--port`: Port to listen on (default: 8080)

## Usage

### Starting the Server

To start the server with a specific kubeconfig:

```bash
KUBECONFIG=~/.kube/config ./kyverno-mcp-server --kubeconfig ~/.kube/config
```

### API Endpoints

#### Health Check

Check if the server is running:

```bash
curl http://localhost:8080/health
```

#### Apply Policies

Apply Kyverno policies to resources:

```bash
curl -X POST -H "Content-Type: application/json" -d @request.json http://localhost:8080/apply
```

Example `request.json`:

```json
{
  "policyPaths": ["/path/to/policy.yaml"],
  "resourceQueries": [
    {
      "apiVersion": "v1",
      "kind": "Pod",
      "namespace": "default"
    }
  ],
  "mutate": false,
  "validate": true
}
```

### Example Workflow

1. Start the server:
   ```bash
   KUBECONFIG=~/.kube/config ./kyverno-mcp-server --kubeconfig ~/.kube/config
   ```

2. Create a policy file (e.g., `disallow-privileged.yaml`):
   ```yaml
   apiVersion: kyverno.io/v1
   kind: ClusterPolicy
   metadata:
     name: disallow-privileged
   spec:
     validationFailureAction: enforce
     rules:
     - name: validate-privileged
       match:
         resources:
           kinds:
           - Pod
       validate:
         message: "Privileged mode is not allowed. Please remove privileged mode or set it to false."
         pattern:
           spec:
             containers:
             - (name): "*"
               securityContext:
                 privileged: false
   ```

3. Create a request file (e.g., `request.json`):
   ```json
   {
     "policyPaths": ["./disallow-privileged.yaml"],
     "resourceQueries": [
       {
         "apiVersion": "v1",
         "kind": "Pod",
         "namespace": "default"
       }
     ]
   }
   ```

4. Send the request:
   ```bash
   curl -X POST -H "Content-Type: application/json" -d @request.json http://localhost:8080/apply
   ```

## Development

### Building

```bash
go build -o kyverno-mcp-server ./cmd/kyverno-mcp-server
```

### Testing

Run the tests:

```bash
go test ./...
```

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
