package kyverno

import (
	"context"
	"testing"

	"github.com/kyverno/go-kyverno-mcp/pkg/mcp/types"
	kyvernov1 "github.com/kyverno/kyverno/api/kyverno/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type mockPolicyLoader struct {
	policies []kyvernov1.PolicyInterface
	err      error
}

func (m *mockPolicyLoader) Load(_ context.Context, _ string) ([]kyvernov1.PolicyInterface, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.policies, nil
}

type mockResourceLoader struct {
	resources []*unstructured.Unstructured
	err       error
}

func (m *mockResourceLoader) Load(_ context.Context, _ []types.ResourceQuery, _ string) ([]*unstructured.Unstructured, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.resources, nil
}

func TestKyvernoEngine_ApplyPolicies(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func() (*mockPolicyLoader, *mockResourceLoader)
		expectedError  string
		expectedResult *types.ApplyResponse
	}{
		{
			name: "successful policy application",
			setupMocks: func() (*mockPolicyLoader, *mockResourceLoader) {
				policy := &kyvernov1.ClusterPolicy{
					Spec: kyvernov1.Spec{
						Rules: []kyvernov1.Rule{
							{
								Name: "test-rule",
								MatchResources: kyvernov1.MatchResources{
									ResourceDescription: kyvernov1.ResourceDescription{
										Kinds: []string{"Pod"},
									},
								},
								Validation: &kyvernov1.Validation{
									Message: "Test validation rule",
								},
							},
						},
					},
				}

				resource := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Pod",
						"metadata": map[string]interface{}{
							"name":      "test-pod",
							"namespace": "default",
							"uid":       "test-uid",
						},
					},
				}

				return &mockPolicyLoader{
						policies: []kyvernov1.PolicyInterface{policy},
					}, &mockResourceLoader{
						resources: []*unstructured.Unstructured{resource},
					}
			},
			expectedResult: &types.ApplyResponse{
				Results: []types.PolicyApplicationResult{
					{
						Policy: "", // Policy name will be empty in the mock
						Resource: types.ResourceInfo{
							APIVersion: "v1",
							Kind:       "Pod",
							Namespace:  "default",
							Name:       "test-pod",
							UID:        "test-uid",
						},
						Rules: []types.RuleResult{
							{
								Name:    "test-rule",
								Type:    "validate",
								Message: "Policy rule evaluated",
								Status:  "pass",
							},
						},
					},
				},
				Resources: []*unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Pod",
							"metadata": map[string]interface{}{
								"name":      "test-pod",
								"namespace": "default",
								"uid":       "test-uid",
							},
						},
					},
				},
			},
		},
		{
			name: "no policies to apply",
			setupMocks: func() (*mockPolicyLoader, *mockResourceLoader) {
				return &mockPolicyLoader{
						policies: []kyvernov1.PolicyInterface{},
					}, &mockResourceLoader{
						resources: []*unstructured.Unstructured{},
					}
			},
			expectedResult: &types.ApplyResponse{
				Results:   []types.PolicyApplicationResult{},
				Resources: []*unstructured.Unstructured{},
			},
		},
		{
			name: "no resources to apply policies to",
			setupMocks: func() (*mockPolicyLoader, *mockResourceLoader) {
				policy := &kyvernov1.ClusterPolicy{
					Spec: kyvernov1.Spec{
						Rules: []kyvernov1.Rule{
							{
								Name: "test-rule",
								MatchResources: kyvernov1.MatchResources{
									ResourceDescription: kyvernov1.ResourceDescription{
										Kinds: []string{"Pod"},
									},
								},
								Validation: &kyvernov1.Validation{
									Message: "Test validation rule",
								},
							},
						},
					},
				}

				return &mockPolicyLoader{
						policies: []kyvernov1.PolicyInterface{policy},
					}, &mockResourceLoader{
						resources: []*unstructured.Unstructured{},
					}
			},
			expectedResult: &types.ApplyResponse{
				Results:   []types.PolicyApplicationResult{},
				Resources: []*unstructured.Unstructured{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policyLoader, resourceLoader := tt.setupMocks()
			engine := NewKyvernoEngine(policyLoader, resourceLoader, "")

			result, err := engine.ApplyPolicies(context.Background(), &types.ApplyRequest{
				ResourceQueries: []types.ResourceQuery{
					{
						APIVersion: "v1",
						Kind:       "Pod",
						Namespace:  "default",
					},
				},
			})

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestNewKyvernoEngine(t *testing.T) {
	policyLoader := &mockPolicyLoader{}
	resourceLoader := &mockResourceLoader{}
	serverKubeconfigPath := "/path/to/kubeconfig"

	engine := NewKyvernoEngine(policyLoader, resourceLoader, serverKubeconfigPath)

	// Verify that the engine was created with the correct dependencies
	assert.NotNil(t, engine)
}
