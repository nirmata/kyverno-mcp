# Kyverno MCP Server

A Model Context Protocol (MCP) server that provides Kyverno policy management capabilities through a standardized interface. This server allows AI assistants to interact with Kyverno policies in a Kubernetes cluster.

## Prerequisites

- Go 1.24 or higher (for building the binary)

## Installation

### Option A: Homebrew

```bash
brew tap nirmata/tap
brew install kyverno-mcp
```

### Option B: Nirmata downloads page

Choose the appropriate architecture and platform here: https://downloads.nirmata.io/kyverno-mcp/downloads/

## Usage

### Starting the Server

```bash
# Basic usage – uses the default kubeconfig at ~/.kube/config and the current context
./kyverno-mcp

# With an explicit kubeconfig file
./kyverno-mcp --kubeconfig=/path/to/kubeconfig

# The server always starts in the kubeconfig's current context; use the `switch_context` tool to change it at runtime.
```

#### Exposing the MCP transport over HTTP(S)

If you need to expose the server over the network (for example to a browser-based or remote MCP client) you can enable the built-in **Streamable HTTP** transport:

```bash
# Plain HTTP  (🚫 NOT recommended for production – use ONLY for local testing on a trusted network)
./kyverno-mcp --http-addr :8080

# HTTPS with TLS  (✅ Recommended for production usage)
./kyverno-mcp \
  --http-addr :8443 \
  --tls-cert /path/to/cert.pem \
  --tls-key /path/to/key.pem

# Alternatively, run the server without the --tls-* flags and terminate TLS in
# a reverse-proxy such as NGINX, Caddy, or a cloud load balancer.
```

Plain HTTP traffic is unencrypted and vulnerable to eavesdropping and
man-in-the-middle attacks; therefore **never expose the server over plain HTTP in
production**. Always provide valid `--tls-cert` and `--tls-key` files **or** place
the binary behind an HTTPS-terminating proxy when making the service reachable
outside of localhost.

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

### Running in a container (Docker / Podman)

If you prefer not to install Go or the binary on the host you can build and run a container image instead.

```bash
# 1. Build the image
docker build -t kyverno-mcp:latest .

# 2. Run the server (mount your kubeconfig read-only)
docker run --rm -i \
  -v $HOME/.kube/config:/kube/config:ro \
  kyverno-mcp:latest -- \
  --kubeconfig /kube/config
```

Notes:

1. The `--` tells Docker to pass the remaining flags to the `kyverno-mcp` binary inside the container.
2. Inside the container the kubeconfig is expected at `/kube/config`, hence the corresponding flag value.
3. Replace `$HOME/.kube/config` with an alternative path if your kubeconfig is elsewhere.

### Running in Kubernetes

The `k8s-manifests/` directory contains Kubernetes deployment manifests. By default, these are configured for **HTTPS** with TLS certificates.

#### Default Configuration (HTTPS)

The default manifests use HTTPS:
- `service.yaml`: Exposes port 443 (HTTPS)
- `deployment.yaml`: Includes TLS certificate configuration

```bash
kubectl apply -f k8s-manifests/
```

#### Switching to HTTP Configuration

If you prefer to run without TLS (🚫 **NOT recommended for production**), you need to modify both manifests:

**1. Update `k8s-manifests/service.yaml`:**
```yaml
ports:
  - port: 80        # Change from 443 to 80
    targetPort: 8000
```

**2. Update `k8s-manifests/deployment.yaml`:**
Remove the TLS-related arguments and volume mounts:
```yaml
args:
  - --http-addr
  - :8000
  # Remove these TLS lines:
  # - --tls-cert
  # - /etc/tls/tls.crt
  # - --tls-key
  # - /etc/tls/tls.key

# Also remove the TLS volume mount and volume sections:
# volumeMounts:
#   - name: tls-certs
#     mountPath: /etc/tls
#     readOnly: true
# volumes:
#   - name: tls-certs
#     secret:
#       secretName: kyverno-mcp-tls
```

**3. Update the liveness probe scheme:**
```yaml
livenessProbe:
  httpGet:
    path: /mcp
    port: 8000
    scheme: HTTP    # Change from HTTPS to HTTP
```

## Command Line Flags

- `--kubeconfig` (string): Path to the kubeconfig file (defaults to the value of $KUBECONFIG, or ~/.kube/config if unset)
- `--http-addr` (string): Address to bind the HTTP(S) server. If not provided, the server runs on stdio (default mode)
- `--tls-cert` (string): Path to the TLS certificate file. When provided with `--tls-key`, enables HTTPS
- `--tls-key` (string): Path to the TLS private key file. When provided with `--tls-cert`, enables HTTPS

**Note:** The server automatically determines the transport mode:
- If `--http-addr` is provided with both `--tls-cert` and `--tls-key`, it runs as HTTPS
- If `--http-addr` is provided without TLS credentials, it runs as plain HTTP
- If `--http-addr` is not provided, it runs on stdio (default)

## Available Tools

### Table of Contents

- [1. list_contexts](#1-list-kubernetes-contexts)
- [2. switch_context](#2-switch-kubernetes-context)
- [3. apply_policies](#3-apply-policies)
- [4. show_violations](#4-show-policy-violations)
- [5. help](#5-kyverno-documentation-help)

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

### 3. Apply Policies

Apply policies to cluster resources using curated Kyverno policy sets, Git repositories, or local filesystem.


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

### 4. Show Policy Violations

Display all policy violations and errors from Kyverno `PolicyReport` (namespaced) and `ClusterPolicyReport` (cluster-wide) resources.

If Kyverno is not installed in the active cluster, this tool instead returns a short set of Helm commands that you can run to install the Kyverno controller and its default policy sets.

**Parameters:**
- `namespace` (string, optional): Namespace whose `PolicyReports` should be returned (default: `default`). Cluster-wide `ClusterPolicyReports` are always included regardless of the value.

**Example Request:**
```json
{
  "tool": "show_violations",
  "namespace": "default"
}
```

### 4. Kyverno Documentation Help

Retrieve official Kyverno documentation snippets directly from the server. Useful for quick reference on installation steps or troubleshooting guidance without leaving your chat client.

**Parameters:**
- `topic` (string, required): Documentation topic to retrieve. Accepted values:
  - `installation` – Kyverno installation guide
  - `troubleshooting` – Common troubleshooting tips

**Example Request:**
```json
{
  "tool": "help",
  "topic": "installation"
}
```
