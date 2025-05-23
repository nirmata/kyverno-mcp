package utils

import (
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// TestGetDynamicClient_InCluster tests the in-cluster configuration path
func TestGetDynamicClient_InCluster(t *testing.T) {
	// Save original function and restore it at the end
	oldInClusterConfig := inClusterConfig
	defer func() { inClusterConfig = oldInClusterConfig }()

	// Mock the in-cluster config function
	inClusterConfig = func() (*rest.Config, error) {
		return &rest.Config{
			Host: "https://test-cluster:443",
		}, nil
	}

	// Test with empty kubeconfig path (should use in-cluster config)
	client, err := GetDynamicClient("")
	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}

	// Verify the client was created
	if client == nil {
		t.Error("Expected a non-nil client, but got nil")
	}
}

// TestGetDynamicClient_WithKubeconfig tests the kubeconfig path
func TestGetDynamicClient_WithKubeconfig(t *testing.T) {
	// Create a temporary kubeconfig file
	tempDir := t.TempDir()
	kubeconfigPath := filepath.Join(tempDir, "kubeconfig")

	// Create a minimal kubeconfig
	config := api.Config{
		Clusters: map[string]*api.Cluster{
			"test-cluster": {
				Server: "https://test-cluster:443",
			},
		},
		Contexts: map[string]*api.Context{
			"test-context": {
				Cluster:  "test-cluster",
				AuthInfo: "test-user",
			},
		},
		CurrentContext: "test-context",
		AuthInfos: map[string]*api.AuthInfo{
			"test-user": {},
		},
	}

	// Write the kubeconfig to a file
	if err := clientcmd.WriteToFile(config, kubeconfigPath); err != nil {
		t.Fatalf("Error writing kubeconfig: %v", err)
	}

	// Test with the kubeconfig path
	client, err := GetDynamicClient(kubeconfigPath)
	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}

	// Verify the client was created
	if client == nil {
		t.Error("Expected a non-nil client, but got nil")
	}
}

// TestGetDynamicClient_Error tests error conditions
func TestGetDynamicClient_Error(t *testing.T) {
	// Test with non-existent kubeconfig path
	_, err := GetDynamicClient("/non/existent/kubeconfig")
	if err == nil {
		t.Error("Expected an error for non-existent kubeconfig, but got none")
	}
}

// inClusterConfig is a variable that holds the function to get in-cluster config
// This allows us to mock it in tests
// var inClusterConfig = rest.InClusterConfig

func TestGVKToGVR(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		kind       string
		expected   schema.GroupVersionResource
		err        bool
	}{
		{
			name:       "core v1 Pod",
			apiVersion: "v1",
			kind:       "Pod",
			expected:   schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			err:        false,
		},
		{
			name:       "apps v1 Deployment",
			apiVersion: "apps/v1",
			kind:       "Deployment",
			expected:   schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
			err:        false,
		},
		{
			name:       "batch v1 Job",
			apiVersion: "batch/v1",
			kind:       "Job",
			expected:   schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"},
			err:        false,
		},
		{
			name:       "networking.k8s.io v1 Ingress",
			apiVersion: "networking.k8s.io/v1",
			kind:       "Ingress",
			expected:   schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
			err:        false,
		},
		{
			name:       "rbac.authorization.k8s.io v1 Role",
			apiVersion: "rbac.authorization.k8s.io/v1",
			kind:       "Role",
			expected:   schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
			err:        false,
		},
		{
			name:       "empty apiVersion should default to v1",
			apiVersion: "",
			kind:       "Pod",
			expected:   schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			err:        false,
		},
		{
			name:       "non-existent resource",
			apiVersion: "example.com/v1",
			kind:       "NonExistent",
			expected:   schema.GroupVersionResource{},
			err:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GVKToGVR(tt.apiVersion, tt.kind)
			
			if tt.err {
				if err == nil {
					t.Error("expected error, but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			
			if result != tt.expected {
				t.Errorf("expected %#v, got %#v", tt.expected, result)
			}
		})
	}
}
