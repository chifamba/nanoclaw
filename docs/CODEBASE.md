# NanoClaw Codebase Guide

A developer-focused walkthrough of how NanoClaw works. Start here if you want to understand, modify, or extend the code.

---

## Table of Contents

1. [The Big Picture](#the-big-picture)
2. [End-to-End Message Flow](#end-to-end-message-flow)
3. [Source Files](#source-files)
4. [Channel System](#channel-system)
5. [Container System](#container-system)
6. [IPC: Host ↔ Container Communication](#ipc-host--container-communication)
7. [Database (SQLite)](#database-sqlite)
8. [Scheduled Tasks](#scheduled-tasks)
9. [Security Model](#security-model)
10. [Skills](#skills)
11. [Tests](#tests)
12. [Directory Layout](#directory-layout)

---

## The Big Picture

NanoClaw is a **single Node.js process** that:

1. Receives messages from one or more messaging channels (WhatsApp, Telegram, Slack, Discord, Gmail).
2. Stores them in SQLite and polls for new messages every 2 seconds.
3. When a trigger is detected (e.g. `@Andy`), spawns a Docker container running the Claude Agent SDK.
4. Streams the agent's response back to the originating channel.

Every group (conversation) gets its own isolated container with its own filesystem mount and its own `CLAUDE.md` memory file. Containers communicate back to the host via a filesystem-based IPC directory — no sockets, no shared memory.

```
┌──────────────────────────────────────────────────────┐
│                 HOST (Node.js process)                │
│                                                      │
│  Channels ──► SQLite DB ──► Message Loop             │
│                                 │                    │
│                          GroupQueue                  │
│                                 │                    │
│                     spawn container                  │
└──────────────────────────────────────────────────────┘
                         │ stdin/stdout + IPC files
┌──────────────────────────────────────────────────────┐
│             CONTAINER (Linux, per group)             │
│   Claude Agent SDK (claude CLI) + Bash + Web tools   │
└──────────────────────────────────────────────────────┘
```

---

## End-to-End Message Flow

```
User sends "@Andy hello" in WhatsApp group
         │
         ▼
channel.onMessage()  →  storeMessage() in SQLite
         │
         ▼
startMessageLoop() polls getNewMessages() every 2 s
         │
         ├─ Is this a registered group?  No → ignore
         ├─ Is the sender allowed?        No → drop
         └─ Is the trigger present?       No → ignore (unless main group)
         │
         ▼
GroupQueue.enqueueMessageCheck(groupJid)
         │
         ├─ Container already active for this group?
         │   Yes → write message to IPC input/ dir (agent polls it)
         │   No  → call processGroupMessages(groupJid)
         │
         ▼  (no active container)
getMessagesSince(chatJid, lastAgentTimestamp)
formatMessages()  →  XML prompt context
         │
         ▼
runContainerAgent(prompt, sessionId, groupMetadata)
  ├─ build volume mount args (group folder, global, IPC dir, …)
  ├─ start credential proxy (once per process, port 3001)
  ├─ docker run nanoclaw-agent:latest
  └─ stream stdout → parse OUTPUT_START / OUTPUT_END markers
         │
         ▼
container runs claude CLI with Agent SDK
  ├─ reads /workspace/group/CLAUDE.md  (per-group memory)
  ├─ reads /workspace/global/CLAUDE.md (global memory)
  ├─ executes Bash, web, browser, IPC-MCP tools
  └─ writes messages to /workspace/ipc/messages/  (outbound)
         │
         ▼
Host IPC watcher (1 s poll) reads messages/ files
channel.sendMessage(jid, text)
         │
         ▼
User receives response in WhatsApp
```

---

## Source Files

### `src/index.ts` — Orchestrator

The main entry point. Owns all mutable state and wires every subsystem together.

**State:**
| Variable | Purpose |
|---|---|
| `lastTimestamp` | Cursor: last message timestamp polled from DB |
| `lastAgentTimestamp` | Per-group cursor: last message seen by an agent run |
| `sessions` | Per-group Claude Agent SDK session IDs (persistent, multi-turn) |
| `registeredGroups` | Map of JID → `RegisteredGroup` (loaded from DB) |
| `channels` | Active `Channel` instances |
| `queue` | `GroupQueue` — concurrency manager |

**Key functions:**

- `startMessageLoop()` — 2 s poll; filters, deduplicates, routes messages to `GroupQueue`.
- `processGroupMessages(groupJid)` — loads context, formats prompt, calls `runAgent()`.
- `runAgent()` — calls `runContainerAgent()`, handles session tracking, rolls back cursor on error.
- `registerGroup()` — validates path, writes to DB, creates `logs/` dir.
- `main()` — starts credential proxy, initialises DB, connects channels, starts all loops.

---

### `src/config.ts` — Configuration

Reads environment variables once at startup and exports typed constants.

| Export | Default | Purpose |
|---|---|---|
| `ASSISTANT_NAME` | `"Andy"` | Trigger word |
| `TRIGGER_PATTERN` | `/^@Andy\b/i` | Compiled regex |
| `POLL_INTERVAL` | 2000 ms | Message loop cadence |
| `IDLE_TIMEOUT` | 30 min | How long to keep idle container alive |
| `CONTAINER_IMAGE` | `"nanoclaw-agent:latest"` | Docker image |
| `CONTAINER_TIMEOUT` | 30 min | Hard kill timeout |
| `MAX_CONCURRENT_CONTAINERS` | 5 | Global concurrency cap |
| `CREDENTIAL_PROXY_PORT` | 3001 | Local proxy port |
| `GROUPS_DIR` | `groups/` | Group memory folders |
| `DATA_DIR` | `data/` | Sessions, IPC, snapshots |
| `STORE_DIR` | `store/` | SQLite file |
| `MOUNT_ALLOWLIST_PATH` | `~/.config/nanoclaw/mount-allowlist.json` | External security config |

---

### `src/types.ts` — Shared Types

Defines the core interfaces that every module uses:

- **`Channel`** — interface every channel plugin must implement (`connect`, `sendMessage`, `ownsJid`, etc.)
- **`NewMessage`** — a single inbound message row
- **`RegisteredGroup`** — a conversation registered with NanoClaw
- **`ScheduledTask`** — a recurring or one-off task
- **`ContainerConfig`** — per-group container overrides (extra mounts, timeout)
- **`MountAllowlist`** / **`AdditionalMount`** — security config for extra bind mounts

---

### `src/db.ts` — SQLite Interface

Uses `better-sqlite3` (synchronous). All queries are named exports; no ORM.

**Schema:**

| Table | Key Columns | Purpose |
|---|---|---|
| `chats` | `jid`, `name`, `channel`, `is_group` | Chat metadata (name, last_message_time) |
| `messages` | `id`, `chat_jid`, `content`, `timestamp`, `is_bot_message` | All messages |
| `scheduled_tasks` | `id`, `group_folder`, `schedule_type`, `schedule_value`, `next_run` | Recurring jobs |
| `task_run_logs` | `task_id`, `run_at`, `status`, `result` | Execution history |
| `router_state` | `key`, `value` | KV store for cursors / misc state |
| `sessions` | `group_folder`, `session_id` | Per-group agent sessions |
| `registered_groups` | `jid`, `folder`, `trigger_pattern`, `is_main` | Group registry |

**Migrations** run at startup — `runMigrations()` adds missing columns so schema changes are non-breaking.

---

### `src/channels/registry.ts` — Channel Registry

Channels are plugins that **self-register** when their module is imported:

```typescript
// Inside a channel file (e.g. src/channels/whatsapp/index.ts):
import { registerChannel } from '../registry.js';
registerChannel('whatsapp', whatsappFactory);
```

`src/channels/index.ts` is a barrel that imports every installed channel. `src/index.ts` imports this barrel at startup, triggering all registrations as a side-effect.

A factory receives `ChannelOpts` (callbacks for inbound messages and chat metadata) and returns a `Channel` instance, or `null` if credentials are missing.

---

### `src/router.ts` — Message Formatting & Routing

- **`formatMessages(messages, timezone)`** — converts `NewMessage[]` to XML:
  ```xml
  <context timezone="America/New_York" />
  <messages>
    <message sender="Alice" time="2024-03-14 10:30:45">Hey</message>
  </messages>
  ```
- **`stripInternalTags(text)`** — removes `<internal>…</internal>` blocks (agent reasoning, not for users).
- **`formatOutbound(text)`** — strips internal tags before sending to user.
- **`findChannel(channels, jid)`** — returns the channel that owns a given JID.

---

### `src/group-queue.ts` — Concurrency Manager

Enforces `MAX_CONCURRENT_CONTAINERS` globally and serialises access per group.

- `enqueueMessageCheck(groupJid)` — if container active: mark `pendingMessages`; if at limit: add to `waitingGroups`; otherwise: start processing.
- `sendMessage(groupJid, text)` — writes to IPC `input/` dir when container is alive (multi-turn handoff).
- `registerProcess(groupJid, proc, containerName, groupFolder)` — records the running child process so it can be killed / waited on.
- When a container finishes, the queue automatically starts the next waiting group.

---

### `src/container-runner.ts` — Container Spawner

Builds the `docker run` command and streams output.

**Volume mounts built by `buildVolumeMounts()`:**

| Mount | Group Type | Access |
|---|---|---|
| `groups/{name}/` → `/workspace/group` | all | read-write |
| `groups/global/` → `/workspace/global` | non-main | read-only |
| Project root → `/workspace/project` | main only | read-only |
| `.env` → `/dev/null` | main only | shadows credentials |
| `data/ipc/{folder}/` → `/workspace/ipc` | all | read-write |
| Extra mounts from `containerConfig` | all | validated by allowlist |

**Output parsing:** the container writes markers `OUTPUT_START_MARKER` / `OUTPUT_END_MARKER` around each response block. The host captures output between markers and sends it to the channel.

**Snapshots:** `writeGroupsSnapshot()` and `writeTasksSnapshot()` dump current state to `/workspace/ipc/{folder}/` so the agent can read registered groups and tasks without DB access.

---

### `src/container-runtime.ts` — Runtime Abstraction

All Docker-specific CLI calls live here; swapping to another runtime (e.g. Apple Container, Podman) means changing only this file.

- `CONTAINER_RUNTIME_BIN` — `"docker"`
- `CONTAINER_HOST_GATEWAY` — `"host.docker.internal"`
- `PROXY_BIND_HOST` — where the credential proxy binds:
  - macOS & WSL: `127.0.0.1` (Docker Desktop routes `host.docker.internal` → loopback)
  - Linux bare-metal: docker0 bridge IP (so only containers can reach the proxy), falls back to `0.0.0.0` with a warning
- `ensureContainerRuntimeRunning()` — checks `docker info`, exits with a clear error if Docker is not running.
- `cleanupOrphans()` — kills leftover `nanoclaw-*` containers from crashed runs.

---

### `src/credential-proxy.ts` — API Key Injector

Containers never receive the real Anthropic API key. Instead, they connect through a local HTTP proxy (`localhost:3001`) that intercepts every API request and injects credentials.

Two auth modes (auto-detected from `.env`):
- **API key** — injects `x-api-key` header on every request.
- **OAuth** — injects the real OAuth Bearer token only on the exchange request; subsequent requests carry a short-lived temp key.

The proxy starts once in `main()` and is shared across all containers.

---

### `src/ipc.ts` — IPC Watcher

Polls `data/ipc/{groupFolder}/` every 1 second. Containers write JSON files there; the host reads, processes, and deletes them.

**IPC file types:**

| Directory | Direction | Purpose |
|---|---|---|
| `messages/` | container → host | Send a message to a channel JID |
| `tasks/` | container → host | Create/update/cancel a scheduled task, register groups |
| `input/` | host → container | Deliver follow-up messages to an active agent |

**Authorization rules:**
- Non-main groups can only send to their own JID.
- Only the main group can register new groups, cancel other groups' tasks, or call `refresh_groups`.
- Group identity is determined by the IPC directory path, which the host controls.

---

### `src/task-scheduler.ts` — Scheduled Tasks

Runs a 60-second loop. On each tick, calls `getDueTasks()` and invokes `runContainerAgent()` for each due task — same container path as regular messages, but with a synthesised prompt.

Schedule types:
- **`cron`** — standard cron expression (evaluated with the configured `TZ` timezone).
- **`interval`** — number of seconds between runs.
- **`once`** — run at a specific ISO timestamp, then mark `completed`.

---

### `src/mount-security.ts` — Mount Allowlist Validator

Validates `containerConfig.additionalMounts` against `~/.config/nanoclaw/mount-allowlist.json`, which lives **outside** the project root so container agents cannot modify it.

Rules enforced:
1. Requested path must be under an `allowedRoots` entry.
2. Path must not match any `blockedPatterns` (e.g. `.ssh`, `.aws`, `.env`).
3. If `nonMainReadOnly: true`, non-main groups cannot get read-write mounts.

Default blocked patterns include: `.ssh`, `.gnupg`, `.aws`, `.azure`, `.gcloud`, `.kube`, `.docker`, `credentials`, `.env`, `.netrc`, and common private key filenames.

---

### `src/sender-allowlist.ts` — Sender Filter

Optional per-group allowlist stored at `~/.config/nanoclaw/sender-allowlist.json`. Two modes:
- **`allow`** — only listed senders can trigger the agent.
- **`drop`** — messages from listed senders are silently dropped.

---

### `src/group-folder.ts` — Path Validation

`resolveGroupFolderPath(folder)` validates and resolves group folder names to absolute paths under `GROUPS_DIR`. Rejects path-traversal attempts (names containing `..`, `/`, or null bytes).

---

### `src/logger.ts` — Structured Logging

Re-exports a `pino` logger with pretty-print enabled. Log level controlled by `LOG_LEVEL` env var (default: `info`).

---

## Channel System

Channels are installed as **skills** (git branches) and self-register at startup.

### Adding a New Channel

1. Create `src/channels/{name}/index.ts` that exports a factory:
   ```typescript
   import { registerChannel } from '../registry.js';
   registerChannel('mychannel', (opts) => {
     // Return a Channel or null if credentials are missing
   });
   ```
2. Add an import to `src/channels/index.ts`:
   ```typescript
   import './mychannel/index.js';
   ```
3. Add credentials to `.env`.

### `Channel` Interface

```typescript
interface Channel {
  name: string;
  connect(): Promise<void>;
  sendMessage(jid: string, text: string): Promise<void>;
  isConnected(): boolean;
  ownsJid(jid: string): boolean;
  disconnect(): Promise<void>;
  setTyping?(jid: string, isTyping: boolean): Promise<void>;  // optional
  syncGroups?(force: boolean): Promise<void>;                 // optional
}
```

The `jid` (JID = "Jabber ID") is a unique string that identifies a conversation. Format varies by channel (e.g. `120363000000000000@g.us` for WhatsApp groups, `telegram:-1001234567890` for Telegram groups).

---

## Container System

### Agent Container (`container/`)

The Docker image (`container/Dockerfile`) is a slim Linux image with:
- Node.js + the Claude Agent SDK CLI (`claude`)
- `container/agent-runner.js` — the process that runs inside the container
- MCP servers: filesystem, IPC (for sending messages back to host), optional extras

The container entry point reads a JSON prompt from stdin, calls `query()` from the Agent SDK, and streams results back via stdout with markers.

### Lifecycle

1. **Start** — `docker run` with volume mounts, env vars (proxy URL, session ID), resource limits.
2. **Idle** — after responding, the container stays alive for `IDLE_TIMEOUT` (default 30 min) polling `ipc/input/` for follow-up messages. This makes multi-turn conversation fast (no container restart overhead).
3. **Stop** — idle timeout reached, new task queued, or explicit shutdown.

### Building the Image

```bash
./container/build.sh
```

> **Cache note:** `--no-cache` alone does not invalidate `COPY` steps when using BuildKit. Run `docker builder prune` first for a truly clean rebuild.

---

## IPC: Host ↔ Container Communication

```
HOST                          CONTAINER
────                          ─────────
data/ipc/{folder}/
  messages/  ◄─────────────  agent writes {type: "message", chatJid, text}
  tasks/     ◄─────────────  agent writes {type: "schedule_task", ...}
  input/     ─────────────►  host writes follow-up message files
  groups.json ────────────►  host writes registered groups snapshot
  tasks.json  ────────────►  host writes scheduled tasks snapshot
```

Files are processed atomically (read → process → delete). Failed files move to `ipc/errors/` for inspection.

---

## Database (SQLite)

File: `store/nanoclaw.db`

All access goes through `src/db.ts`. No direct SQL elsewhere. `better-sqlite3` is synchronous, which avoids async complexity in the hot message loop.

Key design decisions:
- `router_state` is a KV table (not separate columns) so new state can be added without migrations.
- `sessions` persist across restarts so multi-turn conversations survive service restarts.
- `is_bot_message` flag prevents the agent's own replies from re-triggering itself.

---

## Scheduled Tasks

Tasks are created by agents via IPC (`tasks/` files with `type: "schedule_task"`). Users can also ask the agent to schedule things in natural language; the agent figures out the cron or interval syntax.

```json
{
  "type": "schedule_task",
  "chatJid": "...",
  "prompt": "Summarise overnight news",
  "schedule_type": "cron",
  "schedule_value": "0 8 * * 1-5",
  "context_mode": "isolated"
}
```

`context_mode`:
- `"group"` — agent has access to recent group messages for context.
- `"isolated"` — agent runs with no prior context (good for autonomous jobs).

---

## Security Model

### Container Isolation

- Each agent runs in a **separate Linux container** (Docker or Apple Container), not just a sandboxed process.
- Filesystem access is limited to explicitly mounted directories.
- The `.env` file is shadowed with `/dev/null` inside main-group containers (credentials are never visible to the agent).
- Network access inside containers is restricted; Anthropic API calls go through the credential proxy.

### Credential Proxy

Containers receive a placeholder API key. All Anthropic API calls are intercepted by the local proxy at `CREDENTIAL_PROXY_PORT` (3001), which injects the real credentials before forwarding. Containers never see the real API key or OAuth token.

### Mount Allowlist

Additional mounts requested via `containerConfig.additionalMounts` are validated against `~/.config/nanoclaw/mount-allowlist.json`. This file lives **outside the project root** so a compromised agent cannot modify it. Default blocked patterns cover all common credential locations.

### IPC Authorization

The host verifies the identity of IPC requests by the directory path of the file (which it controls). Non-main groups:
- Can only send messages to their own JID.
- Cannot register new groups.
- Cannot manage other groups' tasks.

### Sender Allowlist

Optional per-group allowlist (`~/.config/nanoclaw/sender-allowlist.json`) filters which senders can trigger the agent. Prevents unwanted users from consuming API credits.

---

## Skills

Skills are Claude Code slash commands (`.claude/skills/`) that transform the codebase. They are applied as git merges rather than executed scripts.

| Skill | What it does |
|---|---|
| `/setup` | Install dependencies, authenticate channels, configure services |
| `/add-whatsapp` | Merge the WhatsApp channel branch |
| `/add-telegram` | Merge the Telegram channel branch |
| `/add-slack` | Merge the Slack channel branch |
| `/add-discord` | Merge the Discord channel branch |
| `/add-gmail` | Merge the Gmail channel branch |
| `/customize` | Interactive: add channels, integrations, change behaviour |
| `/debug` | Diagnose container issues, inspect logs |
| `/update-nanoclaw` | Pull upstream core updates into a customised fork |

See `docs/nanoclaw-architecture-final.md` for the full skills architecture.

---

## Tests

Run with:

```bash
npm test           # unit tests
npm run test:skills  # skills tests (slower, requires git)
```

Tests use [Vitest](https://vitest.dev/). Each test file sits alongside the module it tests (`src/db.test.ts`, `src/ipc-auth.test.ts`, etc.).

Key test files:

| File | What it covers |
|---|---|
| `src/db.test.ts` | SQLite operations, migrations, task CRUD |
| `src/ipc-auth.test.ts` | IPC authorization rules (main vs non-main groups) |
| `src/group-queue.test.ts` | Concurrency limits, retry logic, idle waiting |
| `src/container-runner.test.ts` | Volume mount building, path sanitisation |
| `src/container-runtime.test.ts` | Proxy bind host detection across OS environments |
| `src/sender-allowlist.test.ts` | Sender filter allow/drop modes |
| `src/group-folder.test.ts` | Group folder path traversal prevention |
| `src/task-scheduler.test.ts` | Cron/interval/once scheduling logic |
| `src/formatting.test.ts` | XML message formatting, internal tag stripping |
| `src/routing.test.ts` | Channel JID ownership, message routing |
| `src/timezone.test.ts` | Timezone-aware timestamp formatting |

---

## Directory Layout

```
nanoclaw/
├── src/                    # TypeScript source
│   ├── index.ts            # Orchestrator (main entry point)
│   ├── config.ts           # Environment / constants
│   ├── types.ts            # Shared interfaces
│   ├── db.ts               # SQLite operations
│   ├── router.ts           # Message formatting + channel routing
│   ├── group-queue.ts      # Per-group concurrency manager
│   ├── container-runner.ts # Docker run + output streaming
│   ├── container-runtime.ts# Runtime abstraction (Docker CLI)
│   ├── credential-proxy.ts # API key injection proxy
│   ├── ipc.ts              # IPC file watcher
│   ├── task-scheduler.ts   # Scheduled task runner
│   ├── mount-security.ts   # Mount allowlist validation
│   ├── sender-allowlist.ts # Sender filter
│   ├── group-folder.ts     # Group folder path validation
│   ├── logger.ts           # Pino logger
│   └── channels/
│       ├── registry.ts     # Channel registration system
│       └── index.ts        # Barrel: imports all installed channels
│
├── container/              # Agent container image
│   ├── Dockerfile
│   ├── agent-runner.js     # Runs inside the container
│   ├── build.sh            # Build script
│   └── skills/             # Tools available to agents (e.g. browser)
│
├── groups/                 # Per-group memory (git-ignored data)
│   ├── global/CLAUDE.md    # Shared memory across all groups
│   └── {name}/
│       ├── CLAUDE.md       # Per-group memory file
│       └── logs/           # Agent run logs
│
├── data/                   # Runtime data (git-ignored)
│   └── ipc/{group}/        # IPC directories (messages, tasks, input)
│
├── store/                  # SQLite database (git-ignored)
│   └── nanoclaw.db
│
├── docs/                   # Documentation
│   ├── CODEBASE.md         # This file
│   ├── SPEC.md             # Full specification
│   ├── REQUIREMENTS.md     # Design decisions
│   ├── SECURITY.md         # Security considerations
│   └── SDK_DEEP_DIVE.md    # Claude Agent SDK internals
│
├── .claude/skills/         # Claude Code skill definitions
├── .env.example            # Environment variable template
├── CLAUDE.md               # Quick context for Claude Code
└── README.md               # User-facing overview
```
