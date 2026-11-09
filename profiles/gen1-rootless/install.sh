REQUIRES="kind helm kubectl"

# gen1 without Cilium, for rootless Docker: kind's default CNI (kindnet)
# replaces Cilium. All other steps and files come from the gen1 profile.
GEN1_DIR="$(cd "$KINDTKS_PROFILE_DIR/../gen1" && pwd)"
source "$GEN1_DIR/install.sh"

CLUSTER_NAME="gen1-rootless"

install_cilium() { :; }
