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

patch_managedseeds() {
  local ready=$1
  local condition_status
  if [[ "$ready" == "true" ]]; then
    condition_status="True"
  else
    condition_status="False"
  fi

  for name in $(kubectl -n garden get managedseed -ojsonpath='{.items[*].metadata.name}'); do
    managedseed="$(kubectl -n garden get managedseed "$name" -oyaml)"
    generation="$(echo "$managedseed" | yq '.metadata.generation')"

    kubectl -n garden patch --subresource=status managedseed "$name" --patch-file /dev/stdin <<EOF
status:
  observedGeneration: $generation
  conditions:
  - type: SeedRegistered
    status: "$condition_status"
    reason: Reconcile
    message: Reconcile
EOF

    kubectl -n garden apply -f - <<EOF
apiVersion: core.gardener.cloud/v1beta1
kind: Seed
metadata:
  name: $name
  labels:
    name: my-managed-seed-set
spec:
  dns:
    internal:
      credentialsRef:
        apiVersion: v1
        kind: Secret
        name: internal-domain-internal-local-gardener-cloud
        namespace: garden
      domain: $name.internal.local.seed.local.gardener.cloud
      type: local
    provider:
      credentialsRef:
        apiVersion: v1
        kind: Secret
        name: default-domain-external-local-gardener-cloud
        namespace: garden
      type: local
  ingress:
    controller:
      kind: nginx
    domain: ingress.my-mss-0.garden.external.local.gardener.cloud
  networks:
    nodes: 10.0.0.0/16
    pods: 10.3.0.0/16
    services: 10.4.0.0/16
  provider:
    region: local
    type: local
  settings:
    verticalPodAutoscaler:
      enabled: false
EOF

    seed_generation="$(kubectl -n garden get seed "$name" -ojsonpath='{.metadata.generation}')"
    kubectl -n garden patch --subresource=status seed "$name" --patch-file /dev/stdin <<EOF
status:
  observedGeneration: $seed_generation
  conditions:
  - type: GardenletReady
    status: "$condition_status"
    reason: Reconcile
    message: Reconcile
  - type: BackupBucketsReady
    status: "$condition_status"
    reason: Reconcile
    message: Reconcile
  - type: SeedSystemComponentsHealthy
    status: "$condition_status"
    reason: Reconcile
    message: Reconcile
  - type: ExtensionsReady
    status: "$condition_status"
    reason: Reconcile
    message: Reconcile
EOF
  done
}

if [[ "$repatch" == "true" ]]; then
  patch_managedseeds false
fi
patch_managedseeds true
