#!/usr/bin/env bash
set -euo pipefail

# Verify the okub-ee CLI tools are present. Runs inside the image via
# `just check-tools`.

tools=(go oc kubectl tofu helm yq just jq skopeo qemu-img virsh dig sshpass git)

missing=()
for t in "${tools[@]}"; do
  if command -v "${t}" >/dev/null 2>&1; then
    printf '  ok    %s\n' "${t}"
  else
    printf '  MISS  %s\n' "${t}" >&2
    missing+=("${t}")
  fi
done

if [ "${#missing[@]}" -ne 0 ]; then
  echo "error: ${#missing[@]} tool(s) missing: ${missing[*]}" >&2
  exit 1
fi

echo "All ${#tools[@]} tools present."
