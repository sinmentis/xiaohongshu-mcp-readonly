# Copilot instructions

Follow `AGENTS.md`.

The public product interface is exactly six read-only MCP tools and matching
local HTTP routes. Never expose account mutations. Preserve loopback, Host,
Origin, JSON POST, cookie-permission, browser-hash, and access-gate safeguards.

Use English for public documentation, identifiers, commits, and new comments.
Prefer go-rod operations over large JavaScript injections. Run Go formatting,
vet, unit tests, race tests, and integration-test compilation for relevant
changes.
