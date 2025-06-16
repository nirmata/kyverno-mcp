#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail


REPO_ROOT="$(git rev-parse --show-toplevel)"
cd ${REPO_ROOT}

find . -name "*.go" | xargs go run github.com/google/addlicense@master -c "Google LLC" -l apache

for f in $(find ${REPO_ROOT} -name go.mod); do
  cd $(dirname ${f})
  gofmt -w .
done