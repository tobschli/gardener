#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

export KUBECONFIG="$PWD/dev/envtest-kubeconfig.yaml"

kubectl apply -k dev-setup/gardenconfig/overlays/operator
kubectl apply -k dev-setup/extensions/provider-local/components/controllerregistration
kubectl apply -f example/provider-local/managedseedset/managedseedset.yaml
