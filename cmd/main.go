// Package main implements a Model Context Protocol (MCP) server for Kyverno.
// It provides tools for managing and interacting with Kyverno policies and resources.
package main

import (
	"flag"
	"fmt"
	"github.com/nirmata/kyverno-mcp/pkg/tools"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/mark3labs/mcp-go/server"
)

// kubeconfigPath holds the path to the kubeconfig file supplied via the --kubeconfig flag.
var kubeconfigPath string

// httpAddr specifies the address the Streamable HTTP server will bind to.
var httpAddr string

// tlsCert specifies the path to the TLS certificate file.
var tlsCert string

// tlsKey specifies the path to the TLS key file.
var tlsKey string

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
	defer klog.Flush()
	if err := flag.Set("v", "2"); err != nil {
		klog.ErrorS(err, "failed to set klog verbosity")
	}
	// Define CLI flags (guard against duplicate registration from imported packages)
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfigPath, "kubeconfig", "", "Path to the kubeconfig file to use. If not provided, defaults are used.")
	}
	if flag.Lookup("http-addr") == nil {
		flag.StringVar(&httpAddr, "http-addr", "", "Address to bind the Streamable HTTP server (ignored if --http is false)")
	}
	if flag.Lookup("tls-cert") == nil {
		flag.StringVar(&tlsCert, "tls-cert", "", "Path to the TLS certificate file to use. If not provided, defaults are used.")
	}
	if flag.Lookup("tls-key") == nil {
		flag.StringVar(&tlsKey, "tls-key", "", "Path to the TLS key file to use. If not provided, defaults are used.")
	}

	// Parse CLI flags early so subsequent init can rely on them. Capture ErrHelp
	if err := flag.CommandLine.Parse(os.Args[1:]); err == flag.ErrHelp {
		// flag package has already printed the usage message via flag.Usage
		return
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

	// Prefer HTTPS when TLS credentials are supplied. If not, fall back to plain HTTP.
	if tlsCert != "" && tlsKey != "" {
		// Create the streamable HTTP handler backed by our MCP server
		streamSrv := server.NewStreamableHTTPServer(s)

		// Default to a secure non-privileged port if no address is specified
		addr := httpAddr
		if addr == "" {
			addr = ":8443"
		}

		// net/http server configuration (HTTPS)
		httpServer := &http.Server{
			Addr:    addr,
			Handler: streamSrv,
		}

		klog.InfoS("Starting Streamable HTTPS server", "addr", addr, "tlsCert", tlsCert, "tlsKey", tlsKey)

		// Run the server in a goroutine so that the main thread can continue to serve stdio
		go func() {
			if err := httpServer.ListenAndServeTLS(tlsCert, tlsKey); err != nil && err != http.ErrServerClosed {
				klog.ErrorS(err, "Streamable HTTPS server terminated with error")
			}
		}()

		// ------------------------------------------------------------------
		// Block main goroutine until an OS termination signal is received.
		// ------------------------------------------------------------------
		stopCh := make(chan os.Signal, 1)
		signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

		klog.Info("Server started. Waiting for termination signal...")
		<-stopCh

		klog.Info("Termination signal received. Exiting.")
	} else if httpAddr != "" {
		// Create the streamable HTTP handler backed by our MCP server
		streamSrv := server.NewStreamableHTTPServer(s)

		// net/http server configuration (HTTP)
		httpServer := &http.Server{
			Addr:    httpAddr,
			Handler: streamSrv,
		}

		klog.InfoS("Starting Streamable HTTP server", "addr", httpAddr)

		// Run the server in a goroutine so that the main thread can continue to serve stdio
		go func() {
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				klog.ErrorS(err, "Streamable HTTP server terminated with error")
			}
		}()

		// ------------------------------------------------------------------
		// Block main goroutine until an OS termination signal is received.
		// ------------------------------------------------------------------
		stopCh := make(chan os.Signal, 1)
		signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

		klog.Info("Server started. Waiting for termination signal...")
		<-stopCh

		klog.Info("Termination signal received. Exiting.")
	} else {
		// Start the MCP server on stdio in a separate goroutine to allow
		// the main goroutine to listen for OS termination signals and
		// perform graceful shutdown (SIGINT/SIGTERM).
		klog.Info("Starting MCP server on stdio...")

		go func() {
			if err := server.ServeStdio(s); err != nil {
				klog.ErrorS(err, "error in MCP stdio server")
			}
			klog.Info("MCP stdio server terminated.")
		}()

		// ------------------------------------------------------------------
		// Block main goroutine until an OS termination signal is received.
		// ------------------------------------------------------------------
		stopCh := make(chan os.Signal, 1)
		signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

		klog.Info("Server started. Waiting for termination signal...")
		<-stopCh

		klog.Info("Termination signal received. Exiting.")
	}
}
