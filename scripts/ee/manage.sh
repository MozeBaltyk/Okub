#!/usr/bin/env bash
set -euo pipefail

# EE (okub-ee) lifecycle: build/push the image, render the Helm chart and run
# it with `podman play kube`.

# Resolve config knobs from the environment (set by the justfile).
resolve_knobs() {
    EE_IMAGE="${EE_IMAGE:-localhost/okub-ee}"
    EE_VERSION="${EE_VERSION:-latest}"
    REGISTRY_URL="${REGISTRY_URL:-}"
    ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

    # Deploy knobs (defaults mirror helm/values.yaml).
    CHART="${CHART:-${ROOT}/helm}"
    RELEASE_NAME="${RELEASE_NAME:-okub-ee}"
    IMAGE_REPO="${IMAGE_REPO:-${EE_IMAGE}}"
    IMAGE_TAG="${IMAGE_TAG:-${EE_VERSION}}"
    IMAGE_PULL_POLICY="${IMAGE_PULL_POLICY:-IfNotPresent}"
}

# Build the okub-ee image from the repo's Containerfile.
build() {
    resolve_knobs
    sudo podman build \
        -t "${EE_IMAGE}:${EE_VERSION}" \
        -f "${ROOT}/Containerfile" \
        "${ROOT}"
}

# Tag the locally-built image for the registry and push it.
push() {
    resolve_knobs
    if [ -z "${REGISTRY_URL}" ]; then
        echo "error: REGISTRY_URL is empty — set it in .env or the environment" >&2
        return 1
    fi
    # Strip a leading registry prefix (e.g. localhost/) from the local name.
    remote="${REGISTRY_URL}/${EE_IMAGE#*/}:${EE_VERSION}"
    sudo podman tag "${EE_IMAGE}:${EE_VERSION}" "${remote}"
    sudo podman push "${remote}"
}

# Render the Helm chart and feed it to podman play kube (KUBEFILE|-).
kube_play() {
    resolve_knobs
    cd "${ROOT}"
    # workspace.hostPath must be an absolute filesystem path: Podman treats a
    # hostPath value with no leading slash as a named volume, not a bind mount.
    helm template "${RELEASE_NAME}" "${CHART}" \
        --set "image.repository=${IMAGE_REPO}" \
        --set "image.tag=${IMAGE_TAG}" \
        --set "image.pullPolicy=${IMAGE_PULL_POLICY}" \
        --set "workspace.hostPath=${ROOT}" \
        | sudo podman play kube "$@" -
}

deploy() {
    resolve_knobs
    # Build locally if the image is missing; --replace makes deploy idempotent.
    sudo podman image exists "${EE_IMAGE}:${EE_VERSION}" || build
    kube_play --replace
}

destroy() {
    resolve_knobs
    # --down tears down the pod; tolerate "not found" for idempotent teardown.
    kube_play --down || true
    sudo podman rmi "${IMAGE_REPO}:${IMAGE_TAG}" 2>/dev/null || true
}

redeploy() {
    destroy
    deploy
}

# Launch the EE container and drop into an interactive shell.
connect() {
    resolve_knobs
    # Already running inside the container (k8s pod) — open a shell directly.
    if [ -n "${KUBERNETES_SERVICE_HOST:-}" ] || [ -f "/run/secrets/kubernetes.io/serviceaccount/token" ]; then
        exec bash
    fi
    kubeconfig="${KUBECONFIG:-${HOME}/.kube/config}"
    kube_args=()
    if [ -f "${kubeconfig}" ]; then
        kube_args=(-v "${kubeconfig}:/root/.kube/config:ro,z" -e KUBECONFIG=/root/.kube/config)
    fi
    sudo podman run --rm -it \
        -v "${ROOT}:/workspace:z" \
        "${kube_args[@]}" \
        -e REGISTRY_URL="${REGISTRY_URL}" \
        -w /workspace \
        "${EE_IMAGE}:${EE_VERSION}"
}

case "${1:-}" in
    build)    build ;;
    push)     push ;;
    connect)  connect ;;
    deploy)   deploy ;;
    destroy)  destroy ;;
    redeploy) redeploy ;;
    *)
        echo "usage: $0 {build|push|connect|deploy|destroy|redeploy}" >&2
        exit 1
        ;;
esac
