#!/bin/bash

# Based on https://kwok.sigs.k8s.io/docs/user/kwok-in-cluster/
# This is the alternative to using go binary

# Temporary directory
KWOK_WORK_DIR=$(mktemp -d)
# KWOK repository
KWOK_REPO=kubernetes-sigs/kwok
# Get latest
KWOK_LATEST_RELEASE=$(curl "https://api.github.com/repos/${KWOK_REPO}/releases/latest" | jq -r '.tag_name')

cat <<EOF > "${KWOK_WORK_DIR}/kustomization.yaml"
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
images:
- name: registry.k8s.io/kwok/kwok
  newTag: "${KWOK_LATEST_RELEASE}"
resources:
- "https://github.com/${KWOK_REPO}/kustomize/kwok?ref=${KWOK_LATEST_RELEASE}"
EOF

kubectl kustomize "${KWOK_WORK_DIR}" > "${KWOK_WORK_DIR}/kwok.yaml"

kubectl apply -f "${KWOK_WORK_DIR}/kwok.yaml"