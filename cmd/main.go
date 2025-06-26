// Package main implements a Model Context Protocol (MCP) server for Kyverno.
// It provides tools for managing and interacting with Kyverno policies and resources.
package main

import (
	"flag"
	"fmt"
	"kyverno-mcp/pkg/tools"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/mark3labs/mcp-go/server"
)

// kubeconfigPath holds the path to the kubeconfig file supplied via the --kubeconfig flag.
var kubeconfigPath string

func init() {
	flag.Usage = func() {
		// Header
		if _, err := fmt.Fprintf(flag.CommandLine.Output(), "\nKyverno MCP Server – a Model-Context-Protocol server for Kyverno\n"); err != nil {
			klog.ErrorS(err, "failed to write header")
		}
		if _, err := fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags]\n\n", os.Args[0]); err != nil {
			klog.ErrorS(err, "failed to write usage")
		}

		// Flags
		if _, err := fmt.Fprintln(flag.CommandLine.Output(), "Flags:"); err != nil {
			klog.ErrorS(err, "failed to write flags header")
		}
		flag.PrintDefaults()

		// Tooling section – keep this in sync with tools registered in pkg/tools.
		if _, err := fmt.Fprintln(flag.CommandLine.Output(), "\nAvailable tools exposed over MCP:"); err != nil {
			klog.ErrorS(err, "failed to write tools header")
		}
		msgs := []string{
			"  list_contexts   – List all available Kubernetes contexts",
			"  switch_context  – Switch to a different Kubernetes context (requires --context)",
			"  apply_policies  – Apply policies to a cluster",
			"  help            – Get Kyverno documentation for installation and troubleshooting",
			"  show_violations – Show violations for a given resource",
		}
		for _, m := range msgs {
			if _, err := fmt.Fprintln(flag.CommandLine.Output(), m); err != nil {
				klog.ErrorS(err, "failed to write tool description", "tool", m)
			}
		}

		// Terminate after printing help to match standard behaviour.
	}
}

func main() {
	klog.InitFlags(nil)
	if err := flag.Set("v", "2"); err != nil {
		klog.ErrorS(err, "failed to set klog verbosity")
	}
	// Define CLI flags (guard against duplicate registration from imported packages)
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfigPath, "kubeconfig", "", "Path to the kubeconfig file to use. If not provided, defaults are used.")
	}

	// Parse CLI flags early so subsequent init can rely on them. Capture ErrHelp
	if err := flag.CommandLine.Parse(os.Args[1:]); err == flag.ErrHelp {
		// flag package has already printed the usage message via flag.Usage
		os.Exit(0)
		defer klog.Flush()
	}

	// If the kubeconfig flag was registered elsewhere, capture its value
	if kubeconfigPath == "" {
		if kubeFlag := flag.Lookup("kubeconfig"); kubeFlag != nil {
			kubeconfigPath = kubeFlag.Value.String()
		}
	}

	if kubeconfigPath != "" {
		// Ensure downstream libraries relying on KUBECONFIG honour the supplied path (e.g., Kyverno CLI helpers)
		_ = os.Setenv("KUBECONFIG", kubeconfigPath)
		klog.InfoS("Using kubeconfig file: %s", kubeconfigPath)
	}

	// Setup logging to standard output
	klog.SetOutput(os.Stderr)
	klog.Info("Logging initialized to Stdout.")
	klog.Info("------------------------------------------------------------------------")
	klog.InfoS("Kyverno MCP Server starting at %s", time.Now().Format(time.RFC3339))

	klog.Info("kyverno-mcp: ")
	klog.Info("Starting Kyverno MCP server...")

	// Create a new MCP server
	klog.InfoS("Creating new MCP server instance...")
	s := server.NewMCPServer(
		"Kyverno MCP Server",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)
	klog.Info("MCP server instance created.")

	// Register tools
	tools.ListContexts(s)
	tools.SwitchContext(s)
	tools.ApplyPolicies(s)
	tools.Help(s)
	tools.ShowViolations(s)

	// Start the MCP server
	klog.Info("Starting MCP server on stdio...")
	var err error
	if err = server.ServeStdio(s); err != nil {
		klog.ErrorS(err, "error starting server")
	}
}
