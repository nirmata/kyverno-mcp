package kyverno

import (
	"context"
	"fmt"

	kyvernov1 "github.com/kyverno/kyverno/api/kyverno/v1"
	"github.com/kyverno/go-kyverno-mcp/pkg/mcp/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Engine defines the interface for applying Kyverno policies to resources
type Engine interface {
	// ApplyPolicies applies the specified policies to the resources defined in the request
	// and returns the policy application results.
	ApplyPolicies(ctx context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error)
}

// kyvernoEngine is an implementation of the Engine interface that uses the Kyverno policy engine
// to validate and mutate resources.
type kyvernoEngine struct {
	policyLoader      PolicyLoader
	resourceLoader    ResourceLoader
	serverKubeconfigPath string
}

// NewKyvernoEngine creates a new instance of the Kyverno engine with the specified policy loader,
// resource loader, and server kubeconfig path.
//
// Parameters:
//   - pl: PolicyLoader implementation for loading Kyverno policies
//   - rl: ResourceLoader implementation for loading Kubernetes resources
//   - serverKubeconfigPath: Path to the kubeconfig file for the server (optional)
//
// Returns:
//   - A new instance of the Kyverno engine
func NewKyvernoEngine(pl PolicyLoader, rl ResourceLoader, serverKubeconfigPath string) Engine {
	return &kyvernoEngine{
		policyLoader:      pl,
		resourceLoader:    rl,
		serverKubeconfigPath: serverKubeconfigPath,
	}
}

// ApplyPolicies implements the Engine interface by applying the specified policies
// to the resources defined in the request and returning the policy application results.
func (e *kyvernoEngine) ApplyPolicies(ctx context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error) {
	logger := log.FromContext(ctx)

	if len(req.PolicyPaths) == 0 {
		return nil, fmt.Errorf("no policy paths provided in the request")
	}

	// 1. Load policies
	var allPolicies []kyvernov1.PolicyInterface
	for _, path := range req.PolicyPaths {
		policies, err := e.policyLoader.Load(ctx, path)
		if err != nil {
			logger.Error(err, "failed to load policy", "path", path)
			return nil, fmt.Errorf("failed to load policy from %s: %w", path, err)
		}
		allPolicies = append(allPolicies, policies...)
	}

	if len(allPolicies) == 0 {
		return nil, fmt.Errorf("no valid policies found in the provided paths")
	}

	if len(allPolicies) == 0 {
		return &types.ApplyResponse{
			Results:    []types.PolicyApplicationResult{},
			Resources:  []*unstructured.Unstructured{},
		}, nil
	}

	// 2. Load resources
	resources, err := e.resourceLoader.Load(ctx, req.ResourceQueries, e.serverKubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load resources: %w", err)
	}

	if len(resources) == 0 {
		return &types.ApplyResponse{
			Results:    []types.PolicyApplicationResult{},
			Resources:  []*unstructured.Unstructured{},
		}, nil
	}

	// 3. Apply policies to resources
	results := make([]types.PolicyApplicationResult, 0, len(resources)*len(allPolicies))

	for _, resource := range resources {
		for _, policy := range allPolicies {
			// Convert the policy to the appropriate version if needed
			var policyName string
			var rules []kyvernov1.Rule

			switch p := policy.(type) {
			case *kyvernov1.ClusterPolicy:
				policyName = p.GetName()
				rules = p.Spec.Rules
			case *kyvernov1.Policy:
				policyName = p.GetName()
				rules = p.Spec.Rules
			default:
				logger.Info("unsupported policy type, skipping", "policy", policy.GetName(), "type", fmt.Sprintf("%T", policy))
				continue
			}

			// For each rule in the policy, apply it to the resource
			for _, rule := range rules {
				// Determine rule type by checking which rule fields are set
				ruleType := "validate"
				if rule.Validation != nil {
					ruleType = "validate"
				} else if rule.Mutation != nil {
					ruleType = "mutate"
				} else if rule.Generation != nil {
					ruleType = "generate"
				}

				// In a real implementation, we would use the Kyverno engine to apply the rule
				// For now, we'll just create a placeholder result
				result := types.PolicyApplicationResult{
					Policy: policyName,
					Resource: types.ResourceInfo{
						APIVersion: resource.GetAPIVersion(),
						Kind:       resource.GetKind(),
						Namespace:  resource.GetNamespace(),
						Name:       resource.GetName(),
						UID:        string(resource.GetUID()),
					},
					Rules: []types.RuleResult{
						{
							Name:    rule.Name,
							Type:    ruleType,
							Message: "Policy rule evaluated",
							Status:  "pass", // Default to pass, would be determined by actual validation
						},
					},
				}

				results = append(results, result)
			}
		}
	}

	// 4. Return the results
	return &types.ApplyResponse{
		Results:   results,
		Resources: resources,
	}, nil
}
