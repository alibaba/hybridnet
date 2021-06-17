#!/bin/bash

# generate root CA
openssl req -nodes -new -x509 -days 3650 -keyout ca.key -out ca.crt -subj "/CN=rama"

# encode root CA
ca_bundle=$(openssl base64 -A < ca.crt)
echo "${ca_bundle}" > ca.bundle