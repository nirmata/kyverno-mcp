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

A standard Kyverno installation consists of a number of different components, some of which are optional.

* **Deployments**
  * Admission controller (required): The main component of Kyverno which handles webhook callbacks from the API server for verification, mutation, [Policy Exceptions](/docs/exceptions/), and the processing engine.
  * Background controller (optional): The component responsible for processing of generate and mutate-existing rules.
  * Reports controller (optional): The component responsible for handling of [Policy Reports](/docs/policy-reports/).
  * Cleanup controller (optional): The component responsible for processing of [Cleanup Policies](/docs/policy-types/cleanup-policy/).
* **Services**
  * Services needed to receive webhook requests.
  * Services needed for monitoring of metrics.
* **ServiceAccounts**
  * One ServiceAccount per controller to segregate and confine the permissions needed for each controller to operate on the resources for which it is responsible.
* **ConfigMaps**
  * ConfigMap for holding the main Kyverno configuration.
  * ConfigMap for holding the metrics configuration.
* **Secrets**
  * Secrets for webhook registration and authentication with the API server.
* **Roles and Bindings**
  * Roles and ClusterRoles, Bindings and ClusterRoleBindings authorizing the various ServiceAccounts to act on the resources in their scope.
* **Webhooks**
  * ValidatingWebhookConfigurations for receiving both policy and resource validation requests.
  * MutatingWebhookConfigurations for receiving both policy and resource mutating requests.
* **CustomResourceDefinitions**
  * CRDs which define the custom resources corresponding to policies, reports, and their intermediary resources.

## Compatibility Matrix

Kyverno follows the same support policy as the Kubernetes project (N-2 policy) in which the current release and the previous two minor versions are maintained. Although prior versions may work, they are not tested and therefore no guarantees are made as to their full compatibility. The below table shows the compatibility matrix.

| Kyverno Version                | Kubernetes Min | Kubernetes Max |
|--------------------------------|----------------|----------------|
| 1.12.x                         | 1.26           | 1.29           |
| 1.13.x                         | 1.28           | 1.31           |
| 1.14.x                         | 1.29           | 1.32           |

**NOTE:** For long term compatibility Support select a [commercially supported Kyverno distribution](https://kyverno.io/support/nirmata).

## Security vs Operability

For a production installation, Kyverno should be installed in [high availability mode](/docs/installation/methods.md#high-availability-installation). Regardless of the installation method used for Kyverno, it is important to understand the risks associated with any webhook and how it may impact cluster operations and security especially in production environments. Kyverno configures its resource webhooks by default (but [configurable](/docs/policy-types/cluster-policy/policy-settings.md)) in [fail closed mode](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#failure-policy). This means if the API server cannot reach Kyverno in its attempt to send an AdmissionReview request for a resource that matches a policy, the request will fail. For example, a validation policy exists which checks that all Pods must run as non-root. A new Pod creation request is submitted to the API server and the API server cannot reach Kyverno. Because the policy cannot be evaluated, the request to create the Pod will fail. Care must therefore be taken to ensure that Kyverno is always available or else configured appropriately to exclude certain key Namespaces, specifically that of Kyverno's, to ensure it can receive those API requests. There is a tradeoff between security by default and operability regardless of which option is chosen.

The following combination may result in cluster inoperability if the Kyverno Namespace is not excluded:

1. At least one Kyverno rule matching on `Pods` is configured in fail closed mode (the default setting).
2. No Namespace exclusions have been configured for at least the Kyverno Namespace, possibly other key system Namespaces (ex., `kube-system`). This is not the default as of Helm chart version 2.5.0.
3. All Kyverno Pods become unavailable due to a full cluster outage or improper scaling in of Nodes (for example, a cloud PaaS destroying too many Nodes in a node group as part of an auto-scaling operation without first cordoning and draining Pods).

If this combination of events occurs, the only way to recover is to manually delete the ValidatingWebhookConfigurations thereby allowing new Kyverno Pods to start up. Recovery steps are provided in the [troubleshooting section](../troubleshooting/_index.md#api-server-is-blocked).

{{% alert title="Note" color="info" %}}
Kubernetes will not send ValidatingWebhookConfiguration or MutatingWebhookConfiguration objects to admission controllers, so therefore it is not possible to use a Kyverno policy to validate or mutate these objects.
{{% /alert %}}

By contrast, these operability concerns can be mitigated by making some security concessions. Specifically, by excluding the Kyverno and other system Namespaces during installation, should the aforementioned failure scenarios occur Kyverno should be able to recover by itself with no manual intervention. This is the default behavior as of the Helm chart version 2.5.0. However, configuring these exclusions means that subsequent policies will not be able to act on resources destined for those Namespaces as the API server has been told not to send AdmissionReview requests for them. Providing controls for those Namespaces, therefore, lies in the hands of the cluster administrator to implement, for example, Kubernetes RBAC to restrict who and what can take place in those excluded Namespaces.

{{% alert title="Note" color="info" %}}
Namespaces and/or objects within Namespaces may be excluded in a variety of ways including namespaceSelectors and objectSelectors. The Helm chart provides options for both, but by default the Kyverno Namespace will be excluded.
{{% /alert %}}

{{% alert title="Note" color="warning" %}}
When using objectSelector, it may be possible for users to spoof the same label key/value used to configure the webhooks should they discover how it is configured, thereby allowing resources to circumvent policy detection. For this reason, a namespaceSelector using the `kubernetes.io/metadata.name` immutable label is recommended.
{{% /alert %}}

The choices and their implications are therefore:

1. Do not exclude system Namespaces, including Kyverno's, (not default) during installation resulting in a more secure-by-default posture but potentially requiring manual recovery steps in some outage scenarios.
2. Exclude system Namespaces during installation (default) resulting in easier cluster recovery but potentially requiring other methods to secure those Namespaces, for example with Kubernetes RBAC.

You should choose the best option based upon your risk aversion, needs, and operational practices.

{{% alert title="Note" color="info" %}}
If you choose to *not* exclude Kyverno or system Namespaces/objects and intend to cover them with policies, you may need to modify the Kyverno [resourceFilters](/docs/installation/customization.md#resource-filters) entry in the [ConfigMap](/docs/installation/customization.md#configmap-keys) to remove those items.
{{% /alert %}}

## Installing Kyverno Policy Sets via Helm

After installing Kyverno, you can deploy predefined policy sets that implement security and best practice standards. Kyverno provides several curated policy sets available through Helm charts.

### Prerequisites

Before installing policy sets, ensure you have:

1. **Kyverno installed**: The core Kyverno components must be running in your cluster
2. **Helm 3.x**: Required for installing the policy charts
3. **kubectl access**: Cluster admin permissions to deploy policies

### Adding the Kyverno Helm Repository

First, add the official Kyverno Helm repository:

```bash
helm repo add kyverno https://kyverno.github.io/kyverno/
helm repo update
```

### 1. Pod Security Standards

The Pod Security Standards policy set implements the Kubernetes Pod Security Standards using Kyverno policies. This includes both **Baseline** and **Restricted** security profiles.

**Features:**
- Baseline security controls (prevents most privileged escalations)
- Restricted security controls (heavily restricted, following pod hardening best practices)
- Host namespace restrictions
- Capability dropping requirements
- Security context validation

**Installation:**

```bash
# Install the complete Pod Security Standards policy set
helm install pod-security-policies kyverno/kyverno-policies -n kyverno
```

**Custom Installation:**
```bash
# Install with custom values for specific security level
helm install pod-security-policies kyverno/kyverno-policies \
  -n kyverno \
  --set podSecurityStandard=restricted \
  --set validationFailureAction=enforce
```

**Verification:**
```bash
# Check deployed policies
kubectl get clusterpolicy | grep pod-security

# View policy details
kubectl describe clusterpolicy podsecurity-subrule-baseline
kubectl describe clusterpolicy podsecurity-subrule-restricted
```

### 2. RBAC Best Practices

The RBAC Best Practices policy set enforces security controls around Role-Based Access Control (RBAC) resources.

**Features:**
- Prevents overly permissive RBAC bindings
- Validates service account security configurations
- Enforces least privilege access principles
- Restricts dangerous RBAC combinations

**Installation:**

```bash
# Install RBAC best practices policies
helm install rbac-policies kyverno/kyverno-policies \
  -n kyverno \
  --set policies.require-rbac-best-practices.enabled=true \
  --set policies.disallow-rbac-on-default-serviceaccounts.enabled=true
```

**Alternative installation using Nirmata curated policies:**
```bash
# Add Nirmata policy repository (contains additional RBAC policies)
helm repo add nirmata https://nirmata.github.io/kyverno-policies/

# Install RBAC best practices
helm install rbac-best-practices nirmata/rbac-best-practices -n kyverno
```

### 3. Kubernetes Best Practices

The Kubernetes Best Practices policy set includes general operational and security best practices for Kubernetes workloads.

**Features:**
- Resource limits and requests enforcement
- Required labels and annotations
- Image security (prevent latest tags, require specific registries)
- Service security (restrict NodePort, LoadBalancer services)
- Network policies and pod networking controls
- Configuration best practices

**Installation:**

```bash
# Install Kubernetes best practices policies
helm install k8s-best-practices kyverno/kyverno-policies \
  -n kyverno \
  --set policies.require-labels.enabled=true \
  --set policies.require-cpu-limit.enabled=true \
  --set policies.require-memory-limit.enabled=true \
  --set policies.disallow-latest-tag.enabled=true \
  --set policies.disallow-nodeport-services.enabled=true
```

**Installation with custom configuration:**
```bash
# Create custom values file
cat > k8s-best-practices-values.yaml << EOF
policies:
  require-labels:
    enabled: true
    parameters:
      require:
        - "app.kubernetes.io/name"
        - "app.kubernetes.io/instance"
        - "app.kubernetes.io/version"
  
  require-cpu-limit:
    enabled: true
    parameters:
      require: ["<4"]
  
  require-memory-limit:
    enabled: true
    parameters:
      require: ["<8Gi"]
  
  disallow-image-tags:
    enabled: true
    parameters:
      disallow: ["latest", "main", "master"]

validationFailureAction: "Audit"  # Change to "Enforce" for production
EOF

# Install with custom values
helm install k8s-best-practices kyverno/kyverno-policies \
  -n kyverno \
  -f k8s-best-practices-values.yaml
```

### Installing All Three Policy Sets

To install all three policy sets together:

```bash
# Install all policy sets with recommended configuration
helm install kyverno-all-policies kyverno/kyverno-policies \
  -n kyverno \
  --set policies.require-labels.enabled=true \
  --set policies.require-cpu-limit.enabled=true \
  --set policies.require-memory-limit.enabled=true \
  --set policies.disallow-latest-tag.enabled=true \
  --set policies.disallow-privileged-containers.enabled=true \
  --set policies.disallow-host-namespaces.enabled=true \
  --set policies.require-drop-all-capabilities.enabled=true \
  --set policies.disallow-rbac-on-default-serviceaccounts.enabled=true \
  --set validationFailureAction=Audit
```

### Post-Installation Verification

After installing the policy sets, verify they are working correctly:

```bash
# List all cluster policies
kubectl get clusterpolicy

# Check policy status
kubectl get clusterpolicy -o wide

# View specific policy details
kubectl describe clusterpolicy <policy-name>

# Check for policy violations (if any exist)
kubectl get policyreport -A
kubectl get clusterpolicyreport
```

### Policy Set Management

**Upgrading Policy Sets:**
```bash
# Update the Helm repository
helm repo update

# Upgrade the policy sets
helm upgrade kyverno-all-policies kyverno/kyverno-policies -n kyverno
```

**Uninstalling Policy Sets:**
```bash
# Remove specific policy set
helm uninstall pod-security-policies -n kyverno

# Remove all policies
helm uninstall kyverno-all-policies -n kyverno
```

### Important Considerations

1. **Start with Audit Mode**: When first deploying policies, use `validationFailureAction: Audit` to monitor violations without blocking workloads.

2. **Test in Non-Production**: Always test policy sets in development environments before applying to production clusters.

3. **Namespace Exclusions**: Consider excluding system namespaces from certain policies to prevent cluster operational issues.

4. **Gradual Rollout**: Implement policies incrementally, starting with less restrictive policies and gradually adding more stringent controls.

5. **Monitor Policy Reports**: Regularly review PolicyReports to understand compliance status and policy violations.

For more detailed policy configuration options, visit the [Kyverno Policies documentation](https://kyverno.io/policies/) and [Helm chart repository](https://github.com/kyverno/kyverno).