---
title: Installation
linkTitle: Installation
weight: 20
description: Understand how to install and configure Kyverno.
---

Kyverno provides multiple methods for installation: Helm and YAML manifest. When installing in a production environment, Helm is the recommended and most flexible method as it offers convenient configuration options to satisfy a wide range of customizations. Regardless of the method, Kyverno must always be installed in a dedicated Namespace; it must not be co-located with other applications in existing Namespaces including system Namespaces such as `kube-system`. The Kyverno Namespace should also not be used for deployment of other, unrelated applications and services.

The diagram below shows a typical Kyverno installation featuring all available controllers.

<img src="/images/kyverno-installation.png" alt="Kyverno Installation" width="80%"/>
<br/><br/>

## Installing Kyverno via Helm

```bash
helm repo add kyverno https://kyverno.github.io/kyverno/
helm repo update
helm install kyverno kyverno/kyverno -n kyverno --create-namespace
```

## Installing Kyverno Policy Sets via Helm

After installing Kyverno, you can deploy predefined policy sets that implement security and best practice standards. Kyverno provides several curated policy sets available through Helm charts.

### Prerequisites

Before installing policy sets, ensure you have:

1. **Kyverno installed**: The core Kyverno components must be running in your cluster
2. **Helm 3.x**: Required for installing the policy charts
3. **kubectl access**: Cluster admin permissions to deploy policies

### 1. Pod Security Standards

**Installation:**

```bash
# Install the complete Pod Security Standards policy set
helm install pod-security-policies kyverno/kyverno-policies -n kyverno
```

**Verification:**
```bash
# Check deployed policies
kubectl get clusterpolicy | grep pod-security

# View policy details
kubectl describe clusterpolicy podsecurity-subrule-baseline
kubectl describe clusterpolicy podsecurity-subrule-restricted
```

#### Uninstalling Policy Sets:
```bash
# Remove specific policy set
helm uninstall pod-security-policies -n kyverno
```
