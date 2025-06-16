// Package main implements a Model Context Protocol (MCP) server for Kyverno.
// It provides tools for managing and interacting with Kyverno policies and resources.
package main

import (
	"flag"
	"fmt"
	"kyverno-mcp/pkg/tools"
	"log"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// kubeconfigPath holds the path to the kubeconfig file supplied via the --kubeconfig flag.
// If empty, the default resolution logic from client-go is used.
var kubeconfigPath string

// awsconfigPath holds the path to the AWS config file supplied via the --awsconfig flag.
// If empty, the default resolution logic (environment variable AWS_CONFIG_FILE or ~/.aws/config) is used.
var awsconfigPath string

// awsProfile holds the AWS profile name supplied via the --awsprofile flag.
var awsProfile string

func init() {
	flag.Usage = func() {
		// Header
		fmt.Fprintf(flag.CommandLine.Output(), "\nKyverno MCP Server – a Model-Context-Protocol server for Kyverno\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags]\n\n", os.Args[0])

		// Flags
		fmt.Fprintln(flag.CommandLine.Output(), "Flags:")
		flag.PrintDefaults()

		// Tooling section – keep this in sync with tools registered in pkg/tools.
		fmt.Fprintln(flag.CommandLine.Output(), "\nAvailable tools exposed over MCP:")
		fmt.Fprintln(flag.CommandLine.Output(), "  list_contexts   – List all available Kubernetes contexts")
		fmt.Fprintln(flag.CommandLine.Output(), "  switch_context  – Switch to a different Kubernetes context (requires --context)")
		fmt.Fprintln(flag.CommandLine.Output(), "  apply_policies  – Apply policies to a cluster")

		// Terminate after printing help to match standard behaviour.
	}
}

func main() {
	// Define CLI flags (guard against duplicate registration from imported packages)
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfigPath, "kubeconfig", "", "Path to the kubeconfig file to use. If not provided, defaults are used.")
	}
	flag.StringVar(&awsconfigPath, "awsconfig", "", "Path to the AWS config file to use. If not provided, defaults to environment variable AWS_CONFIG_FILE or ~/.aws/config.")
	flag.StringVar(&awsProfile, "awsprofile", "", "AWS profile to use (defaults to current profile).")

	// Parse CLI flags early so subsequent init can rely on them. Capture ErrHelp
	if err := flag.CommandLine.Parse(os.Args[1:]); err == flag.ErrHelp {
		// flag package has already printed the usage message via flag.Usage
		os.Exit(0)
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
		log.Printf("Using kubeconfig file: %s", kubeconfigPath)
	}

	if awsconfigPath != "" {
		_ = os.Setenv("AWS_CONFIG_FILE", awsconfigPath)
		log.Printf("Using AWS config file: %s", awsconfigPath)
	}

	if awsProfile != "" {
		_ = os.Setenv("AWS_PROFILE", awsProfile)
		log.Printf("Using AWS profile: %s", awsProfile)
	}

	// Setup logging to standard output
	log.SetOutput(os.Stderr)
	log.Println("Logging initialized to Stdout.")
	log.Println("------------------------------------------------------------------------")
	log.Printf("Kyverno MCP Server starting at %s", time.Now().Format(time.RFC3339))

	log.SetPrefix("kyverno-mcp: ")
	log.Println("Starting Kyverno MCP server...")

	// Create a new MCP server
	log.Println("Creating new MCP server instance...")
	s := server.NewMCPServer(
		"Kyverno MCP Server",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)
	log.Println("MCP server instance created.")

	// Register tools
	tools.ListContexts(s)
	tools.SwitchContext(s)
	tools.ApplyPolicies(s)

	// Start the MCP server
	log.Println("Starting MCP server on stdio...")
	var err error
	if err = server.ServeStdio(s); err != nil {
		log.Fatalf("Error starting server: %v\n", err)
	}
}
