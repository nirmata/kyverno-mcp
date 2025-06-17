// Package kyverno provides a shim for the Kyverno CLI.
package kyverno

import (
	"time"

	"github.com/kyverno/kyverno/api/kyverno"
	policyreportv1alpha2 "github.com/kyverno/kyverno/api/policyreport/v1alpha2"
	engineapi "github.com/kyverno/kyverno/pkg/engine/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildPolicyReportResults builds policy report results from engine responses
func BuildPolicyReportResults(auditWarn bool, engineResponses ...engineapi.EngineResponse) []policyreportv1alpha2.PolicyReportResult {
	var results []policyreportv1alpha2.PolicyReportResult
	now := metav1.Timestamp{Seconds: time.Now().Unix()}
	for _, engineResponse := range engineResponses {
		policyName := engineResponse.Policy().GetName()
		annotations := engineResponse.Policy().GetAnnotations()
		scored := true
		if policyScored, ok := annotations[kyverno.AnnotationPolicyScored]; ok {
			if policyScored == "false" {
				scored = false
			}
		}
		category := annotations[kyverno.AnnotationPolicyCategory]
		severity := annotations[kyverno.AnnotationPolicySeverity]
		for _, ruleResponse := range engineResponse.PolicyResponse.Rules {
			if ruleResponse.RuleType() != engineapi.Validation {
				continue
			}
			if ruleResponse.Status() == engineapi.RuleStatusPass || ruleResponse.Status() == engineapi.RuleStatusSkip {
				continue
			}
			result := policyreportv1alpha2.PolicyReportResult{
				Policy: policyName,
				Rule:   ruleResponse.Name(),
				Resources: []corev1.ObjectReference{{
					Kind:       engineResponse.Resource.GetKind(),
					Namespace:  engineResponse.Resource.GetNamespace(),
					APIVersion: engineResponse.Resource.GetAPIVersion(),
					Name:       engineResponse.Resource.GetName(),
					UID:        engineResponse.Resource.GetUID(),
				}},
				Scored:  scored,
				Message: ruleResponse.Message(),
			}

			// Determine the result status. Pass and Skip statuses are already filtered out earlier.
			if ruleResponse.Status() == engineapi.RuleStatusError {
				result.Result = policyreportv1alpha2.StatusError
			} else if ruleResponse.Status() == engineapi.RuleStatusFail {
				if !scored {
					result.Result = policyreportv1alpha2.StatusWarn
				} else if auditWarn && engineResponse.GetValidationFailureAction().Audit() {
					result.Result = policyreportv1alpha2.StatusWarn
				} else {
					result.Result = policyreportv1alpha2.StatusFail
				}
			} else if ruleResponse.Status() == engineapi.RuleStatusWarn {
				result.Result = policyreportv1alpha2.StatusWarn
			} else {
				// Fallback: treat any unforeseen status as an error to surface the issue clearly
				result.Result = policyreportv1alpha2.StatusError
			}
			result.Source = kyverno.ValueKyvernoApp
			result.Timestamp = now
			result.Category = category
			result.Severity = policyreportv1alpha2.PolicySeverity(severity)
			results = append(results, result)
		}
	}
	if results == nil {
		results = []policyreportv1alpha2.PolicyReportResult{
			{
				Policy:  "No policies applied",
				Rule:    "No policies applied",
				Result:  policyreportv1alpha2.StatusSkip,
				Message: "No policies applied",
			},
		}
	}
	return results
}
