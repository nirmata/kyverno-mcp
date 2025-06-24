package tools

// docsMetadata is a collection of DocumentMetadata instances,
// each describing metadata like URL, group, kind, and related keywords.
var docsMetadata = []DocumentMetadata{
	{
		URL:   "https://kyverno.io/docs/installation/",
		Group: "kyverno.io",
		Kind:  "Installation",
		Keywords: []string{
			// Core
			"kyverno", "installation", "install", "setup", "deploy", "deployment", "quickstart",

			// Helm-specific
			"helm", "helm-chart", "helm-repo", "helm-install", "helm-upgrade", "values.yaml",

			// Kubectl / manifests
			"kubectl", "kubectl-apply", "manifest", "yaml",

			// Kustomize / OLM
			"kustomize", "olm", "operator",

			// Platforms & distros
			"eks", "aks", "gke", "openshift", "rancher", "k3s", "minikube",

			// Kyverno internals
			"admission-controller", "policy-engine", "crd",
		},
	},
}
