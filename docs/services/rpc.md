# RPC Layer (Port 9000)

> A request/response abstraction built on top of the MVCP wire format.
> Formalises `msg_id` correlation, pipelining, streaming, and timeouts.

## Overview

The RPC layer is a **software abstraction**, not a new wire format. It sits
between the service layer (Control, Execution) and the MVCP wire layer,
defining how `msg_id`, `IS_RESPONSE`, and `IS_STREAM_MORE` are used
together to implement remote procedure calls.

Services that use the RPC layer:
- **Control** (`0x00`–`0x0F`): PING/PONG, SHUTDOWN/SHUTDOWN_ACK
- **Execution** (`0x10`–`0x1F`): EXEC, EXEC_RESULT, streaming STDOUT/STDERR
- **Tools** (`0x30`–`0x3F`): TOOL_CALL/TOOL_RESULT, LIST_TOOLS/LIST_TOOLS_RESULT

Services that explicitly do **not** use the RPC layer:
- **File Transfer** (port 9004): dedicated data plane, chunked file push — host-initiated, sender-push model
- **Events** (port 9002): async notifications, guest→host
- **Heartbeat** (port 9003): periodic liveness, guest→host (now part of Status service)

## Layer Architecture

```
┌──────────────────────────────────────────────────────────┐
│ SERVICE LAYER                                             │
│   Control  │  Execution  │  Tools                        │
├──────────────────────────────────────────────────────────┤
│ RPC LAYER (this document)                                 │
│ Request/Response │ Pipelining │ Streaming │ Timeouts     │
├──────────────────────────────────────────────────────────┤
│ MVCP WIRE LAYER                                           │
│ type(1B) + flags(1B) + msg_id(4B) + body(M)             │
├──────────────────────────────────────────────────────────┤
│ TRANSPORT LAYER                                           │
│ length(4B BE) + payload(N)                               │
└──────────────────────────────────────────────────────────┘
```

The RPC layer is implemented in `mvcp/rpc/` as a Go package. It consumes
the MVCP wire layer (`mvcp/protocol/`) for frame I/O and exposes a
`Client` / `Server` API to the service layer.

## Port Assignment

| Port | Layer            | Direction     | Services                    |
|------|-----------------|---------------|-----------------------------|
| 9000 | MVCP + RPC      | Bidirectional | Control, Execution, Tools   |
| 9001 | VPP             | Bidirectional | Console                     |
| 9002 | MVCP (no RPC)   | Guest → Host  | Events (async)              |
| 9003 | MVCP (no RPC)   | Bidirectional | Status (heartbeat + query)  |
| 9004 | MVCP (no RPC)   | Host-init, bidir | File Transfer (data plane) |

## Message Contract

The RPC layer uses the MVCP header fields (`type`, `flags`, `msg_id`) with
well-defined semantics for each message role:

| Role             | `type`            | `flags`                              | `msg_id`                 |
|------------------|-------------------|--------------------------------------|--------------------------|
| **Request**      | service type      | `0x00`                               | non-zero, unique per conn |
| **Response**     | result type       | `IS_RESPONSE` (`0x01`)              | matches request          |
| **Stream chunk** | chunk type        | `IS_STREAM_MORE` (`0x02`)           | matches request          |
| **Stream end**   | result type       | `IS_RESPONSE` (`0x01`)             | matches request          |
| **Started**      | `0xFA`            | `IS_RESPONSE` (`0x01`)             | matches request          |
| **Error**        | `0xFE`            | `IS_RESPONSE` (`0x01`)             | matches request          |
| **One-way**      | event type        | `0x00`                               | `0`                      |

### Dispatch Acknowledgment (STARTED)

Long-running operations (EXEC, TOOL_CALL) may receive a `STARTED` (type
`0xFA`) response from the server after it accepts and begins processing
the request:

```
Host → EXEC(msg_id=1, "long_build.sh")           → Guest
Guest → STARTED(msg_id=1, IS_RESPONSE, stream)   → Host   ← "accepted, processing..."
Guest → EXEC_STDOUT(msg_id=1, MORE)              → Host   ← streaming output
Guest → EXEC_RESULT(msg_id=1, IS_RESPONSE)       → Host   ← "done, exit=0"
```

`STARTED` confirms the handler has **begun processing** the request
(process spawned, tool invoked, file opened), not merely dispatched it.
The `STARTED` body is a single `bool` (`EncodeStarted`) indicating
whether the response will be streamed (multiple chunks with
`IS_STREAM_MORE`) or buffered (single response frame).

If the server cannot start processing (no handler, resource exhausted),
it sends `ERROR` (`0xFE`) instead of `STARTED`.

Fast RPC calls (PING, GET_STATUS) typically omit `STARTED` — the
response arrives quickly enough that a separate dispatch ack adds
unnecessary overhead.

### Request

Initiates a remote procedure. The client allocates a unique `msg_id`
(monotonic, starting at 1 per connection) and sends it with the service
type. The body encodes the call parameters.

```
Host ── type=0x10 flags=0x00 msg_id=0x01 body=<EXEC params> ──→ Guest
```

### Response (unary)

Completes a request. The server echoes the request's `msg_id`, sets
`IS_RESPONSE`, and sends the result type with the response body.

```
Guest ── type=0x11 flags=0x01 msg_id=0x01 body=<EXEC_RESULT> ──→ Host
```

### Streaming Response

For operations that produce output incrementally (EXEC stdout/stderr,
file chunks), the server sends multiple frames sharing the same `msg_id`:

- **Chunks**: set `IS_STREAM_MORE` on every chunk except the last.
- **Final frame**: clears `IS_STREAM_MORE` and sets `IS_RESPONSE`.

```
Guest ── type=0x12 flags=0x02 msg_id=0x01 body=<stdout chunk 1> ──→ Host
Guest ── type=0x12 flags=0x02 msg_id=0x01 body=<stdout chunk 2> ──→ Host
Guest ── type=0x11 flags=0x01 msg_id=0x01 body=<EXEC_RESULT>   ──→ Host
```

Streaming requests (host→guest) follow the same pattern with roles
reversed.

### Error

When a request fails, the server responds with type `0xFE`, `IS_RESPONSE`,
and the original `msg_id`. The body encodes the error code and message
(see [04-error-codes.md](../04-error-codes.md)).

```
Guest ── type=0xFE flags=0x01 msg_id=0x01 body=<uint16 code>+<string msg> ──→ Host
```

## Pipelining

Multiple requests can be in-flight on a single connection. Responses are
correlated by `msg_id`, not by order.

```
Host ── EXEC(msg_id=1) ──────→ Guest
Host ── GET_STATUS(msg_id=2) ─→ Guest
Host ←── STATUS(msg_id=2) ──── Guest   (response to 2, arrives before 1)
Host ←── EXEC_RESULT(msg_id=1) Guest  (response to 1)
```

The client maintains a `map[uint32]*pendingCall` keyed by `msg_id`. Each
incoming frame is dispatched to the matching pending call based on
`msg_id` + `IS_RESPONSE`.

## Streaming and Multiplexing

A streaming operation does **not** block the connection:

- Requests and streams are multiplexed on one connection; the client
  dispatches every frame to the pending call matching its `msg_id`.
- Frames are self-delimiting, so streams interleave safely. Each frame
  must be written atomically (single buffer) when multiple handlers
  write concurrently.
- Ordering is guaranteed **within** a stream (same `msg_id`; vsock is
  ordered); there is no ordering guarantee across requests or streams.
  Stream end is signaled by the frame with `IS_STREAM_MORE` cleared.
- A slow stream consumer causes backpressure, never silent frame loss.

## Timeout Semantics

Timeouts are managed via Go's `context.Context` — no wire-level timeout
fields.

### Client side

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
resp, err := client.Call(ctx, EXEC, body)
// err is context.DeadlineExceeded if timeout fires
```

When the context is cancelled or the deadline expires:
1. The client removes the pending entry from its map.
2. The `Call` / `Stream` method returns with an error.
3. The underlying connection is **not** closed — other in-flight requests
   are unaffected.

### Server side

The handler receives the client's context as `ctx`:

```go
func execHandler(ctx context.Context, req *Request) error {
    select {
    case <-ctx.Done():
        return ctx.Err() // client cancelled or timed out
    case result := <-cmdDone:
        return req.Respond(EXEC_RESULT, result)
    }
}
```

Servers SHOULD check `ctx.Done()` during long-running operations to avoid
wasted work.

## Go API

### Module: `mvcp/rpc`

```go
import "github.com/Wolf258/mvcp/rpc"
```

### Client

```go
type Client struct { /* ... */ }

// NewClient wraps an existing MVCP connection for RPC.
func NewClient(conn *mvcp.Conn) *Client

// Call sends a request and blocks until a single response arrives.
// The response has IS_RESPONSE set and the matching msg_id.
// Returns an error if ctx is cancelled or the deadline expires.
func (c *Client) Call(ctx context.Context, msgType uint8, body []byte) (*Response, error)

// Stream sends a request and returns a channel of stream frames.
// The channel delivers every frame that shares the request's msg_id
// (chunks with IS_STREAM_MORE, then the final response with IS_RESPONSE).
// The channel is closed when the stream ends or ctx is cancelled.
func (c *Client) Stream(ctx context.Context, msgType uint8, body []byte) (<-chan StreamFrame, error)

// Close shuts down the client and its underlying connection.
func (c *Client) Close() error
```

```go
type Response struct {
    Type uint8  // result type (e.g. EXEC_RESULT)
    Body []byte // result payload
}

type StreamFrame struct {
    Type uint8  // chunk type (e.g. EXEC_STDOUT) or result type
    Body []byte // chunk or result payload
    More bool   // true if IS_STREAM_MORE was set
}
```

### Server

```go
type Server struct { /* ... */ }

// NewServer wraps an existing MVCP connection for RPC.
func NewServer(conn *mvcp.Conn) *Server

// Handle registers a handler for a message type.
func (s *Server) Handle(msgType uint8, h Handler)

// Serve reads frames from the connection and dispatches to handlers.
// Blocks until ctx is cancelled or the connection closes.
func (s *Server) Serve(ctx context.Context) error
```

```go
type Handler func(ctx context.Context, req *Request) error
```

### Request (server-side response writer)

```go
type Request struct {
    Type  uint8
    MsgID uint32
    Flags uint8
    Body  []byte
}

// Respond sends a single response frame with IS_RESPONSE.
func (r *Request) Respond(msgType uint8, body []byte) error

// Stream sends a chunk frame with IS_STREAM_MORE set.
// msg_id matches the request automatically.
func (r *Request) Stream(msgType uint8, body []byte) error

// StreamEnd sends the final frame: IS_RESPONSE, no IS_STREAM_MORE.
func (r *Request) StreamEnd(msgType uint8, body []byte) error

// Error sends an ERROR response (type=0xFE) with IS_RESPONSE.
func (r *Request) Error(code uint16, message string) error
```

## Request Lifecycle

### Unary (PING/PONG, GET_STATUS/STATUS)

```
Host (Client)                         Guest (Server)
    │                                       │
    │── PING(msg_id=1, flags=0) ──────────→│
    │                                       │ handler(ctx, req)
    │                                       │ req.Respond(PONG, nil)
    │←── PONG(msg_id=1, IS_RESPONSE) ──────│
    │                                       │
```

### Streaming (EXEC with stdout/stderr)

```
Host (Client)                         Guest (Server)
    │                                       │
    │── EXEC(msg_id=1, flags=0) ──────────→│
    │                                       │ handler(ctx, req)
    │                                       │ go run command
    │←── EXEC_STDOUT(msg_id=1, MORE) ──────│ req.Stream(EXEC_STDOUT, data)
    │←── EXEC_STDERR(msg_id=1, MORE) ──────│ req.Stream(EXEC_STDERR, data)
    │←── EXEC_STDOUT(msg_id=1, MORE) ──────│ req.Stream(EXEC_STDOUT, data)
    │←── EXEC_RESULT(msg_id=1, RESPONSE) ──│ req.StreamEnd(EXEC_RESULT, result)
    │                                       │
```

### Error

```
Host (Client)                         Guest (Server)
    │                                       │
    │── EXEC(msg_id=1) ───────────────────→│
    │                                       │ handler fails
    │←── ERROR(msg_id=1, IS_RESPONSE) ─────│ req.Error(TIMEOUT, "timed out")
    │                                       │
```

## Wire Examples

### Unary: GET_STATUS

**Request** (host → guest, 6 bytes on wire):
```
 length: 0x00_00_00_06   (6 = 6 header + 0 body)
   type: 0x05             (GET_STATUS)
  flags: 0x00
 msg_id: 0x00_00_00_01
```

**Response** (guest → host):
```
 length: 0x00_00_00_22   (6 + 28 payload)
   type: 0x06             (STATUS)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches)
   body:
    string "0.1.0"        → 0x0005 + "0.1.0"
    uint32 1              → 0x00_00_00_01 (pid)
    bool false            → 0x00 (shutting_down)
```

### Streaming: EXEC

**Request** (host → guest):
```
 length: 0x00_00_00_2A   (6 + 36 payload)
   type: 0x10             (EXEC)
  flags: 0x00
 msg_id: 0x00_00_00_02
   body:
    string "ls -la"       → 0x0006 + "ls -la"
    string "/home/user"   → 0x000B + "/home/user"
    map<string,string> {} → 0x0000
    uint32 30000          → 0x00_00_75_30
```

**Stream chunk** (guest → host):
```
 length: 0x00_00_00_1A   (6 + 20 payload)
   type: 0x12             (EXEC_STDOUT)
  flags: 0x02             (IS_STREAM_MORE)
 msg_id: 0x00_00_00_02    (matches)
   body:
    bytes [data]          → uint32 len + data
```

**Final response** (guest → host):
```
 length: 0x00_00_00_1A   (6 + 20 payload)
   type: 0x11             (EXEC_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_02    (matches)
   body:
    int32 0               → exit_code
    bytes ""              → stdout (empty, was streamed)
    bytes ""              → stderr (empty, was streamed)
    uint32 42             → duration_ms
```

### Error

**Response** (guest → host):
```
 length: 0x00_00_00_16   (6 + 16 payload)
   type: 0xFE             (ERROR)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_02    (matches request)
   body:
    uint16 0x0005         (EXEC_FAILED)
    string "command not found"
```

## Design Decisions

| Decision                                                    | Rationale                                                                                      |
|-------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| RPC is a Go abstraction, not a new wire format              | No protocol bloat. The MVCP wire layer already has `msg_id` + flags — we just define how to use them. |
| msg_id is per-connection, monotonic starting at 1           | 32-bit range (~4B messages) is effectively infinite. Per-connection avoids cross-connection complexity. |
| IS_RESPONSE + type=0xFE for errors, no separate error flag  | The type already indicates error; a dedicated flag would be redundant.                         |
| Streaming is multiplexed on one connection                  | Frames are self-delimiting and correlated by `msg_id`; concurrent requests and streams coexist, each stream preserving its own ordering. |
| Multiple connections remain supported                       | Independent `msg_id` counters and session isolation; not required for concurrency.                                                |
| Timeouts via context.Context, not wire-level fields         | Clients already carry deadlines via contexts. No need to duplicate in the protocol.            |
| No server-initiated requests on port 9000                   | Host is always the RPC caller. Guest notifications go through port 9002 (Events).              |
| Pipelining: responses may arrive out of order               | The client uses a `map[msg_id]` — order doesn't matter. Simplifies server implementation.      |

---

See also:
- [02-wire-format.md](../02-wire-format.md) for the MVCP header layout and flags.
- [05-concurrency.md](../05-concurrency.md) for the concurrency model (pipelining, streaming multiplexing, multiple connections).
- [04-error-codes.md](../04-error-codes.md) for the error code registry.
- [control.md](control.md) for the Control service messages.
- [execution.md](execution.md) for the Execution service messages.
- [events.md](events.md) for the Events service (port 9002, no RPC).
- [status.md](status.md) for the Status service (port 9003, no RPC).
