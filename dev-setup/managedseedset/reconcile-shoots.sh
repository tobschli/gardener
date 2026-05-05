#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

export KUBECONFIG="$PWD/dev/envtest-kubeconfig.yaml"

repatch=${1:-false}

patch_shoots() {
  local ready=$1

  for name in $(kubectl -n garden get shoot -ojsonpath='{.items[*].metadata.name}'); do
    shoot="$(kubectl -n garden get shoot "$name" -oyaml)"
    generation="$(echo "$shoot" | yq '.metadata.generation')"

    if [[ "$ready" == "true" ]]; then
      kubectl -n garden label shoot "$name" shoot.gardener.cloud/status=healthy --overwrite
      kubectl -n garden patch --subresource=status shoot "$name" --patch-file /dev/stdin <<EOF
status:
  observedGeneration: $generation
  lastOperation:
    type: Reconcile
    state: Succeeded
    progress: 100
EOF
    else
      kubectl -n garden label shoot "$name" shoot.gardener.cloud/status=unhealthy --overwrite
      kubectl -n garden patch --subresource=status shoot "$name" --patch-file /dev/stdin <<EOF
status:
  observedGeneration: $generation
  lastOperation:
    type: Reconcile
    state: Processing
    progress: 0
EOF
    fi
  done
}

if [[ "$repatch" == "true" ]]; then
  patch_shoots false
fi
patch_shoots true
