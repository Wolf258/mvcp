# App Channel (port 9005)

Capability-scoped, capability `AppChannel` (0x06, rev 1). One persistent
MVCP connection per VM; byte streams multiplexed by `stream_id`, credit
flow control, half-close. Core and vhandler terminate the protocol and
bridge to endpoints; app payloads are opaque to core and never persisted.

## Wire

All `APP_*` frames carry `flags=0` and `msg_id=0`; correlation is
`stream_id`. `stream_id` is `uint32`: bit 31 = initiator (0 = host/core,
1 = guest/vhandler), bits 0-30 = per-connection, per-direction counter.
0 is reserved; IDs are never reused within a connection.

| Type | Name | Direction | Body |
|------|------|-----------|------|
| `0x50` | `APP_OPEN` | both | `stream_id u32`, `service string` (≤64B, `^[a-z][a-z0-9-]{0,30}$`), `meta bytes` (≤1 KiB) |
| `0x51` | `APP_ACCEPT` | both | `stream_id u32`, `meta bytes` (≤1 KiB), `grant u32` |
| `0x52` | `APP_REJECT` | both | `stream_id u32`, `code u16`, `message string` |
| `0x53` | `APP_DATA` | both | `stream_id u32`, `data bytes` (≤64 KiB) |
| `0x54` | `APP_CREDIT` | both | `stream_id u32`, `bytes u32` |
| `0x55` | `APP_CLOSE` | both | `stream_id u32` (half-close, FIN) |
| `0x56` | `APP_RESET` | both | `stream_id u32`, `code u16`, `message string` |

Tolerance rules:

- `APP_DATA`/`APP_CREDIT`/`APP_CLOSE`/`APP_RESET` with an unknown
  `stream_id` MUST be ignored (normal races after reset/close).
- Malformed body (decode failure) → `ERROR 0xFE` with `BAD_PAYLOAD` and
  close of the connection.
- Decodable but invalid `APP_OPEN` (bad `service`, bad `stream_id`,
  duplicate) → `APP_REJECT(PROTOCOL_ERROR)`.

## Flow control

Credit is per stream and per direction, in bytes of `APP_DATA` payload.
`APP_OPEN` grants the acceptor the initial window; `APP_ACCEPT` grants
the opener its window (`grant`). Default initial window: 256 KiB.

- A sender MUST NOT emit `APP_DATA` without credit. Data with no credit
  is queued per stream (cap 1 MiB); queue overflow →
  `APP_RESET(OVERFLOW)`, never silent drop.
- Outstanding credit cap is 4× the initial window; an `APP_CREDIT` that
  exceeds it → `APP_RESET(PROTOCOL_ERROR)`.
- The receiver returns credit when the local endpoint consumed bytes.
- End-to-end backpressure: slow endpoint → no credit → peer stalls →
  vsock window closes.

## Limits (v1)

| Scope | Limit |
|-------|-------|
| concurrent streams per VM | 64 |
| `APP_DATA` frame | 64 KiB |
| initial window per direction | 256 KiB |
| outstanding credit cap | 4× initial window |
| send queue per stream | 1 MiB |
| buffered memory per session | 8 MiB |
| open/accept `meta` | 1 KiB |
| idle timeout | disabled (game connections are idle-friendly) |

## Error codes

`0x0020 APP_SERVICE_NOT_FOUND`, `0x0021 APP_NOT_AUTHORIZED`,
`0x0022 APP_SERVICE_BUSY`, `0x0023 APP_QUOTA_EXCEEDED`,
`0x0024 APP_PEER_GONE`, `0x0025 APP_OVERFLOW`, `0x0026 APP_TIMEOUT`,
`0x0027 APP_PROTOCOL_ERROR`, `0x0028 APP_LOCAL_ERROR`.

## Guest contract (vhandler)

Unix sockets under `/run/shifty/app/`; no network, no ports. vhandler
creates `control.sock` as 0600 and the 0700 parent directory; each
service owns `<service_id>.sock` (recommended 0600).

- Host→guest: vhandler dials `/run/shifty/app/<service_id>.sock` per
  stream; only services declared in the VM manifest are routed.
- Guest→host: the service dials `/run/shifty/app/control.sock` and sends
  one NDJSON header line:

  ```json
  {"service":"minecraft","meta":"<base64>"}
  ```

  Response before the byte pipe:

  ```json
  {"ok":true,"meta":"<base64>"}
  {"ok":false,"code":"NOT_AUTHORIZED","message":"..."}
  ```

  Once accepted, the connection is a raw byte pipe.

## Host contract (v1)

The host half of a plugin (core, in-process) registers endpoints and
accepts guest-initiated streams through an `io.ReadWriteCloser`; core
authorizes every open and applies quotas without interpreting payloads.
External host processes and dynamic registration are out of scope for v1
and do not change the wire.

## Lifecycle

- The core→guest connection is established at VM watch and kept for the
  VM lifetime; reconnect uses 3 attempts × 250 ms per burst, then
  `2s` between bursts. A guest that does not advertise the capability is
  permanently disabled (fail-closed).
- `APP_CLOSE` is half-close; the stream ends when both sides sent it.
- `APP_RESET` is terminal and immediate.
- VM stop/crash → `APP_RESET(PEER_GONE)` to host endpoints; plugin
  disable/crash → `APP_RESET(PEER_GONE)` to guest streams; connection
  loss resets every stream with a clean reason (never ambiguous EOF).
