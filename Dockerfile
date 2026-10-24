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
