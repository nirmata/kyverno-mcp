package kyverno

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"regexp"

	// blank import
	_ "unsafe"

	"github.com/golang/glog"
	"github.com/kyverno/kyverno/cmd/cli/kubectl-kyverno/commands/apply"
	"github.com/kyverno/kyverno/cmd/cli/kubectl-kyverno/processor"
	engineapi "github.com/kyverno/kyverno/pkg/engine/api"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Compile the regular expression
var re = regexp.MustCompile(`Applying \d+ policy rule\(s\) to \d+ resource\(s\)`)

// ApplyResult represents the result of applying policies to resources
type ApplyResult struct {
	ResultCounts               *processor.ResultCounts
	Unstructured               []*unstructured.Unstructured
	SkippedInvalidPolicies     apply.SkippedInvalidPolicies
	EngineResponses            []engineapi.EngineResponse
	PolicyResourceMappingCount string
}

// ApplyCommandHelper applies policies to resources
func ApplyCommandHelper(config *apply.ApplyCommandConfig) (*ApplyResult, error) {
	var b bytes.Buffer
	out := bufio.NewWriter(&b)
	originalStdOut := os.Stdout
	defer func() { os.Stdout = originalStdOut }()
	rc, us, sip, results, err := invokeApply(config, out)
	if flushErr := out.Flush(); flushErr != nil {
		return nil, err
	}

	return &ApplyResult{
		ResultCounts:               rc,
		Unstructured:               us,
		SkippedInvalidPolicies:     sip,
		EngineResponses:            results,
		PolicyResourceMappingCount: extractPolicyResourceMappingCount(b.Bytes()),
	}, err
}

//go:linkname invokeApply github.com/kyverno/kyverno/cmd/cli/kubectl-kyverno/commands/apply.(*ApplyCommandConfig).applyCommandHelper
func invokeApply(*apply.ApplyCommandConfig, io.Writer) (
	*processor.ResultCounts,
	[]*unstructured.Unstructured,
	apply.SkippedInvalidPolicies,
	[]engineapi.EngineResponse,
	error,
)

func extractPolicyResourceMappingCount(content []byte) string {
	// Find the first occurrence of the pattern in the big string
	policyResourceMappingCount := re.FindString(string(content))
	// Check if the substring was found
	if policyResourceMappingCount == "" {
		glog.V(2).Info("result mapping not found")
	}
	return policyResourceMappingCount
}
