# Tool Calls — Wire Examples

> Request/response wire breakdowns for the Tools service (port 9005).
> All frames use the RPC layer: `IS_RESPONSE` on responses, `msg_id`
> correlation, error envelope `0xFE` on protocol errors.

## Unary: read_file

**Request** (host → guest):

```
 length: 0x00_00_00_1C   (6 + 22 payload)
   type: 0x30             (TOOL_CALL)
  flags: 0x00
 msg_id: 0x00_00_00_01
   body:
    string "read_file"    → 0x0009 + "read_file"
    bytes [params]        → uint32 len + params
      string "/src/main.go"  → 0x000C + "/src/main.go"
      uint64 0               → 0x00_00_00_00_00_00_00_00
      uint32 4096            → 0x00_00_10_00
```

**Response** (guest → host):

```
 length: 0x00_00_01_0E   (6 + 264 payload)
   type: 0x31             (TOOL_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_01    (matches)
   body:
    bytes [result]        → uint32 len + result
      bytes <data>           → uint32 256 + 256 bytes
      bool true              → 0x01 (eof)
    bool true              (ok)
    string ""              (error_msg, empty)
```

## Unary: write_file

**Request** (host → guest):

```
 length: 0x00_00_00_2E   (6 + 40 payload)
   type: 0x30             (TOOL_CALL)
  flags: 0x00
 msg_id: 0x00_00_00_02
   body:
    string "write_file"   → 0x000A + "write_file"
    bytes [params]
      string "/tmp/out"      → 0x0008 + "/tmp/out"
      bytes "hello"          → 0x00_00_00_05 + "hello"
      uint64 0               → 0x00_00_00_00_00_00_00_00
      bool false             → 0x00 (truncate)
```

**Response** (guest → host):

```
 length: 0x00_00_00_13   (6 + 13 payload)
   type: 0x31             (TOOL_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_02
   body:
    bytes [result]
      uint64 5              → 0x00_00_00_00_00_00_00_05 (written)
    bool true              (ok)
    string ""              (error_msg)
```

## Unary: edit_file

**Request** (host → guest):

```
 length: 0x00_00_00_31   (6 + 43 payload)
   type: 0x30             (TOOL_CALL)
  flags: 0x00
 msg_id: 0x00_00_00_03
   body:
    string "edit_file"    → 0x0009 + "edit_file"
    bytes [params]
      string "/src/foo.go"   → 0x000B + "/src/foo.go"
      bytes "oldFunc"        → 0x00_00_00_07 + "oldFunc"
      bytes "newFunc"        → 0x00_00_00_07 + "newFunc"
```

**Response** (guest → host):

```
 length: 0x00_00_00_13   (6 + 13 payload)
   type: 0x31             (TOOL_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_03
   body:
    bytes [result]
      bool true              → 0x01 (replaced)
      uint32 2               → 0x00_00_00_02 (count)
    bool true              (ok)
    string ""              (error_msg)
```

## Unary: glob

**Request** (host → guest):

```
 length: 0x00_00_00_24   (6 + 30 payload)
   type: 0x30             (TOOL_CALL)
  flags: 0x00
 msg_id: 0x00_00_00_04
   body:
    string "glob"          → 0x0004 + "glob"
    bytes [params]
      string "src/**/*.go"    → 0x000C + "src/**/*.go"
      string "."              → 0x0001 + "."
```

**Response** (guest → host):

```
 length: 0x00_00_00_2A   (6 + 36 payload)
   type: 0x31             (TOOL_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_04
   body:
    bytes [result]
      uint16 3               → 0x0003 (count)
      string "src/main.go"   → 0x000C + "src/main.go"
      string "src/util.go"   → 0x000C + "src/util.go"
      string "src/test.go"   → 0x000C + "src/test.go"
    bool true              (ok)
    string ""              (error_msg)
```

## Unary: grep

**Request** (host → guest):

```
 length: 0x00_00_00_24   (6 + 30 payload)
   type: 0x30             (TOOL_CALL)
  flags: 0x00
 msg_id: 0x00_00_00_05
   body:
    string "grep"          → 0x0004 + "grep"
    bytes [params]
      string "func.*Test"     → 0x000B + "func.*Test"
      string "src/"           → 0x0004 + "src/"
```

**Response** (guest → host):

```
 length: 0x00_00_00_3E   (6 + 56 payload)
   type: 0x31             (TOOL_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_05
   body:
    bytes [result]
      uint16 2               → 0x0002 (count)
      string "src/main.go"   → 0x000C + "src/main.go"
      uint32 42              → 0x00_00_00_2A (line)
      bytes "func TestMain"  → 0x00_00_00_0D + "func TestMain"
      string "src/util.go"   → 0x000C + "src/util.go"
      uint32 15              → 0x00_00_00_0F (line)
      bytes "func TestUtil"  → 0x00_00_00_0D + "func TestUtil"
    bool true              (ok)
    string ""              (error_msg)
```

## Unary: bash

**Request** (host → guest):

```
 length: 0x00_00_00_36   (6 + 48 payload)
   type: 0x30             (TOOL_CALL)
  flags: 0x00
 msg_id: 0x00_00_00_06
   body:
    string "bash"          → 0x0004 + "bash"
    bytes [params]
      string "go build ./..." → 0x0010 + "go build ./..."
      string "/src"           → 0x0004 + "/src"
      map<string,string> {}   → 0x0000
      uint32 60000            → 0x00_00_EA_60 (timeout_ms)
```

**Response** (guest → host):

```
 length: 0x00_00_00_1A   (6 + 20 payload)
   type: 0x31             (TOOL_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_06
   body:
    bytes [result]
      int32 0               → 0x00_00_00_00 (exit_code)
      bytes ""              → 0x00_00_00_00 (stdout)
      bytes ""              → 0x00_00_00_00 (stderr)
    bool true              (ok)
    string ""              (error_msg)
```

## Unary: LIST_TOOLS

**Request** (host → guest):

```
 length: 0x00_00_00_06   (6 + 0 payload)
   type: 0x32             (LIST_TOOLS)
  flags: 0x00
 msg_id: 0x00_00_00_07
```

**Response** (guest → host): each tool is a `ToolDescriptor` with name,
description, version, capabilities, permissions, and JSON Schema.

```
 length: 0x00_00_01_5C   (6 + 342 payload, approximate)
   type: 0x33             (LIST_TOOLS_RESULT)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_07
   body:
    uint16 6               → 0x0006 (count)

    ── entry 0: read_file ──
    string "read_file"     → 0x0009 + "read_file"
    string "Read a chunk of a file at offset"
    string "1.0.0"

    uint16 2               → capabilities count
      string "fs:read"
      string "seekable"

    uint16 1               → permissions count
      string "fs:read"

    bytes <schema>         → JSON Schema for read_file params

    ── entry 1: bash ──
    string "bash"          → 0x0004 + "bash"
    string "Execute a shell command inside the guest"
    string "1.0.0"

    uint16 1               → capabilities count
      string "exec:shell"

    uint16 1               → permissions count
      string "exec"

    bytes <schema>         → JSON Schema for bash params

    ... (write_file, edit_file, glob, grep follow same structure)
```

## Error: unknown tool

**Request** (host → guest):

```
 length: 0x00_00_00_16   (6 + 16 payload)
   type: 0x30             (TOOL_CALL)
  flags: 0x00
 msg_id: 0x00_00_00_08
   body:
    string "unknown_tool"  → 0x000C + "unknown_tool"
    bytes ""               → 0x00_00_00_00 (empty params)
```

**Response** (guest → host):

```
 length: 0x00_00_00_19   (6 + 19 payload)
   type: 0xFE             (ERROR)
  flags: 0x01             (IS_RESPONSE)
 msg_id: 0x00_00_00_08
   body:
    uint16 0x0009          (UNKNOWN_TOOL)
    string "unknown_tool is not registered"
```

---

See also:
- [services/tools.md](../services/tools.md) for the full service specification.
- [services/rpc.md](../services/rpc.md) for the RPC layer contract.
- [error.md](error.md) for additional error wire examples.
