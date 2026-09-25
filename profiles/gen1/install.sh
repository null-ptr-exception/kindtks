REQUIRES="kind helm kubectl"

CLUSTER_NAME="gen1"

# Split IMAGE_* env vars into *_REPO and *_TAG for helm values
split_image_refs() {
  local vars
  vars=$(env | grep '^IMAGE_' | cut -d= -f1)
  for var in $vars; do
    local val="${!var}"
    export "${var}_REPO=${val%:*}"
    export "${var}_TAG=${val##*:}"
  done

  # Istio uses hub (registry/org) rather than full image repo
  if [ -n "${IMAGE_ISTIO_PILOT_REPO:-}" ]; then
    export ISTIO_HUB="${IMAGE_ISTIO_PILOT_REPO%/*}"
  fi
}

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
  split_image_refs

  echo "==> Creating Kind cluster '$CLUSTER_NAME' (k8s 1.24)..."
  kind create cluster \
    --name "$CLUSTER_NAME" \
    --image "${IMAGE_KIND_NODE}" \
    --config "${KINDTKS_KIND_CONFIG:-$KINDTKS_PROFILE_DIR/kind-config.yaml}"

  install_cilium

  echo "==> Waiting for nodes to be ready..."
  kubectl wait --for=condition=Ready nodes --all --timeout=600s
  install_storageclasses
  install_istio
  install_vault
  install_vault_secrets_operator

  echo ""
  echo "==> gen1 cluster is ready!"
  echo "    kubectl config use-context kind-$CLUSTER_NAME"
  echo ""
  echo "    Vault UI: http://vault.kindtks.localhost:30080"
  echo "              https://vault.kindtks.localhost:30443 (self-signed)"
  echo ""
  echo "    To expose a service, create a VirtualService:"
  echo "      gateways: [istio-ingress/kindtks]"
  echo "      hosts: [<app>.kindtks.localhost, <app>.kindtks.local]"
  echo "    Then access it at:"
  echo "      http://<app>.kindtks.localhost:30080"
  echo "      https://<app>.kindtks.localhost:30443"
  echo ""
  echo "    Remote access: use *.kindtks.local with /etc/hosts on remote machines"
}

delete() {
  echo "==> Deleting Kind cluster '$CLUSTER_NAME'..."
  kind delete cluster --name "$CLUSTER_NAME"
  echo "==> Cluster '$CLUSTER_NAME' deleted."
}

install_storageclasses() {
  echo "==> Creating per-DC StorageClasses (netapp-dc1, netapp-dc2, netapp-dc3)..."
  kubectl apply -f "$KINDTKS_PROFILE_DIR/storageclasses.yaml"
}

install_cilium() {
  echo "==> Installing Cilium 1.13..."
  helm_install cilium "$KINDTKS_CHARTS_DIR/cilium-1.13.10.tgz" kube-system

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

  echo "==> Creating TLS certificate for *.kindtks.localhost / *.kindtks.local..."
  local certdir
  certdir=$(mktemp -d)
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
    -keyout "$certdir/tls.key" -out "$certdir/tls.crt" -nodes -days 3650 \
    -subj "/CN=kindtks.localhost" \
    -addext "subjectAltName=DNS:*.kindtks.localhost,DNS:kindtks.localhost,DNS:*.kindtks.local,DNS:kindtks.local" 2>/dev/null
  kubectl -n istio-ingress create secret tls kindtks-tls \
    --cert="$certdir/tls.crt" --key="$certdir/tls.key"
  rm -rf "$certdir"

  echo "==> Creating wildcard Gateway (*.kindtks.localhost)..."
  kubectl apply -f "$KINDTKS_PROFILE_DIR/gateway.yaml"
  add_gateway_extra_hosts
}

# Adds profiles.gen1.gateway.extraHosts to the HTTP server (servers[0]) of
# the kindtks Gateway. HTTPS is untouched: kindtks-tls only covers the
# built-in domains.
add_gateway_extra_hosts() {
  local hosts host
  hosts=$("$KINDTKS_BIN" value gateway.extraHosts)
  while IFS= read -r host; do
    [ -n "$host" ] || continue
    echo "==> Adding gateway host ${host} (HTTP)..."
    kubectl -n istio-ingress patch gateway kindtks --type=json \
      -p "[{\"op\":\"add\",\"path\":\"/spec/servers/0/hosts/-\",\"value\":\"${host}\"}]"
  done <<< "$hosts"
}

install_vault() {
  echo "==> Installing Vault (dev mode)..."
  helm_install vault "$KINDTKS_CHARTS_DIR/vault-0.34.0.tgz" vault \
    --wait --timeout 600s

  echo "==> Exposing Vault UI at vault.kindtks.localhost..."
  kubectl apply -f "$KINDTKS_PROFILE_DIR/vault-virtualservice.yaml"
}

install_vault_secrets_operator() {
  echo "==> Installing Vault Secrets Operator (ricoberger) 1.26.0..."
  helm_install vault-secrets-operator "$KINDTKS_CHARTS_DIR/vault-secrets-operator-2.7.0.tgz" vault-secrets-operator \
    --wait --timeout 600s
}
