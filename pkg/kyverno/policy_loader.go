package kyverno

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	kyvernov1 "github.com/kyverno/kyverno/api/kyverno/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// PolicyLoader is an interface for loading Kyverno policies from different sources
type PolicyLoader interface {
	// Load loads Kyverno policies from the specified path
	// Returns a slice of PolicyInterface and an error if any
	Load(ctx context.Context, path string) ([]kyvernov1.PolicyInterface, error)
}

// LocalPolicyLoader loads policies from local files
type LocalPolicyLoader struct{}

// NewLocalPolicyLoader creates a new LocalPolicyLoader
func NewLocalPolicyLoader() *LocalPolicyLoader {
	return &LocalPolicyLoader{}
}

// Load loads policies from the given file path
func (l *LocalPolicyLoader) Load(ctx context.Context, path string) ([]kyvernov1.PolicyInterface, error) {
	log.Printf("Attempting to load policy from: %s", path)
	
	// Clean the path to handle any . or .. or //
	path = filepath.Clean(path)
	
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("policy file does not exist: %s", path)
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file %s: %w", path, err)
	}

	// First try to unmarshal as a single policy
	var policy kyvernov1.ClusterPolicy
	if err := yaml.Unmarshal(data, &policy); err == nil && policy.APIVersion != "" {
		log.Printf("Successfully loaded policy: %s", policy.Name)
		return []kyvernov1.PolicyInterface{&policy}, nil
	}

	// If that fails, try to unmarshal as a list of policies
	var policyList kyvernov1.ClusterPolicyList
	if err := yaml.Unmarshal(data, &policyList); err == nil && len(policyList.Items) > 0 {
		log.Printf("Successfully loaded %d policies", len(policyList.Items))
		result := make([]kyvernov1.PolicyInterface, 0, len(policyList.Items))
		for i := range policyList.Items {
			result = append(result, &policyList.Items[i])
		}
		return result, nil
	}

	return nil, fmt.Errorf("failed to parse policy file %s: not a valid Kyverno policy", path)
}

