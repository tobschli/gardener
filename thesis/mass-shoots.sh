#!/bin/bash
num_resources=$1
for ((i=0; i<num_resources; i++)); do 
    cat <<EOF | kubectl apply -f - 
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
metadata:
    name: local-$i
    namespace: garden-local 
spec:
    cloudProfileName: local 
    region: local 
    provider:
        type: local
EOF
done