// Package common provides shared utilities for kyverno-mcp tools.
package common

import (
	"encoding/json"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeConfig returns InCluster config or falls back to ~/.kube/config.
func KubeConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	return clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
}

// ParseNamespaceExcludes builds a set from a comma-separated string.
func ParseNamespaceExcludes(s string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, ns := range strings.Split(s, ",") {
		if ns = strings.TrimSpace(ns); ns != "" {
			set[ns] = struct{}{}
		}
	}
	return set
}

// MustJSON indents or panics (good for quick helpers, optional).
func MustJSON(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(raw)
}
