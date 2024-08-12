#!/bin/bash

# The number of resources to apply is passed as an argument
num_resources=$1

for ((i=0; i<num_resources; i++)); do
  ./hack/usage/delete shoot local-$i garden-local
done