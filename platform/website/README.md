# Xenomorph Administrative Website

This React and TypeScript application presents gateway state and collects operator intent for agent, terminal, screen, log, and file-workspace workflows. It does not authenticate agents, sign commands, authorize operations, or establish the truth of client-authored data. Those controls belong at the gateway.

## Development Contract

Bun is the only supported package manager and `bun.lock` is the dependency contract.

```bash
bun install --frozen-lockfile
bun run dev
```

Run the complete website gate from the repository root:

```bash
make ci-web
```

The gate checks formatting, ESLint, TypeScript, and a Vite production build. Follow the TypeScript and React rules in `AGENTS.md` and `.docs/code-review.md`.

## Security Boundary

All gateway responses and route parameters are untrusted at the rendering boundary unless the field is explicitly documented as gateway-authored. React text escaping must remain intact. Browser controls are usability controls only; hiding or disabling a control is never authorization.
