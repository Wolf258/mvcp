# Tools Service (Port 9000)

> Message type range `0x30`–`0x3F`. Carried on port 9000 via the RPC layer, alongside
> Control and Execution services. All structured commands are multiplexed on the
> control plane (port 9000), while file transfer uses the dedicated data plane (port 9004).

The tools service provides LLM-facing guest operations: filesystem I/O,
search, and shell execution. It uses a generic dispatch model — `TOOL_CALL`
carries a `tool_name` string and opaque `params` bytes — so custom tools
can be registered without protocol changes.

See [rpc.md](rpc.md) for the request/response contract (pipelining,
timeouts, error handling). All tool calls are **unary** — no streaming.

## Message Types

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x30` | `TOOL_CALL` | H→G | `string tool_name`, `bytes params` |
| `0x31` | `TOOL_RESULT` | G→H | `bytes result`, `bool ok`, `string error_msg` |
| `0x32` | `LIST_TOOLS` | H→G | *(none)* |
| `0x33` | `LIST_TOOLS_RESULT` | G→H | `uint16 count` + N × `ToolDescriptor` |
| `0x34`–`0x3F` | *(reserved)* | — | — |

### ToolDescriptor (per-tool entry in LIST_TOOLS_RESULT)

| Field | Encoding | Description |
|-------|----------|-------------|
| `name` | `string` | Registered tool identifier (e.g. `"read_file"`) |
| `description` | `string` | Human-readable tool summary |
| `version` | `string` | Semver (`"1.0.0"`). Enables tool evolution without breaking callers. |
| `capabilities` | `[]string` | High-level tags (`"fs:read"`, `"exec:shell"`, `"search:regex"`). The LLM uses these to filter/select tools by category. |
| `permissions` | `[]string` | Granular permission labels (`"fs:read"`, `"fs:write"`, `"exec"`). The host decides which tools to expose based on these. |
| `schema` | `bytes` | JSON Schema describing input parameters. The LLM reads this to construct valid `TOOL_CALL` bodies without out-of-band contracts. |

## TOOL_CALL (`0x30`)

Sent by the host as an RPC request (`flags=0x00`, non-zero `msg_id`).

| Field | Encoding | Description |
|-------|----------|-------------|
| `tool_name` | `string` | Registered tool identifier (e.g. `"read_file"`, `"bash"`) |
| `params` | `bytes` | Tool-specific parameters, encoded by the tool contract |

## TOOL_RESULT (`0x31`)

Sent by the guest as an RPC response (`IS_RESPONSE`, matching `msg_id`).

| Field | Encoding | Description |
|-------|----------|-------------|
| `result` | `bytes` | Tool-specific result data, encoded by the tool contract |
| `ok` | `bool` | `true` if the tool executed successfully |
| `error_msg` | `string` | Human-readable error detail when `ok=false`; empty otherwise |

When a tool is not registered, the guest responds with an MVCP `ERROR`
(type `0xFE`) carrying `UNKNOWN_TOOL` (`0x0009`). When a registered tool
fails internally, use `TOOL_RESULT(ok=false, error_msg=...)` — do **not**
use the error envelope for recoverable tool failures.

## LIST_TOOLS / LIST_TOOLS_RESULT

Query the guest for all registered tools. Returns a `ToolDescriptor` per
tool, giving the LLM everything it needs to construct valid calls at
runtime: parameter schema, version info, capabilities, and permissions.

**LIST_TOOLS_RESULT** payload — one `ToolDescriptor` per registered tool:

| Field | Encoding | Description |
|-------|----------|-------------|
| `count` | `uint16` | Number of registered tools |
| `entries` | N × `ToolDescriptor` | See [ToolDescriptor](#tooldescriptor-per-tool-entry-in-list_tools_result) above |

## Built-in Tools

The guest ships with a set of core tools. Each tool defines its own
binary layout for `params` and `result` using MVCP primitives.
All parameters are positional (no key names on the wire).

### `read_file`

Read a chunk of a file. For large files, make multiple calls with
incrementing `offset`.

**Params:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `path` | `string` | Absolute or relative file path |
| `offset` | `uint64` | Byte offset to start reading from |
| `max_bytes` | `uint32` | Maximum bytes to return in this chunk |

**Result:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `data` | `bytes` | File data starting at `offset`, up to `max_bytes` |
| `eof` | `bool` | `true` if the read reached end of file |

### `write_file`

Write data to a file at a given offset. Supports truncate semantics.

**Params:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `path` | `string` | File path |
| `data` | `bytes` | Data to write |
| `offset` | `uint64` | Write position (0 = beginning) |
| `truncate` | `bool` | If `true`, truncate file to `offset + len(data)` after write |

**Result:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `written` | `uint64` | Total bytes written |

### `edit_file`

Replace an exact string match in a file. Atomic: if `old_str` is found
multiple times, all occurrences are replaced. If not found, `replaced=false`.

**Params:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `path` | `string` | File path |
| `old_str` | `bytes` | Exact substring to replace |
| `new_str` | `bytes` | Replacement string |

**Result:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `replaced` | `bool` | `true` if at least one replacement was made |
| `count` | `uint32` | Number of occurrences replaced |

### `glob`

List files matching a glob pattern.

**Params:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `pattern` | `string` | Glob pattern (e.g. `"**/*.go"`, `"src/**/*.ts"`) |
| `path` | `string` | Base directory for the search |

**Result:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `count` | `uint16` | Number of matches |
| `matches` | N × `string` | Matching file paths |

### `grep`

Search for a regex pattern in files.

**Params:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `pattern` | `string` | Regex pattern |
| `path` | `string` | Base directory or specific file to search |

**Result:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `count` | `uint16` | Number of matches |
| `matches` | N × (`string file`, `uint32 line`, `bytes content`) | Matching lines |

### `bash`

Execute a shell command inside the guest. Internally forwards to the
Execution service (port 9000 EXEC) — infrastructure continues to use port
9000 directly; this tool is a convenience facade for the LLM.

**Params:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `command` | `string` | Shell command (passed to `/bin/sh -c`) |
| `cwd` | `string` | Working directory |
| `env` | `map[string]string` | Environment variables (merged with guest defaults) |
| `timeout_ms` | `uint32` | Maximum execution time in ms. 0 = no timeout. |

**Result:**

| Field | Encoding | Description |
|-------|----------|-------------|
| `exit_code` | `int32` | Process exit code. -1 if killed by signal. |
| `stdout` | `bytes` | Captured standard output |
| `stderr` | `bytes` | Captured standard error |

## RPC Contract

All tool messages use the RPC layer on port 9000. The host is always the
caller; the guest is the responder. Pipelining (multiple in-flight
requests per connection) is supported via `msg_id` correlation. Since all
tool calls are unary, head-of-line blocking never applies.

```
Host ── type=0x30 flags=0x00 msg_id=0x01 body=<tool_name + params> ──→ Guest
Guest ── type=0x31 flags=0x01 msg_id=0x01 body=<result>            ──→ Host

Host ── type=0x30 flags=0x00 msg_id=0x02 body=<unknown_tool>      ──→ Guest
Guest ── type=0xFE flags=0x01 msg_id=0x02 body=<0x0009 "unknown">  ──→ Host

Host ── type=0x32 flags=0x00 msg_id=0x03 body=<none>               ──→ Guest
Guest ── type=0x33 flags=0x01 msg_id=0x03 body=<count + entries>   ──→ Host
```

## Custom Tools

The generic dispatch model allows registering additional tools in the
guest without modifying the protocol. A custom tool must:

1. Register its `tool_name` and `description` in the guest's tool registry.
2. Define a binary layout for `params` and `result` using MVCP primitives
   (`string`, `uint32`, `uint64`, `int32`, `bytes`, `bool`, `map`).
3. Be listed in `LIST_TOOLS_RESULT`.

The host and guest must agree on the tool contract out-of-band
(documentation). The protocol does **not** enforce parameter validation at
the wire layer — each tool handler validates its own `params` buffer.

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Generic `TOOL_CALL` dispatch (not fixed type per tool) | Custom tools don't need protocol changes. Binary overhead of `tool_name` string is negligible vs other MVCP string fields. |
| Unary only, no streaming | Tools produce bounded results. Large-file I/O uses offset/chunk without streaming; large command output is captured in `bash` result. Keeps the RPC contract simple. |
| `TOOL_RESULT(ok=false)` for tool failures, not `ERROR(0xFE)` | `0xFE` is for protocol-level errors (bad payload, unknown type). Tool-level failures (file not found, grep no matches) are normal responses. |
| `LIST_TOOLS` returns full `ToolDescriptor` (incl. schema, caps, perms) | The LLM can discover parameter contracts at runtime without out-of-band documentation. Overhead is negligible (~2 KB for 6 built-in tools). |
| `bytes params` / `bytes result` (opaque) | Each tool defines its own struct. The protocol layer does not decode tool payloads — that's the tool handler's job. |

---

See also:
- [rpc.md](rpc.md) for the RPC layer contract (pipelining, timeouts, error handling).
- [execution.md](execution.md) for the Execution service used internally by the `bash` tool.
- [file-transfer.md](file-transfer.md) for large-file streaming (port 9004).
- [04-error-codes.md](../04-error-codes.md) for the error code registry.
- [examples/tool-calls.md](../examples/tool-calls.md) for wire-level examples.

## Provider Model (design notion)

> **Not implemented yet.** This section sketches how tool execution may
> evolve beyond the current embedded-Go model. The exact interface and
> directory structure are likely to change. It exists here to guide
> future work and keep the `ToolDescriptor` wire format forward-compatible.

The tools service layer is split into three independent concerns:

```
MVCP (wire)
  │  Knows nothing about tools. Just transports TOOL_CALL / TOOL_RESULT.
  ▼
Tool Registry (vhandler)
  │  LIST_TOOLS → ToolDescriptor array. Describes tools, does NOT execute.
  ▼
Tool Runtime (future)
  │  Dispatches Execute(name, params) to the right provider.
  ├─ Builtin  (Go func, current model)
  ├─ Script   (Node/TS, single process per provider)
  └─ External (binary/daemon, stdin/stdout JSON)
```

### Tool Provider (conceptual interface)

```go
type Provider interface {
    Descriptor() Descriptor
    Tools() []string
    Execute(ctx context.Context, tool string, params []byte) (Result, error)
}
```

### Runtimes (future possibilities)

| Runtime | How | Pros | Cons |
|---------|-----|------|------|
| **Builtin** | Go func in vhandler | Fast, zero overhead | Requires recompilation |
| **Script** | Node.js process managed by vhandler, stdin/stdout JSON | Edit `.ts` to change behavior, no rebuild | More memory, one process per provider |
| **External** | `execve` a binary found at `/usr/lib/shifty/tools/<name>/tool` | Any language (Python, Rust, etc.) | Cold-start latency, isolation concerns |

### Discovery (auto-registration)

vhandler scans `/usr/lib/shifty/tools/` at boot:

```
/usr/lib/shifty/tools/
  git/
    manifest.yaml    # provider: git, runtime: node, entry: index.ts
    index.ts
```

Each subdirectory contributes one `Provider`. The registry reads manifests
and populates the `LIST_TOOLS_RESULT` from all providers automatically.
No manual `Register()` calls — like systemd unit discovery.

### Security boundary

The tool **never** creates sockets or chooses file paths:
- vhandler creates any required sockets in `/run/shifty/tools/` with
  `0600` permissions.
- The tool process receives only the FD it needs, never the filesystem
  path.
- All `Permissions` fields in `ToolDescriptor` are informational — the
  host (shifty-core) is the enforcement point.
