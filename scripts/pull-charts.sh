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

echo "Charts downloaded to $CHARTS_DIR:"
ls -la "$CHARTS_DIR"/*.tgz
