package utils

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// inClusterConfig is a variable that holds the function to get in-cluster config
// This allows us to mock it in tests
var inClusterConfig = rest.InClusterConfig

// commonGVKtoGVR is a hardcoded map of common GroupVersionKind to GroupVersionResource mappings
// This is a simplified version for the MVP. In a production environment, this should be replaced
// with a proper RESTMapper implementation that can discover the mappings from the API server.
var commonGVKtoGVR = map[string]schema.GroupVersionResource{
	// Core API Group
	"v1/Pod":                   {Group: "", Version: "v1", Resource: "pods"},
	"v1/Service":               {Group: "", Version: "v1", Resource: "services"},
	"v1/Namespace":             {Group: "", Version: "v1", Resource: "namespaces"},
	"v1/Node":                  {Group: "", Version: "v1", Resource: "nodes"},
	"v1/ConfigMap":             {Group: "", Version: "v1", Resource: "configmaps"},
	"v1/Secret":                {Group: "", Version: "v1", Resource: "secrets"},
	"v1/ServiceAccount":        {Group: "", Version: "v1", Resource: "serviceaccounts"},
	"v1/PersistentVolume":      {Group: "", Version: "v1", Resource: "persistentvolumes"},
	"v1/PersistentVolumeClaim": {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},

	// Apps API Group
	"apps/v1/Deployment":  {Group: "apps", Version: "v1", Resource: "deployments"},
	"apps/v1/StatefulSet": {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"apps/v1/DaemonSet":   {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"apps/v1/ReplicaSet":  {Group: "apps", Version: "v1", Resource: "replicasets"},

	// Batch API Group
	"batch/v1/Job":     {Group: "batch", Version: "v1", Resource: "jobs"},
	"batch/v1/CronJob": {Group: "batch", Version: "v1", Resource: "cronjobs"},

	// Networking API Group
	"networking.k8s.io/v1/NetworkPolicy": {Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
	"networking.k8s.io/v1/Ingress":       {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},

	// RBAC API Group
	"rbac.authorization.k8s.io/v1/Role":               {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
	"rbac.authorization.k8s.io/v1/RoleBinding":        {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
	"rbac.authorization.k8s.io/v1/ClusterRole":        {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
	"rbac.authorization.k8s.io/v1/ClusterRoleBinding": {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},

	// Storage API Group
	"storage.k8s.io/v1/StorageClass": {Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"},
}

// GVKToGVR converts a GroupVersionKind to a GroupVersionResource.
// This is a simplified implementation that uses a hardcoded map of common resources.
// In a production environment, this should be replaced with a proper RESTMapper
// implementation that can discover the mappings from the API server.
//
// The apiVersion parameter should be in the format "group/version" (e.g., "apps/v1")
// or just "version" for core resources (e.g., "v1").
// The kind parameter is the Kubernetes resource kind (e.g., "Deployment", "Pod").
//
// Returns the GroupVersionResource and nil on success, or an error if the mapping is not found.
func GVKToGVR(apiVersion, kind string) (schema.GroupVersionResource, error) {
	// Normalize the apiVersion to handle empty group case
	if apiVersion == "" {
		apiVersion = "v1"
	}

	// Create the lookup key
	key := fmt.Sprintf("%s/%s", apiVersion, kind)

	// Try exact match first
	if gvr, exists := commonGVKtoGVR[key]; exists {
		return gvr, nil
	}

	// If not found, try to handle core API group (empty group)
	if !strings.Contains(apiVersion, "/") {
		// This is a core API group (e.g., "v1")
		key = fmt.Sprintf("%s/%s", apiVersion, kind)
		if gvr, exists := commonGVKtoGVR[key]; exists {
			return gvr, nil
		}
	}

	// Not found in our hardcoded map
	return schema.GroupVersionResource{}, fmt.Errorf("no GroupVersionResource found for apiVersion=%s, kind=%s", apiVersion, kind)
}

// GetDynamicClient creates a dynamic Kubernetes client using the provided kubeconfig path.
// If kubeconfigPath is empty, it attempts to create an in-cluster configuration.
// Returns a dynamic.Interface that can be used to interact with Kubernetes resources.
func GetDynamicClient(kubeconfigPath string) (dynamic.Interface, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath == "" {
		// In-cluster configuration
		config, err = inClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("error creating in-cluster config: %v", err)
		}
	} else {
		// Out-of-cluster configuration using provided kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("error building kubeconfig: %v", err)
		}
	}

	// Create the dynamic client
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating dynamic client: %v", err)
	}

	return client, nil
}
