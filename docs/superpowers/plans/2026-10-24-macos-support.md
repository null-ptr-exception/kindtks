# macOS Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install and run kindtks on Apple Silicon Macs with Colima, from the same image and one-liner as on Linux.

**Architecture:** The image becomes multi-arch (`linux/amd64`, `linux/arm64`) and also carries macOS binaries in `/dist/darwin-arm64/`. The installer picks the binary directory by `uname -s`/`uname -m`. All build stages except the final `scratch` run on the build platform, so no QEMU is needed.

**Tech Stack:** Dockerfile (BuildKit, buildx), POSIX sh (fetch script), bash 3.2-compatible installer, bats, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-24-macos-support-design.md`

## Global Constraints

- Supported: macOS on Apple Silicon (`darwin/arm64`) with Colima. Not supported: Intel Macs, Docker Desktop, OrbStack. Linux arm64 is not documented as supported.
- Tool versions stay kind v0.33.0, kubectl v1.33.1, helm v3.21.0. Every download is pinned with its SHA-256.
- The installer must run under macOS `/bin/bash` 3.2.57 and BSD userland. Test files must not use GNU-only commands (`head -n -1`, `stat -c`, `sed -i` without suffix, `readlink -f`).
- Commit format `<type>: <description>`, 1-5 lines. No mention of Claude. No `Co-Authored-By`. Never disable GPG signing.
- No push, no GitHub changes, no PR without the user's explicit instruction.
- Nothing committed may contain internal hostnames or IP addresses (for example the Mac test host address). `*.kindtks.localhost` is allowed: it is part of kindtks.
- Work on branch `macos-support`.

---

### Task 1: Multi-arch image with macOS binaries

**Files:**
- Create: `tools.lock`
- Create: `scripts/fetch-tools.sh`
- Modify: `Dockerfile` (whole file)

**Interfaces:**
- Produces: image paths `/usr/local/bin/{kindtks,kind,kubectl,helm}` (Linux, image architecture) and `/dist/darwin-arm64/{kindtks,kind,kubectl,helm}` (macOS arm64), in both `linux/amd64` and `linux/arm64` variants. Label `io.kindtks.build` and env `KINDTKS_BUILD_ID` unchanged.

- [ ] **Step 1: Create `tools.lock`**

All checksums were checked against upstream `.sha256`/`.sha256sum` files on 2026-10-24.

```
# Bundled tools. Columns: platform tool sha256 url
# Bump a version by changing the url and its sha256 together.
linux-amd64  kind    aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d https://kind.sigs.k8s.io/dl/v0.33.0/kind-linux-amd64
linux-amd64  kubectl 5de4e9f2266738fd112b721265a0c1cd7f4e5208b670f811861f699474a100a3 https://dl.k8s.io/release/v1.33.1/bin/linux/amd64/kubectl
linux-amd64  helm    0093eb572e3d2380f094df162ddb525e219249de88957afe24cfbb19632acd36 https://get.helm.sh/helm-v3.21.0-linux-amd64.tar.gz
linux-arm64  kind    20022bee6cfcd5086cb7234d218e3454e6090022f2a8f55d1fa7fcf42c3867a2 https://kind.sigs.k8s.io/dl/v0.33.0/kind-linux-arm64
linux-arm64  kubectl d595d1a26b7444e0beb122e25750ee4524e74414bbde070b672b423139295ce6 https://dl.k8s.io/release/v1.33.1/bin/linux/arm64/kubectl
linux-arm64  helm    8de5a0c9a47431e59fd560e91e0779c8cf9316c383da7efb84128a4c339ecb2d https://get.helm.sh/helm-v3.21.0-linux-arm64.tar.gz
darwin-arm64 kind    0c8c7dbe5e23594a198b786c4bc13dacc101fa6196b0cb0b23a1ca44e61f4b4f https://kind.sigs.k8s.io/dl/v0.33.0/kind-darwin-arm64
darwin-arm64 kubectl 8ae6823839993bb2e394c3cf1919748e530642c625dc9100159595301f53bdeb https://dl.k8s.io/release/v1.33.1/bin/darwin/arm64/kubectl
darwin-arm64 helm    68bfbdc022c543a2a022597b20298216877e98abe6e4a345d3ecf114d79cae5f https://get.helm.sh/helm-v3.21.0-darwin-arm64.tar.gz
```

- [ ] **Step 2: Create `scripts/fetch-tools.sh`** (mode 0755)

```sh
#!/bin/sh
# fetch-tools.sh LOCK OUT: download every tool in LOCK, check its SHA-256,
# and install it as OUT/<platform>/<tool>. Helm tarballs hold <platform>/helm.
set -eu
lock="$1"
out="$2"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
grep -v '^#' "$lock" | while read -r platform tool sha url; do
  [ -n "$platform" ] || continue
  mkdir -p "$out/$platform"
  wget -qO "$work/dl" "$url"
  if ! echo "$sha  $work/dl" | sha256sum -c - >/dev/null 2>&1; then
    echo "checksum mismatch: $url" >&2
    exit 1
  fi
  case "$url" in
    *.tar.gz)
      tar xzf "$work/dl" -C "$work" "$platform/$tool"
      install -m 0755 "$work/$platform/$tool" "$out/$platform/$tool"
      ;;
    *) install -m 0755 "$work/dl" "$out/$platform/$tool" ;;
  esac
done
```

The `exit 1` runs in the pipeline's subshell; the pipeline's status is the `while` loop's, so `set -e` stops the script.

- [ ] **Step 3: Check the script fails on a wrong checksum**

```bash
cd /home/rophy/projects/kindtks
S=$(mktemp -d)
printf 'linux-amd64 kind 0000000000000000000000000000000000000000000000000000000000000000 https://kind.sigs.k8s.io/dl/v0.33.0/kind-linux-amd64\n' > "$S/bad.lock"
docker run --rm -v "$PWD/scripts:/s:ro" -v "$S:/w" alpine:3.20.10 sh /s/fetch-tools.sh /w/bad.lock /w/out; echo "exit=$?"
rm -rf "$S"
```
Expected: `checksum mismatch: https://kind.sigs.k8s.io/dl/v0.33.0/kind-linux-amd64` and `exit=1`.

- [ ] **Step 4: Replace `Dockerfile`**

```dockerfile
# Every tool is pinned: Go and Alpine images here, kind/kubectl/helm in
# tools.lock. All stages but the last run on the build platform, so a
# multi-platform build needs no emulation.
ARG GO_IMAGE=golang:1.24.13@sha256:d2d2bc1c84f7e60d7d2438a3836ae7d0c847f4888464e7ec9ba3a1339a1ee804
ARG ALPINE_IMAGE=alpine:3.20.10@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc
# apk keeps only the latest revision per branch: a vanished pin fails the
# build on purpose, as the signal to bump it.
ARG BASH_APK=bash=5.2.26-r0

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS gosrc
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM gosrc AS kindtks-linux
ARG VERSION=dev
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags "-s -w -X main.version=${VERSION}" -o /kindtks .

FROM gosrc AS kindtks-darwin
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
    go build -ldflags "-s -w -X main.version=${VERSION}" -o /kindtks .

FROM --platform=$BUILDPLATFORM ${ALPINE_IMAGE} AS tools
COPY tools.lock scripts/fetch-tools.sh /dl/
RUN /dl/fetch-tools.sh /dl/tools.lock /out

FROM --platform=$BUILDPLATFORM ${ALPINE_IMAGE} AS charts
ARG BASH_APK
ARG BUILDOS
ARG BUILDARCH
RUN apk add --no-cache "${BASH_APK}"
COPY --from=tools /out/${BUILDOS}-${BUILDARCH}/helm /usr/local/bin/helm
COPY scripts/pull-charts.sh /pull-charts.sh
RUN /pull-charts.sh /charts

FROM scratch
ARG TARGETARCH
COPY --from=tools /out/linux-${TARGETARCH}/ /usr/local/bin/
COPY --from=kindtks-linux /kindtks /usr/local/bin/kindtks
COPY --from=tools /out/darwin-arm64/ /dist/darwin-arm64/
COPY --from=kindtks-darwin /kindtks /dist/darwin-arm64/kindtks
COPY profiles/ /profiles/
COPY --from=charts /charts/ /charts/
# Unique per build: the installer finds this image on the host by the label.
ARG BUILD_ID=dev
LABEL io.kindtks.build=${BUILD_ID}
ENV KINDTKS_BUILD_ID=${BUILD_ID}
ENTRYPOINT ["/usr/local/bin/kindtks"]
```

- [ ] **Step 5: Build both platforms and check the contents**

The local Docker uses the containerd image store, so a multi-platform build loads directly.

```bash
cd /home/rophy/projects/kindtks
docker build --platform linux/amd64,linux/arm64 --build-arg BUILD_ID=plan-check -t kindtks:multiarch .
for p in amd64 arm64; do
  cid=$(docker create --platform linux/$p kindtks:multiarch)
  d=$(mktemp -d)
  docker cp "$cid:/usr/local/bin" "$d/linux"
  docker cp "$cid:/dist/darwin-arm64" "$d/darwin"
  docker rm "$cid" >/dev/null
  echo "== $p"; file "$d"/linux/* "$d"/darwin/* | sed "s#$d/##"
  rm -rf "$d"
done
docker run --rm --platform linux/amd64 kindtks:multiarch version
docker image inspect kindtks:multiarch --format '{{index .Config.Labels "io.kindtks.build"}}'
```
Expected: for `amd64`, `linux/*` are `ELF 64-bit LSB executable, x86-64`; for `arm64`, `ARM aarch64`; in both, `darwin/*` are `Mach-O 64-bit arm64 executable`. `version` prints `kindtks version dev`. The label prints `plan-check`.

- [ ] **Step 6: Run the existing tests**

```bash
go test ./...
bats test/unit
bats test/e2e/installer.bats
docker rmi kindtks:multiarch
```
Expected: all pass (e2e builds the image for the host platform).

- [ ] **Step 7: Commit**

```bash
git add tools.lock scripts/fetch-tools.sh Dockerfile
git commit -m "build: multi-arch image with macOS arm64 binaries"
```

---

### Task 2: Installer picks binaries by host platform

**Files:**
- Modify: `internal/cmd/installer.sh`
- Modify: `test/unit/installer.bats`

**Interfaces:**
- Consumes: image paths from Task 1.

- [ ] **Step 1: Update the unit test fixtures**

In `test/unit/installer.bats`:

`fake_tool` takes an optional platform for kind:
```bash
fake_tool() { # fake_tool DIR NAME VERSION [PLATFORM]
  local out
  case "$2" in
    kind) out="kind $3 go1.26.7 ${4:-linux/amd64}" ;;
    kubectl) out="Client Version: $3" ;;
    helm) out="$3+gabcdef0" ;;
  esac
  mkdir -p "$1"
  printf '#!/usr/bin/env bash\necho "%s"\n' "$out" > "$1/$2"
  chmod +x "$1/$2"
}
```

In `setup()`, replace the fake kindtks line with Linux and darwin variants, and add the darwin tools:
```bash
  printf '#!/usr/bin/env bash\necho kindtks linux\n' > "$FAKE_IMAGE_ROOT/usr/local/bin/kindtks"
  chmod +x "$FAKE_IMAGE_ROOT/usr/local/bin/kindtks"
  DARWIN="$FAKE_IMAGE_ROOT/dist/darwin-arm64"
  fake_tool "$DARWIN" kind v0.33.0 darwin/arm64
  fake_tool "$DARWIN" kubectl v1.33.1
  fake_tool "$DARWIN" helm v3.21.0
  printf '#!/usr/bin/env bash\necho kindtks darwin\n' > "$DARWIN/kindtks"
  chmod +x "$DARWIN/kindtks"
```

After the fake docker is written, add a fake `uname` (default Linux x86_64, so the tests give the same result on a macOS runner):
```bash
  cat > "$MOCK_BIN/uname" <<'MOCK'
#!/usr/bin/env bash
case "$1" in
  -s) echo "${FAKE_UNAME_S:-Linux}" ;;
  -m) echo "${FAKE_UNAME_M:-x86_64}" ;;
  *) /usr/bin/uname "$@" ;;
esac
MOCK
  chmod +x "$MOCK_BIN/uname"
```

In the test "installs kindtks and the private copies", after `assert_success`, add:
```bash
  run "$DEST/kindtks"
  assert_output "kindtks linux"
```
and change the final `run cat "$DOCKER_LOG"` block to keep its two assertions plus:
```bash
  assert_output --partial "cp fakecid:/usr/local/bin/kindtks"
```

In "a truncated installer runs nothing", replace `head -n -1 "$INSTALLER"` with `sed '$d' "$INSTALLER"`.

Add these tests at the end:
```bash
@test "macOS on Apple Silicon gets the darwin binaries" {
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 run bash "$INSTALLER" --yes
  assert_success
  run "$DEST/kindtks"
  assert_output "kindtks darwin"
  run "$DATA/bin/kind"
  assert_output --partial "darwin/arm64"
  run "$DEST/kind"
  assert_output --partial "darwin/arm64"
}

@test "Intel Mac is refused before anything is installed" {
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=x86_64 run bash "$INSTALLER" --yes
  assert_failure 1
  assert_output --partial "error: Intel Macs are not supported"
  [ ! -e "$HOME/.local" ]
}

@test "other systems are refused" {
  FAKE_UNAME_S=FreeBSD FAKE_UNAME_M=amd64 run bash "$INSTALLER" --yes
  assert_failure 1
  assert_output --partial "error: unsupported system: FreeBSD amd64"
}
```

- [ ] **Step 2: Run the tests and see the new ones fail**

Run: `bats test/unit/installer.bats`
Expected: the three new tests fail (darwin run installs `kindtks linux`; Intel and FreeBSD runs succeed). The earlier tests pass.

- [ ] **Step 3: Change the installer**

In `internal/cmd/installer.sh`, header comment: replace `# It finds the image on this host by its build label, copies kindtks and the` / `# bundled kind, kubectl and helm out of it, ...` with:
```bash
# It finds the image on this host by its build label, copies kindtks and the
# bundled kind, kubectl and helm for this host (Linux, or macOS on Apple
# Silicon) out of it, and offers the bundled tools where the host lacks them
# or has older ones.
```

In `main`, right after `DEST="${DEST%/}"`, add:
```bash

  # 0. The binaries for this host. Linux hosts run the image's own
  # architecture, so its /usr/local/bin fits.
  case "$(uname -s)/$(uname -m)" in
    Linux/*) SRC=/usr/local/bin ;;
    Darwin/arm64) SRC=/dist/darwin-arm64 ;;
    Darwin/*) die "Intel Macs are not supported" ;;
    *) die "unsupported system: $(uname -s) $(uname -m)" ;;
  esac
```

Replace `copy_out "/usr/local/bin/$tool" "$DATA_DIR/bin.new/$tool"` with `copy_out "$SRC/$tool" "$DATA_DIR/bin.new/$tool"`, and `copy_out /usr/local/bin/kindtks "$tmp"` with `copy_out "$SRC/kindtks" "$tmp"`.

- [ ] **Step 4: Run the tests**

Run: `bats test/unit/installer.bats && go test ./internal/cmd/`
Expected: 17/17 pass; Go tests pass (`installscript.go` embeds the script).

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/installer.sh test/unit/installer.bats
git commit -m "feat: installer copies macOS binaries on Apple Silicon"
```

---

### Task 3: Portable e2e installer test

**Files:**
- Modify: `test/e2e/installer.bats`

- [ ] **Step 1: Replace the ownership test and add a platform test**

Replace the body of "installed files belong to the user" with (`-O`: owned by the effective user; works on GNU and BSD):
```bash
  for f in "$H/.local/bin/kindtks" "$H/.local/share/kindtks/bin/kind" \
    "$H/.local/share/kindtks/bin/kubectl" "$H/.local/share/kindtks/bin/helm"; do
    [ -x "$f" ]
    [ -O "$f" ]
  done
  [ -O "$H/.local/share/kindtks/profiles/gen1/install.sh" ]
```

Add after it:
```bash
@test "installed binaries are built for this host" {
  local os
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  run "$H/.local/share/kindtks/bin/kind" version
  assert_success
  assert_output --partial " $os/"
  run "$H/.local/bin/kindtks" version
  assert_success
}
```

- [ ] **Step 2: Run on Linux**

Run: `bats test/e2e/installer.bats`
Expected: all pass, including `linux/` in the kind version.

- [ ] **Step 3: Commit**

```bash
git add test/e2e/installer.bats
git commit -m "test: e2e installer checks are portable to macOS"
```

---

### Task 4: CI and release workflows

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`

- [ ] **Step 1: `ci.yml`**

Replace the `Docker build` step of `build-and-test` with:
```yaml
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1

      - name: Docker build (linux/amd64, linux/arm64)
        run: docker buildx build --platform linux/amd64,linux/arm64 -t kindtks:ci .
```

Add a second job:
```yaml
  macos:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7

      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
        with:
          go-version-file: go.mod

      - name: Test
        run: go test ./...

      - name: Installer test (bash 3.2, BSD tools)
        run: |
          brew install bats-core
          bats test/unit/installer.bats
```

- [ ] **Step 2: `release.yml`**

In `publish`, before the `docker/build-push-action` step, add:
```yaml
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1
```
In the `docker/build-push-action` step's `with:`, add `platforms: linux/amd64,linux/arm64`.

In the release notes `printf`, replace `Requires Docker and bash.` with `Requires Docker and bash: Linux, WSL2, or macOS on Apple Silicon with Colima.`

- [ ] **Step 3: Validate the YAML**

Run: `for f in .github/workflows/*.yml; do python3 -c 'import sys,yaml; yaml.safe_load(open(sys.argv[1]))' "$f" && echo "ok $f"; done`
Expected: `ok` for both files.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml .github/workflows/release.yml
git commit -m "build: multi-arch release image and macOS CI job"
```

---

### Task 5: README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Requirements line**

Replace `Requires Docker and bash (Linux or WSL2).` with:
```markdown
Requires Docker and bash: Linux or WSL2, or macOS on Apple Silicon with Colima (see [macOS](#macos)).
```

- [ ] **Step 2: Add a `### macOS` section** between the "Bundled versions" paragraph and `### Upgrading`:

````markdown
### macOS

Apple Silicon only, with [Colima](https://github.com/abiosoft/colima) as the Docker runtime:

```bash
brew install colima docker
colima start --cpu 4 --memory 4   # gen1 uses about 2.2 GiB; Colima's default 2 GiB is too small
```

Then run the installer above. With zsh, put the PATH line it prints in `~/.zprofile`.

`*.kindtks.localhost` names resolve in browsers and curl only; other programs (for example Python, Go and Java clients, `ping`, `ssh`) report an unknown host, because macOS resolves only `localhost` itself. Add one `/etc/hosts` line per name you need:

```bash
echo '127.0.0.1 vault.kindtks.localhost' | sudo tee -a /etc/hosts
```

For all names at once, run dnsmasq (from brew) answering `*.localhost` with `127.0.0.1`, and point `/etc/resolver/localhost` at it.
````

- [ ] **Step 3: Bundled versions line**

Replace `Bundled versions: kind v0.33.0, kubectl v1.33.1, helm v3.21.0.` with `Bundled versions (Linux amd64/arm64, macOS arm64): kind v0.33.0, kubectl v1.33.1, helm v3.21.0.` Keep the rest of the paragraph.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: macOS install with Colima and .localhost names"
```

---

### Task 6: Verify on the Mac

Manual, by the controller (not a subagent): the Mac is shared and its use is agreed with the `infra` peer. Connection details and rules are in memory (`reference-mac-test-host`). In every Mac command: `eval "$(/opt/homebrew/bin/brew shellenv)"`. Nothing installed system-wide; everything under `~/kindtks-test`.

- [ ] **Step 1: Prepare Colima** — tell `infra`, check `pgrep -fl Runner.Worker` prints nothing, then `colima stop && colima start --cpu 4 --memory 4`. Record whether `registry:2` is already in the image cache (`docker images -q registry:2`): the runner's cache must not lose it in Step 7.

- [ ] **Step 2: Copy the branch and bats**

```bash
rsync -a --delete --exclude .git --exclude kindtks /home/rophy/projects/kindtks/ <mac>:kindtks-test/src/
ssh <mac> 'git clone -q --depth 1 https://github.com/bats-core/bats-core ~/kindtks-test/bats-core'
```

- [ ] **Step 3: Unit installer test under bash 3.2**

`ssh <mac> 'cd ~/kindtks-test/src && ~/kindtks-test/bats-core/bin/bats test/unit/installer.bats'` → 17/17.

- [ ] **Step 4: Go tests on darwin** (no Go on the Mac: cross-compile test binaries)

```bash
S=$(mktemp -d)
for p in $(go list ./...); do
  rel=${p#github.com/rophy/kindtks}; rel=${rel#/}; [ -n "$rel" ] || rel=.
  GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c -o "$S/$(echo "$rel" | tr / _).test" "./$rel"
done
scp -q "$S"/*.test <mac>:kindtks-test/
ssh <mac> 'cd ~/kindtks-test && for t in *.test; do d=src/$(echo "${t%.test}" | tr _ /); (cd "$d" && "$HOME/kindtks-test/$t") || echo "FAIL $t"; done'
```
Expected: every binary prints `PASS`, no `FAIL`.

- [ ] **Step 5: e2e installer test** (the test sets `HOME`, so point docker at Colima explicitly)

`ssh <mac> 'export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock; cd ~/kindtks-test/src && ~/kindtks-test/bats-core/bin/bats test/e2e/installer.bats'` → all pass, including `darwin/` in the kind version.

- [ ] **Step 6: Install and create gen1 from the image**

```bash
ssh <mac> 'set -e; export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock T=$HOME/kindtks-test
  cd $T/src && docker build -q --build-arg BUILD_ID=mac-$(date +%s) -t kindtks:mac-test .
  docker run --rm kindtks:mac-test install-script | HOME=$T/home bash -s -- --yes
  HOME=$T/home $T/home/.local/bin/kindtks create gen1 > $T/create.log 2>&1'
```
Then check: all pods Running (`$T/home/.local/share/kindtks/bin/kubectl --kubeconfig $T/home/.kube/config get pods -A`), `curl -s -o /dev/null -w "%{http_code}" http://vault.kindtks.localhost:30080/ui/` → `200`.

- [ ] **Step 7: Clean up and restore** — with `DOCKER_HOST` set as in Step 5: `HOME=$T/home $T/home/.local/bin/kindtks delete gen1`; `docker rmi kindtks:mac-test`, and `registry:2` only if Step 1 found it absent; `rm -rf ~/kindtks-test`; `colima stop && colima start --cpu 2 --memory 2 && brew services restart colima`; check `brew services list` and `colima list` (2 CPU / 2 GiB); report to `infra`.
