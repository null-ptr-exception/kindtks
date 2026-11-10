#!/usr/bin/env bash
set -euo pipefail

CHARTS_DIR="${1:-charts}"
mkdir -p "$CHARTS_DIR"

helm repo add cilium https://helm.cilium.io/ --force-update
helm repo add istio https://istio-release.storage.googleapis.com/charts --force-update
helm repo add hashicorp https://helm.releases.hashicorp.com --force-update
helm repo add ricoberger https://ricoberger.github.io/helm-charts/ --force-update
helm repo update

helm pull cilium/cilium --version 1.13.10 --destination "$CHARTS_DIR"
helm pull istio/base --version 1.16.7 --destination "$CHARTS_DIR"
helm pull istio/istiod --version 1.16.7 --destination "$CHARTS_DIR"
helm pull istio/gateway --version 1.16.7 --destination "$CHARTS_DIR"
helm pull hashicorp/vault --version 0.34.0 --destination "$CHARTS_DIR"
helm pull ricoberger/vault-secrets-operator --version 2.7.0 --destination "$CHARTS_DIR"

# kube-network-policies publishes its chart only in git: package it from the
# release source tarball. Its Chart.yaml version is a fixed 0.0.1, so set the
# release version.
KNP_VERSION=1.1.2
KNP_SHA256=f1fc7e87cce0e0d52a4410b3c5fa655267e20ecef28cb055ae8fccbd992974ba
knp_work=$(mktemp -d)
wget -qO "$knp_work/src.tar.gz" "https://github.com/kubernetes-sigs/kube-network-policies/archive/refs/tags/v${KNP_VERSION}.tar.gz"
echo "$KNP_SHA256  $knp_work/src.tar.gz" | sha256sum -c -
tar xzf "$knp_work/src.tar.gz" -C "$knp_work" "kube-network-policies-${KNP_VERSION}/charts/kube-network-policies"
helm package "$knp_work/kube-network-policies-${KNP_VERSION}/charts/kube-network-policies" \
  --version "$KNP_VERSION" --app-version "v${KNP_VERSION}" --destination "$CHARTS_DIR"
rm -rf "$knp_work"

echo "Charts downloaded to $CHARTS_DIR:"
ls -la "$CHARTS_DIR"/*.tgz
