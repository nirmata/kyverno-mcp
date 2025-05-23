package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/kyverno/go-kyverno-mcp/pkg/kyverno"
	"github.com/kyverno/go-kyverno-mcp/pkg/mcp"
	"github.com/kyverno/go-kyverno-mcp/pkg/mcp/types"
)

func main() {
	// Parse command line flags
	kubeconfig := flag.String("kubeconfig", "", "path to the kubeconfig file")
	port := flag.String("port", "8080", "port to listen on")
	flag.Parse()

	// Initialize Kyverno engine components
	fmt.Println("Initializing Kyverno engine components...")
	policyLoader := kyverno.NewLocalPolicyLoader()
	resourceLoader := kyverno.NewAPIResourceLoader()
	engine := kyverno.NewKyvernoEngine(policyLoader, resourceLoader, *kubeconfig)

	// Create and register the ApplyService
	applyService := mcp.NewApplyService(engine)

	// Set up HTTP server
	r := mux.NewRouter()
	r.HandleFunc("/apply", func(w http.ResponseWriter, r *http.Request) {
		// Log the incoming request
		body, _ := io.ReadAll(r.Body)
		log.Printf("Received request: %s", string(body))

		// Restore the request body for JSON decoding
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		var req types.ApplyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("Error decoding request: %v", err)
			http.Error(w, fmt.Sprintf("Error decoding request: %v", err), http.StatusBadRequest)
			return
		}

		log.Printf("Decoded request: %+v", req)

		// Check if policy paths are empty
		if len(req.PolicyPaths) == 0 {
			log.Println("No policy paths provided in request")
		} else {
			log.Printf("Policy paths: %v", req.PolicyPaths)
		}

		resp, err := applyService.ProcessApplyRequest(r.Context(), &req)
		if err != nil {
			log.Printf("Error processing request: %v", err)
			http.Error(w, fmt.Sprintf("Error processing request: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("Error encoding response: %v", err)
			http.Error(w, fmt.Sprintf("Error encoding response: %v", err), http.StatusInternalServerError)
			return
		}
	}).Methods("POST")

	// Add a simple health check endpoint
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// Start HTTP server
	server := &http.Server{
		Addr:    ":" + *port,
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		fmt.Printf("Starting HTTP server on :%s\n", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("\nShutting down server...")
	if err := server.Shutdown(context.Background()); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}
	fmt.Println("Server stopped")
}
