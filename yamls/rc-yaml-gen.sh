#!/usr/bin/env bash

set -o errexit
set -o errtrace
set -o pipefail

PACKAGE='rc-yaml-gen.sh'
KUBE_CFG="$HOME/.kube/config"
RC_NAME=''
OUTPUT_FILE='rc.yaml'

while test $# -gt 0; do
  case "$1" in
    -h|--help)
      echo "$PACKAGE - generate RemoteCluster CRD yaml from kubectl config file"
      echo " "
      echo "$PACKAGE [options] RC_NAME"
      echo " "
      echo "options:"
      echo "-h, --help                show brief help"
      echo "--config=FILE             specify kubectl config file, default to $KUBE_CFG"
      echo "--output=FILE             specify output yaml file name, default to $OUTPUT_FILE"
      exit 0
      ;;
    --config*)
      KUBE_CFG=${1#"--config="}
      shift
      ;;
    --output*)
      OUTPUT_FILE=${1#"--output="}
      shift
      ;;
    *)
      break
      ;;
  esac
done

if [ ! -f "$KUBE_CFG" ]; then
    echo "config file not found: $KUBE_CFG"
    exit 1
fi

if [ $# -ne 1 ]; then
    echo "must specify one remote cluster name"
    exit 1
fi

RC_NAME=$1

ENDPOINT=$(grep 'server' "$KUBE_CFG" | awk '{print $2}')
CA_BUNDLE=$(grep 'certificate-authority-data' "$KUBE_CFG" | awk '{print $2}')
CLIENT_CERT=$(grep 'client-certificate-data' "$KUBE_CFG" | awk '{print $2}')
CLIENT_KEY=$(grep 'client-key-data' "$KUBE_CFG" | awk '{print $2}')

cat>"$OUTPUT_FILE"<<EOF
apiVersion: networking.alibaba.com/v1
kind: RemoteCluster
metadata:
  name: RC_NAME
spec:
  connConfig:
EOF
echo "    endpoint: $ENDPOINT" >> "$OUTPUT_FILE"
cat>>"$OUTPUT_FILE"<<EOF
    caBundle: CA_BUNDLE
    clientCert: CLIENT_CERT
    clientKey: CLIENT_KEY
EOF

sed -i "s/RC_NAME/$RC_NAME/gi" "$OUTPUT_FILE"
sed -i "s/CA_BUNDLE/$CA_BUNDLE/gi" "$OUTPUT_FILE"
sed -i "s/CLIENT_CERT/$CLIENT_CERT/gi" "$OUTPUT_FILE"
sed -i "s/CLIENT_KEY/$CLIENT_KEY/gi" "$OUTPUT_FILE"

echo "save RemoteCluster CRD to $OUTPUT_FILE!"
