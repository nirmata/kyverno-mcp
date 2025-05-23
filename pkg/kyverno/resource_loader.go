// resource_loader.go
package kyverno

import (
	"context"
	"fmt"
	"strings"

	"github.com/kyverno/go-kyverno-mcp/pkg/mcp/types"
	"github.com/kyverno/go-kyverno-mcp/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// ResourceLoader is an interface for loading Kubernetes resources
// from different sources (e.g., Kubernetes API, filesystem, etc.)
type ResourceLoader interface {
	// Load loads Kubernetes resources based on the provided resource queries
	// and kubeconfig path. Returns a list of unstructured resources or an error.
	Load(ctx context.Context, queries []types.ResourceQuery, kubeconfigPath string) ([]*unstructured.Unstructured, error)
}

// apiResourceLoader is an implementation of ResourceLoader that loads resources
// from the Kubernetes API server using a dynamic client
type apiResourceLoader struct {
	// getDynamicClient is a function that returns a dynamic.Interface
	// This is a field to allow for easier testing
	getDynamicClient func(kubeconfigPath string) (dynamic.Interface, error)
}

// NewAPIResourceLoader creates a new instance of apiResourceLoader
func NewAPIResourceLoader() ResourceLoader {
	return &apiResourceLoader{
		getDynamicClient: utils.GetDynamicClient,
	}
}

// validateQuery validates the resource query parameters
func validateQuery(query types.ResourceQuery) error {
	if query.APIVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if query.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	return nil
}

// getResourceInterface returns the appropriate resource interface for the given query
func (l *apiResourceLoader) getResourceInterface(
	client dynamic.Interface,
	gvr schema.GroupVersionResource,
	namespace string,
) (dynamic.ResourceInterface, error) {
	// Get the appropriate resource interface
	var ri dynamic.ResourceInterface
	if namespace != "" {
		ri = client.Resource(gvr).Namespace(namespace)
	} else {
		ri = client.Resource(gvr)
	}
	return ri, nil
}

// processSingleQuery processes a single resource query and returns the matching resources
func (l *apiResourceLoader) processSingleQuery(
	ctx context.Context,
	client dynamic.Interface,
	query types.ResourceQuery,
) ([]*unstructured.Unstructured, error) {
	// Validate the query
	if err := validateQuery(query); err != nil {
		return nil, fmt.Errorf("invalid resource query: %w", err)
	}

	// Convert GVK to GVR
	gvr, err := utils.GVKToGVR(query.APIVersion, query.Kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get GVR for apiVersion=%s, kind=%s: %w",
			query.APIVersion, query.Kind, err)
	}

	// Get the appropriate resource interface
	ri, err := l.getResourceInterface(client, gvr, query.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource interface: %w", err)
	}

	// Handle the query based on whether a name is provided
	if query.Name != "" {
		return l.getSingleResource(ctx, ri, query)
	}
	return l.listResources(ctx, ri, query)
}

// getSingleResource retrieves a single resource by name
func (l *apiResourceLoader) getSingleResource(
	ctx context.Context,
	ri dynamic.ResourceInterface,
	query types.ResourceQuery,
) ([]*unstructured.Unstructured, error) {
	resource, err := ri.Get(ctx, query.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("Resource not found: %s/%s in namespace %s",
				query.Kind, query.Name, query.Namespace)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get %s %s in namespace %s: %w",
			strings.ToLower(query.Kind), query.Name, query.Namespace, err)
	}
	return []*unstructured.Unstructured{resource}, nil
}

// listResources lists resources with optional label and field selectors
func (l *apiResourceLoader) listResources(
	ctx context.Context,
	ri dynamic.ResourceInterface,
	query types.ResourceQuery,
) ([]*unstructured.Unstructured, error) {
	listOptions := metav1.ListOptions{}

	// Apply label selector if provided
	if query.LabelSelector != "" {
		listOptions.LabelSelector = query.LabelSelector
	}

	// Apply field selector if provided
	if query.FieldSelector != "" {
		listOptions.FieldSelector = query.FieldSelector
	}

	resourceList, err := ri.List(ctx, listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	// Convert the list items to a slice of unstructured resources
	resources := make([]*unstructured.Unstructured, 0, len(resourceList.Items))
	for i := range resourceList.Items {
		resources = append(resources, &resourceList.Items[i])
	}

	return resources, nil
}

// Load implements the ResourceLoader interface by loading resources from the Kubernetes API
func (l *apiResourceLoader) Load(
	ctx context.Context,
	queries []types.ResourceQuery,
	kubeconfigPath string,
) ([]*unstructured.Unstructured, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("at least one resource query is required")
	}

	// Create a dynamic client
	client, err := l.getDynamicClient(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	var allResources []*unstructured.Unstructured

	// Process each query
	for i, query := range queries {
		resources, err := l.processSingleQuery(ctx, client, query)
		if err != nil {
			return nil, fmt.Errorf("error processing query %d: %w", i, err)
		}
		allResources = append(allResources, resources...)
	}

	return allResources, nil
}
