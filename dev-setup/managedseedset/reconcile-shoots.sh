#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

export KUBECONFIG="$PWD/dev/envtest-kubeconfig.yaml"

for name in $(kubectl -n garden get shoot -ojsonpath='{.items[*].metadata.name}'); do
  shoot="$(kubectl -n garden get shoot "$name" -oyaml)"
  generation="$(echo "$shoot" | yq '.metadata.generation')"

  kubectl -n garden label shoot "$name" shoot.gardener.cloud/status=healthy --overwrite
  kubectl -n garden patch --subresource=status shoot "$name" --patch-file /dev/stdin <<EOF
status:
  observedGeneration: $generation
  lastOperation:
    type: Reconcile
    state: Succeeded
    progress: 100
EOF
done
