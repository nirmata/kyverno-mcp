package types

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResourceQuery defines a query for Kubernetes resources
type ResourceQuery struct {
	// APIVersion is the API version of the resource (e.g., "v1", "apps/v1")
	APIVersion string `json:"apiVersion"`
	// Kind is the kind of the resource (e.g., "Pod", "Deployment")
	Kind string `json:"kind"`
	// Namespace is the namespace of the resource (empty for cluster-scoped resources)
	Namespace string `json:"namespace,omitempty"`
	// Name is the name of a specific resource to fetch (if empty, all matching resources are returned)
	Name string `json:"name,omitempty"`
	// LabelSelector is a label selector to filter resources (e.g., "app=my-app,env=prod")
	LabelSelector string `json:"labelSelector,omitempty"`
	// FieldSelector is a field selector to filter resources (e.g., "metadata.name=my-pod")
	FieldSelector string `json:"fieldSelector,omitempty"`
}

// RuleResult represents the result of applying a single Kyverno rule
type RuleResult struct {
	// Name is the name of the rule
	Name string `json:"name"`
	// Type is the type of the rule (e.g., "validate", "mutate", "generate")
	Type string `json:"type"`
	// Message contains the result message
	Message string `json:"message"`
	// Status indicates whether the rule passed validation ("pass", "fail", "error")
	Status string `json:"status"`
}

// ResourceInfo contains information about a Kubernetes resource
type ResourceInfo struct {
	// APIVersion is the API version of the resource
	APIVersion string `json:"apiVersion"`
	// Kind is the kind of the resource
	Kind string `json:"kind"`
	// Namespace is the namespace of the resource (empty for cluster-scoped resources)
	Namespace string `json:"namespace,omitempty"`
	// Name is the name of the resource
	Name string `json:"name"`
	// UID is the unique identifier of the resource
	UID string `json:"uid"`
}

// PolicyApplicationResult represents the result of applying a single policy to a resource
type PolicyApplicationResult struct {
	// Policy is the name of the applied policy
	Policy string `json:"policy"`
	// Resource is the information about the resource the policy was applied to
	Resource ResourceInfo `json:"resource"`
	// Rules contains the results of each rule in the policy
	Rules []RuleResult `json:"rules"`
	// ValidationFailureAction is the action to take when validation fails ("audit" or "enforce")
	ValidationFailureAction string `json:"validationFailureAction,omitempty"`
}

// ApplyRequest represents a request to apply Kyverno policies to resources
type ApplyRequest struct {
	// PolicyPaths defines the paths to Kyverno policy files or directories
	PolicyPaths []string `json:"policyPaths"`
	// ResourceQueries defines the resources to apply policies to
	ResourceQueries []ResourceQuery `json:"resourceQueries"`
	// KubeconfigPath is the path to the kubeconfig file for the target cluster
	KubeconfigPath string `json:"kubeconfigPath,omitempty"`
}

// ApplyResponse represents the response from applying Kyverno policies to resources
type ApplyResponse struct {
	// Results contains the results of applying each policy to each resource
	Results []PolicyApplicationResult `json:"results"`
	// Resources contains the original resources that were processed
	Resources []*unstructured.Unstructured `json:"resources"`
}

// ResourcePath represents a path to a resource on the filesystem (for future use)
type ResourcePath struct {
	Path string `json:"path"`
}

// Cluster represents a Kubernetes cluster configuration (for future use)
type Cluster struct {
	Kubeconfig string `json:"kubeconfig"`
}
