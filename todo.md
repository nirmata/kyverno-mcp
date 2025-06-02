# TODO

1. Install Kyverno
2. Scan Kubernetes Resources : takes a set of policy sets in Git repos as input and scans a cluster or a namespace. This can default to the 4 Nirmata policy sets, similar to  nctl but allow users to specify policy sets. It can also take an optional namespace param.
3. Get Kubernetes Reports: gets policy reports for an entire cluster or a namespace. If Kyverno is not installed, this can return a message to install Kyverno with Helm.
4. Kyverno documentation: exposes Kyverno docs and sample policies.
