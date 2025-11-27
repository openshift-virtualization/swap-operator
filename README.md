# Swap Operator
Operator that manages swap on the OpenShift nodes
# Quick Start

You need to have a working OpenShift Cluster
## Bulld & Install
```bash
$ export KUBECONFIG="/path/to/ocp/kubeconfig"
$ git clone https://github.com/openshift-virtualization/swap-operator.git
$ cd swap-operator
$ podman login quay.io
$ export IMG=quay.io/openshift-virtualization/swap-operator:v0.1
$ make docker-build
$ make docker-push
$ oc create -k config/default
```
## Cleanup
```bash
$ oc delete -k config/default
```
