package utils

import (
	"context"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

// MockDynamicClient is a mock implementation of dynamic.Interface
type MockDynamicClient struct {
	mock.Mock
}

func (m *MockDynamicClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	args := m.Called(resource)
	return args.Get(0).(dynamic.NamespaceableResourceInterface)
}

// MockResourceInterface is a mock implementation of dynamic.ResourceInterface
type MockResourceInterface struct {
	mock.Mock
}

func (m *MockResourceInterface) Namespace(ns string) dynamic.ResourceInterface {
	args := m.Called(ns)
	return args.Get(0).(dynamic.ResourceInterface)
}

func (m *MockResourceInterface) Create(ctx context.Context, obj *unstructured.Unstructured, options metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	args := m.Called(ctx, obj, options, subresources)
	return args.Get(0).(*unstructured.Unstructured), args.Error(1)
}

func (m *MockResourceInterface) Update(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	args := m.Called(ctx, obj, options, subresources)
	return args.Get(0).(*unstructured.Unstructured), args.Error(1)
}

func (m *MockResourceInterface) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	args := m.Called(ctx, obj, options)
	return args.Get(0).(*unstructured.Unstructured), args.Error(1)
}

func (m *MockResourceInterface) Delete(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
	args := m.Called(ctx, name, options, subresources)
	return args.Error(0)
}

func (m *MockResourceInterface) DeleteCollection(ctx context.Context, options metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	args := m.Called(ctx, options, listOptions)
	return args.Error(0)
}

func (m *MockResourceInterface) Get(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	args := m.Called(ctx, name, options, subresources)
	return args.Get(0).(*unstructured.Unstructured), args.Error(1)
}

func (m *MockResourceInterface) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*unstructured.UnstructuredList), args.Error(1)
}

func (m *MockResourceInterface) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(watch.Interface), args.Error(1)
}

func (m *MockResourceInterface) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	args := m.Called(ctx, name, pt, data, options, subresources)
	return args.Get(0).(*unstructured.Unstructured), args.Error(1)
}

// MockRESTMapper is a mock implementation of meta.RESTMapper
type MockRESTMapper struct {
	mock.Mock
}

func (m *MockRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	args := m.Called(resource)
	return args.Get(0).(schema.GroupVersionKind), args.Error(1)
}

func (m *MockRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	args := m.Called(resource)
	return args.Get(0).([]schema.GroupVersionKind), args.Error(1)
}

func (m *MockRESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	args := m.Called(input)
	return args.Get(0).(schema.GroupVersionResource), args.Error(1)
}

func (m *MockRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	args := m.Called(input)
	return args.Get(0).([]schema.GroupVersionResource), args.Error(1)
}

func (m *MockRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	args := m.Called(gk, versions)
	return args.Get(0).(*meta.RESTMapping), args.Error(1)
}

func (m *MockRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	args := m.Called(gk, versions)
	return args.Get(0).([]*meta.RESTMapping), args.Error(1)
}

func (m *MockRESTMapper) ResourceSingularizer(resource string) (singular string, err error) {
	args := m.Called(resource)
	return args.String(0), args.Error(1)
}

// Helper function to create a simple RESTMapper for testing
func NewSimpleRESTMapper() meta.RESTMapper {
	// Create a default REST mapper with no special behavior
	return meta.NewDefaultRESTMapper([]schema.GroupVersion{})
}
