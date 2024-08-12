#!/bin/bash

echo "-=[ Installing cert-manager ]=-"
helm repo add jetstack https://charts.jetstack.io --force-update
helm install \
  cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --version v1.15.2 \
  --set crds.enabled=true


echo "-=[ Installing Jaeger operator ]=-"
kubectl create namespace observability
kubectl create -f https://github.com/jaegertracing/jaeger-operator/releases/download/v1.60.0/jaeger-operator.yaml -n observability

echo "-=[ Installing Jaeger ]=-"
k apply -f thesis/jaeger.yaml -n observability