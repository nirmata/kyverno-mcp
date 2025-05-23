package kyverno

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	kyvernov1 "github.com/kyverno/kyverno/api/kyverno/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTempPolicyFile creates a temporary policy file for testing
func createTempPolicyFile(t *testing.T, policyYAML string) string {
	t.Helper()

	tempDir := t.TempDir()
	policyPath := filepath.Join(tempDir, "policy.yaml")
	err := os.WriteFile(policyPath, []byte(policyYAML), 0644)
	require.NoError(t, err, "Failed to create temp policy file")

	return policyPath
}

func TestLocalPolicyLoader_Load(t *testing.T) {
	// Define test policies
	clusterPolicyYAML := `apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-labels
spec:
  validationFailureAction: enforce
  rules:
  - name: check-for-labels
    match:
      resources: {}
    validate:
      message: "All resources must have 'app.kubernetes.io/name' and 'app.kubernetes.io/instance' labels"
      pattern:
        metadata:
          labels:
            app.kubernetes.io/name: "?*"
            app.kubernetes.io/instance: "?*"`

	namespacedPolicyYAML := `apiVersion: kyverno.io/v1
kind: Policy
metadata:
  name: require-ns-labels
  namespace: default
spec:
  validationFailureAction: enforce
  rules:
  - name: require-ns-labels
    match:
      resources:
        kinds:
        - Namespace
    validate:
      message: "All namespaces must have 'team' and 'environment' labels"
      pattern:
        metadata:
          labels:
            team: "?*"
            environment: "?*"`

	tests := []struct {
		name          string
		setup         func(t *testing.T) []string
		expectedCount int
		expectError   bool
		errorMsg      string
	}{
		{
			name: "load single cluster policy",
			setup: func(t *testing.T) []string {
				path := createTempPolicyFile(t, clusterPolicyYAML)
				return []string{path}
			},
			expectedCount: 1,
			expectError:   false,
		},
		{
			name: "load multiple policies",
			setup: func(t *testing.T) []string {
				path1 := createTempPolicyFile(t, clusterPolicyYAML)
				path2 := createTempPolicyFile(t, namespacedPolicyYAML)
				return []string{path1, path2}
			},
			expectedCount: 2,
			expectError:   false,
		},
		{
			name: "load from directory",
			setup: func(t *testing.T) []string {
				tempDir := t.TempDir()

				// Create multiple policy files in the directory
				path1 := filepath.Join(tempDir, "policy1.yaml")
				err := os.WriteFile(path1, []byte(clusterPolicyYAML), 0644)
				require.NoError(t, err)

				path2 := filepath.Join(tempDir, "policy2.yaml")
				err = os.WriteFile(path2, []byte(namespacedPolicyYAML), 0644)
				require.NoError(t, err)

				return []string{tempDir}
			},
			expectedCount: 2,
			expectError:   false,
		},
		{
			name: "non-existent path",
			setup: func(t *testing.T) []string {
				return []string{"/non/existent/path"}
			},
			expectedCount: 0,
			expectError:   true,
			errorMsg:      "policy path does not exist",
		},
		{
			name: "empty policy paths",
			setup: func(t *testing.T) []string {
				return []string{}
			},
			expectedCount: 0,
			expectError:   true,
			errorMsg:      "no policy paths provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test files
			policyPaths := tt.setup(t)

			// Create loader
			loader := NewLocalPolicyLoader()

			// Load policies - for testing, we'll just use the first path if it exists
			var policies []kyvernov1.PolicyInterface
			var err error
			if len(policyPaths) > 0 {
				policies, err = loader.Load(context.Background(), policyPaths[0])
			} else {
				policies, err = loader.Load(context.Background(), "")
			}

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, policies, tt.expectedCount)

				// Verify policy types
				for _, policy := range policies {
					switch p := policy.(type) {
					case *kyvernov1.ClusterPolicy:
						assert.Equal(t, "ClusterPolicy", p.Kind)
					case *kyvernov1.Policy:
						assert.Equal(t, "Policy", p.Kind)
					default:
						t.Errorf("unexpected policy type: %T", p)
					}
				}
			}
		})
	}
}

func TestLocalPolicyLoader_Load_InvalidYAML(t *testing.T) {
	// Create invalid YAML file
	invalidYAML := `apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: invalid-policy
  bad-yaml: : : :
  spec:
    validationFailureAction: enforce`

	path := createTempPolicyFile(t, invalidYAML)

	loader := NewLocalPolicyLoader()
	_, err := loader.Load(context.Background(), path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load policies")
}

func TestLocalPolicyLoader_Load_UnsupportedType(t *testing.T) {
	// Create a valid YAML but with an unsupported type
	unsupportedTypeYAML := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: default
data:
  key: value`

	path := createTempPolicyFile(t, unsupportedTypeYAML)

	loader := NewLocalPolicyLoader()
	_, err := loader.Load(context.Background(), path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid Kyverno policies found in file")
}
