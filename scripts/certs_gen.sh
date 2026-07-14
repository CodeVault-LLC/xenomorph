#!/bin/bash
set -euo pipefail

mkdir -p certs

# The client certificate gets unique identity material per invocation.
# - XENOMORPH_AGENT_HOSTNAME allows explicit hostname override.
# - XENOMORPH_AGENT_UUID allows explicit stable UUID override.
# If not provided, both values are generated from local runtime state.
CLIENT_HOSTNAME="${XENOMORPH_AGENT_HOSTNAME:-$(hostname -f 2>/dev/null || hostname)}"
AGENT_UUID="${XENOMORPH_AGENT_UUID:-$(cat /proc/sys/kernel/random/uuid)}"

# 1. Create CA
openssl req -new -x509 -days 365 -nodes -sha256 -newkey rsa:3072 -out certs/ca.crt -keyout certs/ca.key -subj "/CN=ZeroTrust-CA"

# 2. Create Gateway Certs (Server)
openssl req -new -nodes -sha256 -newkey rsa:3072 -out certs/server.csr -keyout certs/server.key -subj "/CN=localhost"
openssl x509 -req -sha256 -in certs/server.csr -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial -out certs/server.crt -days 365 -extfile <(printf "subjectAltName=DNS:localhost,IP:127.0.0.1")

# 3. Create Client Certs (Agent)
openssl req -new -nodes -sha256 -newkey rsa:3072 -out certs/client.csr -keyout certs/client.key -subj "/CN=${CLIENT_HOSTNAME}/OU=${AGENT_UUID}"
openssl x509 -req -sha256 -in certs/client.csr -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial -out certs/client.crt -days 365 -extfile <(printf "subjectAltName=DNS:%s,URI:urn:xenomorph:agent:%s" "${CLIENT_HOSTNAME}" "${AGENT_UUID}")

printf 'Generated client identity:\n'
printf '  hostname: %s\n' "${CLIENT_HOSTNAME}"
printf '  uuid: %s\n' "${AGENT_UUID}"
