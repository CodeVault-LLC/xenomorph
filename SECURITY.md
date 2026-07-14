# Security Policy

## Supported Status

Xenomorph has not published a supported production release. The current `rewrite` branch is a development integration branch and must not be deployed as a public or Internet-facing service. See `.docs/project-status.md` for the current release decision and known security boundaries.

## Reporting a Vulnerability

Report suspected vulnerabilities privately through the repository's GitHub Security Advisory interface. Do not open a public issue containing exploit details, credentials, private keys, personal data, terminal output, screen captures, or filesystem content.

Include the affected commit or version, component, prerequisites, observed impact, and a minimal reproduction that does not target systems without explicit authorization. The maintainers will acknowledge the report, establish severity and scope, coordinate remediation and credential response, and publish disclosure information when affected users can act safely.

## Security Model

The gateway is the sole trust boundary. Client-authored telemetry and results are untrusted. Browser requests are operator-authored and are not authenticated identity or authorization evidence unless the gateway establishes those properties. No report or workaround may weaken mutual TLS, command verification, certificate validation, gateway mediation, or input/resource bounds.
