package kyverno

import (
	"context"
	"testing"

	mcpTypes "github.com/kyverno/go-kyverno-mcp/pkg/mcp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

// mockDynamicClient is a mock implementation of dynamic.Interface
type mockDynamicClient struct {
	resourceFunc func(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface
}

func (m *mockDynamicClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	if m.resourceFunc != nil {
		return m.resourceFunc(resource)
	}
	return nil
}

// mockResourceInterface is a mock implementation of dynamic.ResourceInterface
type mockResourceInterface struct {
	getFunc   func(context.Context, string, metav1.GetOptions, ...string) (*unstructured.Unstructured, error)
	listFunc  func(context.Context, metav1.ListOptions) (*unstructured.UnstructuredList, error)
	patchFunc func(context.Context, string, types.PatchType, []byte, metav1.PatchOptions, ...string) (*unstructured.Unstructured, error)
}

func (m *mockResourceInterface) Get(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, name, opts, subresources...)
	}
	return nil, nil
}

func (m *mockResourceInterface) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, opts)
	}
	return &unstructured.UnstructuredList{}, nil
}

func (m *mockResourceInterface) Create(ctx context.Context, obj *unstructured.Unstructured, options metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (m *mockResourceInterface) Update(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (m *mockResourceInterface) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (m *mockResourceInterface) Delete(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
	return nil
}

func (m *mockResourceInterface) DeleteCollection(ctx context.Context, options metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return nil
}

func (m *mockResourceInterface) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (m *mockResourceInterface) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	if m.patchFunc != nil {
		return m.patchFunc(ctx, name, pt, data, opts, subresources...)
	}
	return nil, nil
}

func (m *mockResourceInterface) Apply(ctx context.Context, name string, obj *unstructured.Unstructured, options metav1.ApplyOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (m *mockResourceInterface) ApplyStatus(ctx context.Context, name string, obj *unstructured.Unstructured, options metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (m *mockResourceInterface) Namespace(ns string) dynamic.ResourceInterface {
	return m
}

// TestAPILoader_Load tests the Load method of apiResourceLoader
func TestAPILoader_Load(t *testing.T) {
	// Create test resources
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "test-deployment",
				"namespace": "default",
			},
		},
	}

	pod1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "pod-1",
				"namespace": "default",
				"labels": map[string]interface{}{
					"app": "test",
				},
			},
		},
	}

	pod2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "pod-2",
				"namespace": "default",
				"labels": map[string]interface{}{
					"app": "test",
				},
			},
		},
	}

	namespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "test-namespace",
			},
		},
	}

	tests := []struct {
		name          string
		setupMocks    func() (dynamic.Interface, func(*testing.T))
		queries       []mcpTypes.ResourceQuery
		expectedError string
		expectedCount int
	}{
		{
			name: "successful get single resource",
			setupMocks: func() (dynamic.Interface, func(*testing.T)) {
				mockRI := &mockResourceInterface{
					getFunc: func(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
						return deployment, nil
					},
				}

				mockDC := &mockDynamicClient{
					resourceFunc: func(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
						return mockRI
					},
				}

				return mockDC, func(t *testing.T) {
					assert.Equal(t, "test-deployment", deployment.GetName())
				}
			},
			queries: []mcpTypes.ResourceQuery{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test-deployment",
					Namespace:  "default",
				},
			},
			expectedError: "",
			expectedCount: 1,
		},
		{
			name: "list resources with label selector",
			setupMocks: func() (dynamic.Interface, func(*testing.T)) {
				mockRI := &mockResourceInterface{
					listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
						assert.Equal(t, "app=test", opts.LabelSelector)
						return &unstructured.UnstructuredList{
							Items: []unstructured.Unstructured{*pod1, *pod2},
						}, nil
					},
				}

				mockDC := &mockDynamicClient{
					resourceFunc: func(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
						return mockRI
					},
				}

				return mockDC, nil
			},
			queries: []mcpTypes.ResourceQuery{
				{
					APIVersion:   "v1",
					Kind:         "Pod",
					Namespace:    "default",
					LabelSelector: "app=test",
				},
			},
			expectedError: "",
			expectedCount: 2,
		},
		{
			name: "cluster-scoped resource",
			setupMocks: func() (dynamic.Interface, func(*testing.T)) {
				mockRI := &mockResourceInterface{
					getFunc: func(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
						return namespace, nil
					},
				}

				mockDC := &mockDynamicClient{
					resourceFunc: func(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
						// Should not use namespace for cluster-scoped resources
						return mockRI
					},
				}

				return mockDC, nil
			},
			queries: []mcpTypes.ResourceQuery{
				{
					APIVersion: "v1",
					Kind:       "Namespace",
					Name:       "test-namespace",
				},
			},
			expectedError: "",
			expectedCount: 1,
		},
		{
			name: "resource not found",
			setupMocks: func() (dynamic.Interface, func(*testing.T)) {
				mockRI := &mockResourceInterface{
					getFunc: func(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
						return nil, k8serrors.NewNotFound(schema.GroupResource{
							Group:    "apps",
							Resource: "deployments",
						}, "non-existent")
					},
				}

				mockDC := &mockDynamicClient{
					resourceFunc: func(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
						return mockRI
					},
				}

				return mockDC, nil
			},
			queries: []mcpTypes.ResourceQuery{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "non-existent",
					Namespace:  "default",
				},
			},
			expectedError: "not found",
			expectedCount: 0,
		},
		{
			name: "invalid GVK",
			setupMocks: func() (dynamic.Interface, func(*testing.T)) {
				// Should fail before making any API calls
				return &mockDynamicClient{}, nil
			},
			queries: []mcpTypes.ResourceQuery{
				{
					APIVersion: "invalid/version",
					Kind:       "InvalidKind",
					Name:       "test",
				},
			},
			expectedError: "failed to get GVR",
			expectedCount: 0,
		},
		{
			name: "multiple queries",
			setupMocks: func() (dynamic.Interface, func(*testing.T)) {
				mockRI := &mockResourceInterface{
					getFunc: func(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
						switch name {
						case "test-deployment":
							return deployment, nil
						case "test-namespace":
							return namespace, nil
						default:
							return nil, k8serrors.NewNotFound(schema.GroupResource{}, name)
						}
					},
				}

				mockDC := &mockDynamicClient{
					resourceFunc: func(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
						return mockRI
					},
				}

				return mockDC, nil
			},
			queries: []mcpTypes.ResourceQuery{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test-deployment",
					Namespace:  "default",
				},
				{
					APIVersion: "v1",
					Kind:       "Namespace",
					Name:       "test-namespace",
				},
			},
			expectedError: "",
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			dc, verify := tt.setupMocks()

			// Create loader with mock client getter
			loader := &apiResourceLoader{
				getDynamicClient: func(kubeconfigPath string) (dynamic.Interface, error) {
					return dc, nil
				},
			}

			// Execute test queries
			resources, err := loader.Load(context.Background(), tt.queries, "")

			// Verify results
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Len(t, resources, tt.expectedCount)
			}

			// Run any additional verifications
			if verify != nil {
				verify(t)
			}
		})
	}
}
