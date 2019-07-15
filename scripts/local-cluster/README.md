### K3s Cluster

This directory contains scripts and manifests to deploy a local, lightweight cluster based on [k3s](https://github.com/rancher/k3s).

#### Requirements

1. Docker
1. kubectl
1. helm

#### Running

The `make` targets in the project root with the prefix `local.cluster.*` can be used to initiate various stages.

Provided functionality includes:

1. Bring up a local k3s cluster suitable for deploying Istio to - `make local.cluster.up`.
1. Install Istio into the local cluster (expects cluster to be running) - `make local.install-istio`.
1. Install 3scale-istio-adapter into local cluster (expects cluster to be running) - `make local.install-adapter`.
1. Install [httpbin](https://httpbin.org) in local cluster (expects cluster to be running) - `make local.cluster.install-httpbin`.
1. A wrapper target to carry out all of the above steps - `make local.cluster.install-environment`.
1. A clean target, to delete the cluster and all associated applications - `make local.cluster.clean`.

A specific version of Istio can be installed by specifying the `ISTIO_VERSION=x.y.z` variable when running `make.local.*`

##### Working with the cluster

To interact with the cluster you will need to set `kubectl` to use the generated (after cluster creation) `kubeconfig.yaml` located in `3scale-istio-adapter/scripts/local-cluster`.
This can be used in two ways:

 1. export KUBECONFIG=`<some-path>/3scale-istio-adapter/scripts/local-cluster/kubeconfig.yaml` - This is then active for the entire session.
 1. Pass the --kubeconfig=`<some-path>/3scale-istio-adapter/scripts/local-cluster/kubeconfig.yaml` flag on each call to `kubectl`.


#### Cluster Ingress

Assuming the `local.cluster.environment` make target has been used to bring up the cluster, Istio will be used as the ingress gateway.
For convenience, the ports have been mapped to the host, so services can be reached via `8080`, and `8443` on localhost.

Test the clusters functionality by running the following:

`curl "http://127.0.0.1:8080/httpbin/get" -H "accept: application/json"`


#### Troubleshooting

##### DNS

So you are having DNS issues?

If your Pods cannot resolve external hostnames, check the logs from the `coredns` Pod in `kube-system` namespace.
It is likely you will see i/o timeouts for services running on port 53 pointing some DNS issue in your environment.

The following command will patch the appropriate ConfigMap. Replace `8.8.8.8` with your own nameservers' ip address:

```yaml
kubectl get cm coredns -n kube-system -o yaml | sed -e 's/proxy . \/etc\/resolv.conf/proxy . 8.8.8.8/g' | kubectl apply -f -
```

Restart the Pod:
```yaml
kubectl delete po -n kube-system -l k8s-app=kube-dns
```
