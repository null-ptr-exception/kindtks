# Per-registry hosts.toml Support

Let kindtks configure containerd's per-registry host files (`hosts.toml`) on every Kind node, so clusters can pull through a mirror or pull-through cache without changing image names.

## Problem

kindtks can only configure registries through containerd's legacy CRI options:

- Private registries found in `images:` are auto-trusted with `registry.mirrors` + `registry.configs.<reg>.tls`.
- `registryAuth` adds `registry.configs.<reg>.auth`.

There is no way to point a public registry (docker.io, quay.io, ...) at a mirror. The modern mechanism, `config_path` + `certs.d/<registry>/hosts.toml`, cannot be combined with the legacy options: containerd rejects `mirrors` and `configs.tls` when `config_path` is set (containerd v1.7.13 `pkg/cri/config/config.go`, `ValidatePluginConfig`). `configs.auth` is still accepted (deprecated).

## Config

`registryAuth` is replaced by a single `registries` map keyed by registry host (with optional port):

```yaml
registries:
  quay.io:
    hosts: |                      # optional: written to certs.d/quay.io/hosts.toml
      server = "https://quay.io"
      [host."http://zot:5000/v2/quay.io"]
        capabilities = ["pull", "resolve"]
        override_path = true
  registry.corp.com:
    auth:                         # optional: containerd configs.auth
      username: u
      password: p
```

- `hosts` is raw hosts.toml content, passed through verbatim.
- `auth` is kept as a CRI `configs.<reg>.auth` patch. hosts.toml has no credential field, and a static `Authorization` header would break token-auth registries.
- `registryAuth` is removed. `config.Load` switches to a strict decoder (`KnownFields(true)`), so a leftover `registryAuth` key, or any other unknown key, fails with an error.

### Merge

`config.Merge(base, override)` merges `registries` per registry and per field: an override that sets only `auth` keeps the base's `hosts`, and the other way round. For a field set in both, the override wins.

### Auto-trust

Each private registry detected by `PrivateRegistries()` (the existing well-known exclusion list is unchanged) without an explicit `hosts` gets a generated file:

```toml
server = "http://<reg>"
[host."http://<reg>"]
  capabilities = ["pull", "resolve"]
  skip_verify = true
```

An explicit `hosts` entry for the same registry replaces the generated file entirely. The legacy `mirrors` / `configs.tls` patch is removed.

## Generated host files

Location: `~/.local/share/kindtks/profiles/<profile>/certs.d/<registry>/hosts.toml`.

- The files must persist for the cluster's lifetime: containerd reads them on every pull and after node restarts, so a temp dir (as used for the patched kind config) does not work.
- `create` removes and rewrites the profile's `certs.d/` before running the profile.
- `delete` removes the profile's `certs.d/` after the profile's `delete()` succeeds.
- If there is nothing to write (no `hosts` entries and no private registries), no directory is created and the kind config gets no mount or `config_path`. Clusters are the same as today.
- Changing `registries.*.hosts` requires re-creating the cluster, like any other config change. Hand edits to the generated files take effect on running clusters but are overwritten by the next `create`.

## Kind config patching

`internal/kindconfig` works on the `yaml.Node` tree of the profile's `kind-config.yaml` and writes the result to a temp file, as today:

- When host files exist:
  - append `[plugins."io.containerd.grpc.v1.cri".registry] config_path = "/etc/containerd/certs.d"` to `containerdConfigPatches`;
  - add an `extraMounts` entry (`hostPath: <generated certs.d>`, `containerPath: /etc/containerd/certs.d`, `readOnly: true`) to **every** node in `nodes:`, keeping existing mounts and other node settings;
  - if the config has no `nodes:`, add a single `control-plane` node with the mount (kind's default topology).
- When any `auth` exists, append the `configs."<reg>".auth` patch.
- Existing top-level keys and existing `containerdConfigPatches` entries are kept.

## Errors

kindtks fails before invoking kind when:

- a `hosts` value is not valid TOML (syntax only; containerd validates the schema);
- a registry key is empty or contains `/` or `..` (it becomes a directory name);
- the config contains unknown keys (including `registryAuth`);
- a node already has an `extraMounts` entry with `containerPath: /etc/containerd/certs.d`.

## Testing

Unit tests:

- config: load `registries`, strict decoding rejects `registryAuth`, per-field merge, key and TOML validation.
- host file generation: auto-trust only, explicit `hosts` overrides auto-trust, auth-only registry produces no file, empty config produces nothing.
- kind config patching: multi-node gets mounts on every node, config with no `nodes:`, existing mounts and patches kept, conflicting mount is rejected, auth patch.

Integration (bats):

- create a cluster with a `registries.*.hosts` entry pointing at a pull-through cache; verify `/etc/containerd/certs.d/<reg>/hosts.toml` exists on every node and that a `crictl pull` reaches the cache.
- existing air-gap/private-registry tests keep passing with the hosts.toml-based auto-trust.

## Docs

README: replace "Custom Image Registry" and "Registry Authentication" with the `registries` form, covering:

- private HTTP registry via image overrides (auto-trust);
- authentication;
- pull-through mirror example using an override config that only contains `registries`.

Note in the README: the Kind node image (`kind-node`) is pulled by Docker on the host, so `registries` does not apply to it.

## Out of scope

- A `hostsFile: <path>` field that reads hosts.toml from disk (possible follow-up).
- Changing the committed gen1 `config.yaml`; mirror setups belong in user override configs.
