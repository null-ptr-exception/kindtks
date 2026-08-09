# kindtks Design Spec

An opinionated tool for bootstrapping customized Kind clusters with pre-defined profiles.

## Problem

Team members need consistent local Kubernetes environments with specific versions and add-ons (Istio, Cilium, Vault, etc.) pre-configured. Setting these up manually is tedious, error-prone, and hard to keep consistent across the team.

## Solution

A Go CLI (`kindtks`) that executes shell-script-based profiles to bootstrap fully configured Kind clusters. Distributed as a Docker image that self-installs to the host.

## Target Users

Team members who need local K8s dev environments. Assumes Kubernetes familiarity.

## User Experience

### First-time setup

```bash
# Shows AI-friendly setup instructions
docker run --rm kindtks

# Installs kindtks binary and profiles to host
docker run --rm \
  -v ~/.local/bin:/.local/bin \
  -v ~/.local/share/kindtks:/.local/share/kindtks \
  kindtks install
```

### Daily use

```bash
kindtks create gen1        # single cluster: k8s 1.24, istio, cilium, vault
kindtks create gen1-fz2    # multi-cluster with service mesh
kindtks delete gen1        # tear down clusters from a profile
kindtks list               # show available profiles
```

## Architecture

### Components

1. **Go CLI binary** (`kindtks`) — command parsing, prerequisite checks, profile discovery, profile script execution
2. **Profile scripts** (`~/.local/share/kindtks/profiles/<name>.sh`) — shell scripts with full orchestration logic per profile
3. **Docker image** — bundles both for distribution; entrypoint shows instructions or runs install

### Directory layout

```
kindtks/
├── cmd/                    # Go CLI entrypoint
├── internal/
│   ├── cli/                # command implementations (create, delete, list, install)
│   ├── profile/            # profile discovery and execution
│   └── prereq/             # prerequisite checking
├── profiles/               # profile shell scripts (baked into Docker image)
│   ├── gen1.sh
│   └── gen1-fz2.sh
├── Dockerfile
└── docs/
```

### Profile script contract

Each profile is a bash script (e.g. `profiles/gen1.sh`) that must:

1. **Declare required tools** via a `REQUIRES` variable:
   ```bash
   REQUIRES="kind helm kubectl istioctl"
   ```

2. **Define a `create()` function** that bootstraps the cluster(s):
   ```bash
   create() {
     kind create cluster --name gen1 --image kindest/node:v1.24.0
     helm install istio-base istio/base -n istio-system --create-namespace
     # ... wait for CRDs, install gateway, etc.
   }
   ```

3. **Define a `delete()` function** that tears everything down:
   ```bash
   delete() {
     kind delete cluster --name gen1
   }
   ```

The Go CLI sources the script, checks prerequisites, then calls the appropriate function.

### Environment variables provided to profile scripts

| Variable | Description |
|---|---|
| `KINDTKS_DATA_DIR` | Path to `~/.local/share/kindtks` |
| `KINDTKS_PROFILE_NAME` | Name of the current profile |

### Multi-cluster profiles

A profile can create multiple clusters. For example, `gen1-fz2.sh` might:

1. Create cluster A and cluster B
2. Install Istio on both
3. Configure cross-cluster mesh peering

The profile script owns the full orchestration logic — kindtks does not impose any structure on multi-cluster setups.

## Prerequisite Management

**Out of scope.** kindtks does not install prerequisites (kind, helm, kubectl, istioctl, etc.). Users manage these themselves via mise, brew, or whatever their environment provides.

**At runtime**, before executing a profile, kindtks:
1. Reads the `REQUIRES` variable from the profile script
2. Checks each tool is on `PATH`
3. Fails with a clear error listing missing tools

## Docker Image

### Contents

- `kindtks` Go binary at `/usr/local/bin/kindtks`
- Profile scripts at `/profiles/`

### Entrypoint behavior

- **No args** (`docker run --rm kindtks`): prints AI-friendly setup instructions explaining how to install and use kindtks
- **`install`** (`docker run --rm -v ~/.local/bin:/.local/bin -v ~/.local/share/kindtks:/.local/share/kindtks kindtks install`): copies `kindtks` binary to `/.local/bin/kindtks` (mapped to host's `~/.local/bin`) and profiles to `/.local/share/kindtks/profiles/` (mapped to host's `~/.local/share/kindtks/profiles/`)

### Air-gapped support

The Docker image can be exported as a tarball (`docker save`) and loaded on air-gapped machines (`docker load`), making it suitable for corporate environments without internet access.

## Add-on installation

Each add-on within a profile uses whatever install method fits best:
- **Helm charts** for add-ons distributed as charts (Istio, Cilium)
- **Raw manifests** (`kubectl apply`) for simpler components
- **CLI tools** (e.g. `istioctl install`) where appropriate

The profile script decides — kindtks imposes no constraint on install method.

## Non-goals

- Managing prerequisites (kind, helm, kubectl, etc.)
- Runtime version management (mise, asdf, etc.)
- Profile composition or layering (profiles are standalone)
- GUI or web interface
