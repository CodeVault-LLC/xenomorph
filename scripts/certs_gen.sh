#!/bin/bash
mkdir -p certs

# 1. Create CA
openssl req -new -x509 -days 365 -nodes -out certs/ca.crt -keyout certs/ca.key -subj "/CN=ZeroTrust-CA"

# 2. Create Gateway Certs (Server)
openssl req -new -nodes -out certs/server.csr -keyout certs/server.key -subj "/CN=localhost"
openssl x509 -req -in certs/server.csr -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial -out certs/server.crt -days 365 -extfile <(printf "subjectAltName=DNS:localhost,IP:127.0.0.1")

# 3. Create Client Certs (Agent)
# Note: The CN (Common Name) is used as the Agent ID
openssl req -new -nodes -out certs/client.csr -keyout certs/client.key -subj "/CN=agent-001-uuid"
openssl x509 -req -in certs/client.csr -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial -out certs/client.crt -days 365