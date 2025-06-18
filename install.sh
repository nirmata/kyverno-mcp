#!/usr/bin/env bash
set -euo pipefail

# Check for required commands
for cmd in curl tar; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: $cmd is not installed. Please install $cmd to proceed."
    exit 1
  fi
done

REPO="nirmata/kyverno-mcp"
BINARY="kyverno-mcp"

# Detect OS
sysOS="$(uname | tr '[:upper:]' '[:lower:]')"
case "$sysOS" in
  linux)   OS="Linux" ;;
  darwin)  OS="Darwin" ;;
  *)
    echo "OS currently not supported"
    exit 1
    ;;
esac

# Detect ARCH
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="x86_64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Architecture currently not supported"
    exit 1
    ;;
esac

# Get latest version tag from GitHub API, Use GITHUB_TOKEN if available to avoid potential rate limit
if [ -n "${GITHUB_TOKEN:-}" ]; then
  auth_hdr="Authorization: token $GITHUB_TOKEN"
else
  auth_hdr=""
fi
LATEST_TAG=$(curl -s -H "$auth_hdr" \
  "https://api.github.com/repos/$REPO/releases/latest" \
  | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')
if [ -z "$LATEST_TAG" ]; then
  echo "Failed to fetch latest release tag."
  exit 1
fi

# Compose download URL
TARBALL="kyverno-mcp_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$LATEST_TAG/$TARBALL"

# Download and extract
echo "Downloading $URL ..."
curl -fSL --retry 3 "$URL" -o "$TARBALL"
tar --no-same-owner -xzf "$TARBALL"

# Move binary to /usr/local/bin (may require sudo)
echo "Installing $BINARY to /usr/local/bin (may require sudo)..."
sudo install -m 0755 "$BINARY" /usr/local/bin/

# Clean up
rm "$TARBALL"

echo "✅ $BINARY installed successfully! Run '$BINARY --help' to get started."
