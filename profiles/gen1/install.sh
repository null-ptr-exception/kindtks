REQUIRES="kind helm kubectl"

CLUSTER_NAME="gen1"
KIND_IMAGE="kindest/node:v1.24.15"

helm_install() {
  local name="$1" chart="$2" namespace="$3"
  shift 3
  local values_file="$KINDTKS_PROFILE_DIR/values-${name}.yaml"
  local args=()
  if [ -f "$values_file" ]; then
    local tmp
    tmp=$(mktemp)
    envsubst < "$values_file" > "$tmp"
    args+=(--values "$tmp")
  fi
  helm install "$name" "$chart" \
    --namespace "$namespace" --create-namespace \
    "${args[@]}" "$@"
  if [ -n "${tmp:-}" ]; then rm -f "$tmp"; fi
}

create() {
  echo "==> Creating Kind cluster '$CLUSTER_NAME' (k8s 1.24)..."
  kind create cluster \
    --name "$CLUSTER_NAME" \
    --image "$KIND_IMAGE" \
    --config "$KINDTKS_PROFILE_DIR/kind-config.yaml"

  install_cilium

  echo "==> Waiting for nodes to be ready..."
  kubectl wait --for=condition=Ready nodes --all --timeout=600s
  install_istio
  install_vault
  install_vault_secrets_operator

  echo ""
  echo "==> gen1 cluster is ready!"
  echo "    kubectl config use-context kind-$CLUSTER_NAME"
}

delete() {
  echo "==> Deleting Kind cluster '$CLUSTER_NAME'..."
  kind delete cluster --name "$CLUSTER_NAME"
  echo "==> Cluster '$CLUSTER_NAME' deleted."
}

install_cilium() {
  echo "==> Installing Cilium 1.12..."
  helm_install cilium "$KINDTKS_CHARTS_DIR/cilium-1.12.19.tgz" kube-system

  echo "==> Waiting for Cilium to be ready..."
  kubectl -n kube-system rollout status deployment/cilium-operator --timeout=600s
  kubectl -n kube-system rollout status daemonset/cilium --timeout=600s
}

install_istio() {
  echo "==> Installing Istio 1.16..."
  helm_install istio-base "$KINDTKS_CHARTS_DIR/base-1.16.7.tgz" istio-system
  helm_install istiod "$KINDTKS_CHARTS_DIR/istiod-1.16.7.tgz" istio-system \
    --wait --timeout 600s

  echo "==> Installing Istio Ingress Gateway..."
  helm_install istio-ingress "$KINDTKS_CHARTS_DIR/gateway-1.16.7.tgz" istio-ingress

  echo "==> Waiting for Istio Ingress Gateway to be ready..."
  kubectl -n istio-ingress rollout status deployment/istio-ingress --timeout=600s
}

install_vault() {
  echo "==> Installing Vault (dev mode)..."
  helm_install vault "$KINDTKS_CHARTS_DIR/vault-0.34.0.tgz" vault \
    --wait --timeout 600s
}

install_vault_secrets_operator() {
  echo "==> Installing Vault Secrets Operator..."
  helm_install vault-secrets-operator "$KINDTKS_CHARTS_DIR/vault-secrets-operator-1.5.0.tgz" vault-secrets-operator \
    --wait --timeout 600s
}
