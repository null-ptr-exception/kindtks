REQUIRES="kind helm kubectl"

# gen1 without Cilium, for rootless Docker: kind's default CNI (kindnet)
# replaces Cilium, and kube-network-policies enforces NetworkPolicy. All
# other steps and files come from the gen1 profile.
GEN1_DIR="$(cd "$KINDTKS_PROFILE_DIR/../gen1" && pwd)"
source "$GEN1_DIR/install.sh"

CLUSTER_NAME="gen1-rootless"

install_cni() {
  echo "==> Installing kube-network-policies 1.1.2..."
  helm_install kube-network-policies "$KINDTKS_CHARTS_DIR/kube-network-policies-1.1.2.tgz" kube-system

  echo "==> Waiting for kube-network-policies to be ready..."
  kubectl -n kube-system rollout status daemonset/kube-network-policies --timeout=600s
}
