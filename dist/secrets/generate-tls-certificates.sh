#!/bin/bash

# generate private key
openssl genrsa -out tls.key 2048

# generate tls certificate
openssl req -new -sha256 -key tls.key -config "$(pwd)/csr_details.txt" \
    | openssl x509 -req -extensions req_ext -extfile "$(pwd)/csr_details.txt" -CA ca.crt -CAkey ca.key -CAcreateserial -days 3650 -sha256 -out tls.crt

