# Changelog

All notable changes to NanoClaw will be documented in this file.

## [2.0.0] - 2026-03-20

[BREAKING] Orchestrator rewritten in Go 1.26+.
- **feat:** Core host process is now a single Go binary for improved performance and lower memory footprint.
- **feat:** Built-in support for multiple messaging channels (Telegram, WhatsApp, Slack, Discord, Gmail).
- **feat:** Native credential proxy for secure API key handling.
- **feat:** Unified `setup` and `whatsapp-auth` binaries.
- **fix:** Enhanced IPC and task scheduling reliability.

## [1.2.0](https://github.com/qwibitai/nanoclaw/compare/v1.1.6...v1.2.0)
...
[BREAKING] WhatsApp removed from core, now a skill. Run `/add-whatsapp` to re-add (existing auth/groups preserved).
- **fix:** Prevent scheduled tasks from executing twice when container runtime exceeds poll interval (#138, #669)
