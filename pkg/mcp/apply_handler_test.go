package mcp

import (
	"context"
	"testing"

	"github.com/kyverno/go-kyverno-mcp/pkg/mcp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockEngine struct {
	applyPoliciesFunc func(ctx context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error)
}

func (m *mockEngine) ApplyPolicies(ctx context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error) {
	return m.applyPoliciesFunc(ctx, req)
}

func TestApplyService_ProcessApplyRequest(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func() *mockEngine
		expectedResult *types.ApplyResponse
		expectedError  string
	}{
		{
			name: "successful request processing",
			setupMocks: func() *mockEngine {
				return &mockEngine{
					applyPoliciesFunc: func(_ context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error) {
						return &types.ApplyResponse{
							Results: []types.PolicyApplicationResult{
								{
									Policy: "test-policy",
									Resource: types.ResourceInfo{
										APIVersion: "v1",
										Kind:       "Pod",
										Name:       "test-pod",
										Namespace:  "default",
									},
									Rules: []types.RuleResult{
										{
											Name:    "test-rule",
											Type:    "validate",
											Message: "Test message",
											Status:  "pass",
										},
									},
								},
							},
						}, nil
					},
				}
			},
			expectedResult: &types.ApplyResponse{
				Results: []types.PolicyApplicationResult{
					{
						Policy: "test-policy",
						Resource: types.ResourceInfo{
							APIVersion: "v1",
							Kind:       "Pod",
							Name:       "test-pod",
							Namespace:  "default",
						},
						Rules: []types.RuleResult{
							{
								Name:    "test-rule",
								Type:    "validate",
								Message: "Test message",
								Status:  "pass",
							},
						},
					},
				},
			},
		},
		{
			name: "engine returns error",
			setupMocks: func() *mockEngine {
				return &mockEngine{
					applyPoliciesFunc: func(_ context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error) {
						return nil, assert.AnError
					},
				}
			},
			expectedError: assert.AnError.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockEngine := tt.setupMocks()
			service := NewApplyService(mockEngine)

			result, err := service.ProcessApplyRequest(context.Background(), &types.ApplyRequest{})

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

func TestNewApplyService(t *testing.T) {
	mockEngine := &mockEngine{
		applyPoliciesFunc: func(_ context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error) {
			return &types.ApplyResponse{}, nil
		},
	}

	service := NewApplyService(mockEngine)

	assert.NotNil(t, service)
}
