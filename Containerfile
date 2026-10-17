# ─────────────────────────────────────────────────────────────────────────────
#  Containerfile — Okub development execution environment.
#
#  Purpose:  Single container image to run the justfile as if you were on your
#            laptop: build/test the Go CLI and drive the OKD tooling.
#
#  Build:    just build          (multi-arch: amd64, arm64)
#
#  Base:     Ubuntu 24.04 LTS (Noble) — matches standard WSL.
#
#  Note:     openshift-install, coreos-installer and butane are deliberately
#            NOT baked in. The okub CLI resolves and downloads pinned copies
#            into its install path at runtime (see docs/00-repo-contract.md), so
#            bundling them here would only risk version drift.
# ─────────────────────────────────────────────────────────────────────────────

FROM ubuntu:24.04

ARG VERSION=0.0.0
LABEL org.opencontainers.image.title="okub-ee"
LABEL org.opencontainers.image.description="Development execution environment for the okub CLI/TUI"
LABEL org.opencontainers.image.version="${VERSION}"

# ── Prevent apt from prompting for configuration ─────────────────────────────
ENV DEBIAN_FRONTEND=noninteractive

# ── Tool versions (pinned for reproducibility) ───────────────────────────────
ARG GO_VERSION=1.27.1
ARG OKD_VERSION=5.0.0-okd-scos.2
ARG TOFU_VERSION=1.13.1
ARG JUST_VERSION=1.58.0

# ── Normalise the target architecture (amd64 | arm64) ────────────────────────
# TARGETARCH is set by BuildKit/Buildah; fall back to uname for plain builds.
ARG TARGETARCH
RUN set -eux; \
    arch="${TARGETARCH:-$(uname -m)}"; \
    case "${arch}" in \
      amd64|x86_64) arch=amd64 ;; \
      arm64|aarch64) arch=arm64 ;; \
      *) echo "unsupported architecture: ${arch}" >&2; exit 1 ;; \
    esac; \
    echo "${arch}" > /tmp/okub-arch; \
    echo "Building for: ${arch}"

# ── Install system dependencies ─────────────────────────────────────────────
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    wget \
    gnupg \
    lsb-release \
    git \
    make \
    jq \
    dnsutils \
    openssh-client \
    sshpass \
    tar \
    gzip \
    xz-utils \
    unzip \
    skopeo \
    qemu-utils \
    libvirt-clients \
    && rm -rf /var/lib/apt/lists/*

# ── Install Go (the okub CLI is written in Go) ───────────────────────────────
RUN set -eux; \
    arch="$(cat /tmp/okub-arch)"; \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" -o /tmp/go.tgz; \
    rm -rf /usr/local/go; \
    tar -C /usr/local -xzf /tmp/go.tgz; \
    rm /tmp/go.tgz
ENV PATH=/usr/local/go/bin:/root/go/bin:$PATH

# ── Install the OKD client (oc + kubectl), verified against the release sums ──
RUN set -eux; \
    arch="$(cat /tmp/okub-arch)"; \
    case "${arch}" in \
      amd64) oc_arch="" ;; \
      arm64) oc_arch="-${arch}" ;; \
    esac; \
    asset="openshift-client-linux${oc_arch}-${OKD_VERSION}.tar.gz"; \
    curl -fsSL -o "/tmp/${asset}" "https://github.com/okd-project/okd/releases/download/${OKD_VERSION}/${asset}"; \
    curl -fsSL -o /tmp/sha256sum.txt "https://github.com/okd-project/okd/releases/download/${OKD_VERSION}/sha256sum.txt"; \
    awk -v f="${asset}" '$2 == f' /tmp/sha256sum.txt > /tmp/oc.sha; \
    test -s /tmp/oc.sha; \
    ( cd /tmp && sha256sum -c oc.sha ); \
    tar -xzf "/tmp/${asset}" -C /usr/local/bin oc kubectl; \
    rm -f "/tmp/${asset}" /tmp/sha256sum.txt /tmp/oc.sha

# ── Install OpenTofu (used by the libvirt lab) ───────────────────────────────
RUN set -eux; \
    arch="$(cat /tmp/okub-arch)"; \
    curl -fsSL "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_linux_${arch}.zip" -o /tmp/tofu.zip; \
    unzip -o /tmp/tofu.zip -d /usr/local/bin tofu; \
    chmod +x /usr/local/bin/tofu; \
    rm /tmp/tofu.zip

# ── Install Helm (renders the okub-ee chart for `podman play kube`) ──────────
# Unpinned: resolves the latest release at build time.
RUN set -eux; \
    arch="$(cat /tmp/okub-arch)"; \
    helm_ver="$(curl -fsSL https://get.helm.sh/helm-latest-version)"; \
    curl -fsSL "https://get.helm.sh/helm-${helm_ver}-linux-${arch}.tar.gz" -o /tmp/helm.tar.gz; \
    tar -xzf /tmp/helm.tar.gz --strip-components=1 -C /usr/local/bin "linux-${arch}/helm"; \
    rm /tmp/helm.tar.gz

# ── Install yq (unpinned: tracks the latest release) ─────────────────────────
RUN set -eux; \
    arch="$(cat /tmp/okub-arch)"; \
    curl -fsSL -o /usr/local/bin/yq "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_${arch}"; \
    chmod +x /usr/local/bin/yq

# ── Install just ─────────────────────────────────────────────────────────────
RUN set -eux; \
    arch="$(cat /tmp/okub-arch)"; \
    case "${arch}" in \
      amd64) musl="x86_64" ;; \
      arm64) musl="aarch64" ;; \
    esac; \
    curl -fsSL "https://github.com/casey/just/releases/download/${JUST_VERSION}/just-${JUST_VERSION}-${musl}-unknown-linux-musl.tar.gz" \
      | tar -xz -C /usr/local/bin just; \
    chmod +x /usr/local/bin/just

# ── Verify all tools are installed (best-effort, won't fail the build) ────────
RUN echo "=== Installed tool versions ===" && \
    (go version 2>/dev/null                  || echo "go:        FAILED") && \
    (oc version --client 2>/dev/null         || echo "oc:        FAILED") && \
    (kubectl version --client 2>/dev/null    || echo "kubectl:   FAILED") && \
    (tofu version 2>/dev/null                || echo "tofu:      FAILED") && \
    (helm version 2>/dev/null                || echo "helm:      FAILED") && \
    (yq --version 2>/dev/null                || echo "yq:        FAILED") && \
    (jq --version 2>/dev/null                || echo "jq:        FAILED") && \
    (just --version 2>/dev/null              || echo "just:      FAILED") && \
    (skopeo --version 2>/dev/null            || echo "skopeo:    FAILED") && \
    (qemu-img --version 2>/dev/null | head -1 || echo "qemu-img:  FAILED") && \
    (virsh --version 2>/dev/null             || echo "virsh:     FAILED") && \
    (sshpass -V 2>&1 | head -1               || echo "sshpass:   FAILED") && \
    (git --version 2>/dev/null               || echo "git:       FAILED") && \
    (dig -v 2>&1 | head -1                   || echo "dig:       FAILED") && \
    echo "=== Verification complete ==="

# ── Default command ──────────────────────────────────────────────────────────
CMD ["/bin/bash"]
