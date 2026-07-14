# Xenomorph AI Coding Instructions

Read and follow `AGENTS.md`, `.docs/code-quality.md`, and `.docs/code-review.md` before editing. The gateway is the sole trust boundary. The client is an untrusted emitter: its identity claims, clock, telemetry, filesystem observations, terminal results, screen content, and operation results are not trust evidence. Browser input is operator-authored and is not authenticated identity or authorization evidence unless the gateway establishes that property.

Use the root Makefile. Validate the touched component immediately after the first substantive edit and run its complete CI gate before handoff. Preserve unrelated changes, do not invent absent features, and do not claim a release or compliance state beyond `.docs/project-status.md`.
