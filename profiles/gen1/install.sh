REQUIRES="kind helm kubectl"

CLUSTER_NAME="gen1"
KIND_IMAGE="kindest/node:v1.24.15"

create() {
  echo "==> Creating Kind cluster '$CLUSTER_NAME' (k8s 1.24)..."
  kind create cluster \
    --name "$CLUSTER_NAME" \
    --image "$KIND_IMAGE" \
    --config "$KINDTKS_PROFILE_DIR/kind-config.yaml"

  echo "==> Waiting for cluster to be ready..."
  kubectl wait --for=condition=Ready nodes --all --timeout=120s

  install_cilium
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
  local values=""
  values="$values --set ipam.mode=kubernetes"
  values="$values --set image.pullPolicy=IfNotPresent"
  if [ -n "${KINDTKS_REGISTRY:-}" ]; then
    values="$values --set image.repository=${KINDTKS_REGISTRY}cilium/cilium"
    values="$values --set image.useDigest=false"
    values="$values --set operator.image.repository=${KINDTKS_REGISTRY}cilium/operator"
    values="$values --set operator.image.useDigest=false"
    values="$values --set preflight.image.repository=${KINDTKS_REGISTRY}cilium/cilium"
    values="$values --set preflight.image.useDigest=false"
  fi

  helm install cilium "$KINDTKS_CHARTS_DIR/cilium-1.12.19.tgz" \
    --namespace kube-system \
    $values

  echo "==> Waiting for Cilium to be ready..."
  kubectl -n kube-system rollout status deployment/cilium-operator --timeout=120s
  kubectl -n kube-system rollout status daemonset/cilium --timeout=120s
}

install_istio() {
  echo "==> Installing Istio 1.16..."
  local hub_values=""
  if [ -n "${KINDTKS_REGISTRY:-}" ]; then
    hub_values="--set global.hub=${KINDTKS_REGISTRY}istio --set global.tag=1.16.7"
  fi

  helm install istio-base "$KINDTKS_CHARTS_DIR/base-1.16.7.tgz" \
    --namespace istio-system --create-namespace \
    $hub_values

  helm install istiod "$KINDTKS_CHARTS_DIR/istiod-1.16.7.tgz" \
    --namespace istio-system \
    --wait --timeout 120s \
    $hub_values

  echo "==> Installing Istio Ingress Gateway..."
  helm install istio-ingress "$KINDTKS_CHARTS_DIR/gateway-1.16.7.tgz" \
    --namespace istio-ingress --create-namespace \
    --wait --timeout 120s \
    $hub_values
}

install_vault() {
  echo "==> Installing Vault (dev mode)..."
  local values=""
  values="$values --set server.dev.enabled=true"
  values="$values --set injector.enabled=false"
  if [ -n "${KINDTKS_REGISTRY:-}" ]; then
    values="$values --set server.image.repository=${KINDTKS_REGISTRY}hashicorp/vault"
  fi

  helm install vault "$KINDTKS_CHARTS_DIR/vault-0.34.0.tgz" \
    --namespace vault --create-namespace \
    --wait --timeout 120s \
    $values
}

install_vault_secrets_operator() {
  echo "==> Installing Vault Secrets Operator..."
  local values=""
  if [ -n "${KINDTKS_REGISTRY:-}" ]; then
    values="$values --set controller.manager.image.repository=${KINDTKS_REGISTRY}hashicorp/vault-secrets-operator"
  fi

  helm install vault-secrets-operator "$KINDTKS_CHARTS_DIR/vault-secrets-operator-1.5.0.tgz" \
    --namespace vault-secrets-operator --create-namespace \
    --wait --timeout 120s \
    $values
}
