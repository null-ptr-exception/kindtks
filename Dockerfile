# Every tool is pinned; bump a version together with its SHA-256.
ARG GO_IMAGE=golang:1.24.13@sha256:d2d2bc1c84f7e60d7d2438a3836ae7d0c847f4888464e7ec9ba3a1339a1ee804
ARG ALPINE_IMAGE=alpine:3.20.10@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc
# apk keeps only the latest revision per branch: a vanished pin fails the
# build on purpose, as the signal to bump it.
ARG BASH_APK=bash=5.2.26-r0

FROM --platform=linux/amd64 ${GO_IMAGE} AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /kindtks .

FROM --platform=linux/amd64 ${ALPINE_IMAGE} AS tools
ARG KIND_VERSION=0.33.0
ARG KIND_SHA256=aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d
ARG KUBECTL_VERSION=1.33.1
ARG KUBECTL_SHA256=5de4e9f2266738fd112b721265a0c1cd7f4e5208b670f811861f699474a100a3
ARG HELM_VERSION=3.21.0
ARG HELM_SHA256=0093eb572e3d2380f094df162ddb525e219249de88957afe24cfbb19632acd36
WORKDIR /dl
RUN mkdir /out \
 && wget -qO kind "https://kind.sigs.k8s.io/dl/v${KIND_VERSION}/kind-linux-amd64" \
 && echo "${KIND_SHA256}  kind" | sha256sum -c - \
 && install -m 0755 kind /out/kind \
 && wget -qO kubectl "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl" \
 && echo "${KUBECTL_SHA256}  kubectl" | sha256sum -c - \
 && install -m 0755 kubectl /out/kubectl \
 && wget -qO helm.tgz "https://get.helm.sh/helm-v${HELM_VERSION}-linux-amd64.tar.gz" \
 && echo "${HELM_SHA256}  helm.tgz" | sha256sum -c - \
 && tar xzf helm.tgz linux-amd64/helm \
 && install -m 0755 linux-amd64/helm /out/helm

FROM --platform=linux/amd64 ${ALPINE_IMAGE} AS charts
ARG BASH_APK
RUN apk add --no-cache "${BASH_APK}"
COPY --from=tools /out/helm /usr/local/bin/helm
COPY scripts/pull-charts.sh /pull-charts.sh
RUN /pull-charts.sh /charts

FROM --platform=linux/amd64 scratch
COPY --from=tools /out/ /usr/local/bin/
COPY --from=builder /kindtks /usr/local/bin/kindtks
COPY profiles/ /profiles/
COPY --from=charts /charts/ /charts/
# Unique per build: the installer finds this image on the host by the label.
ARG BUILD_ID=dev
LABEL io.kindtks.build=${BUILD_ID}
ENV KINDTKS_BUILD_ID=${BUILD_ID}
ENTRYPOINT ["/usr/local/bin/kindtks"]
