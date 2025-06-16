#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail


REPO_ROOT="$(git rev-parse --show-toplevel)"
cd ${REPO_ROOT}

for f in $(find ${REPO_ROOT} -name go.mod); do
  cd $(dirname ${f})
  go build -o $(dirname ${f})/bin/kyverno-mcp
done