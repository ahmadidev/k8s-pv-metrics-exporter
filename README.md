# k8s-pv-metrics-exporter
A Prometheus exporter for Kubernetes Persistent Volumes. It currently supports Rancher's [local-path-provisioner](https://github.com/rancher/local-path-provisioner) but will support ceph PVs too.

## Installation
```console
helm repo add pv-exporter https://ahmadidev.github.io/k8s-pv-metrics-exporter/helm-chart/repository/
helm upgrade --install --namespace pv-exporter --create-namespace pv-exporter pv-exporter/k8s-pv-metrics-exporter
```