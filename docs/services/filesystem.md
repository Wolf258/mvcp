# Filesystem Service (Obsolete)

> **Superseded by [tools.md](tools.md).**
> Message type range `0x30`–`0x3F` is now used by the Tools service on port 9005.
> The Filesystem service never shipped; it was always reserved.

The wire format below is kept for historical reference. All filesystem
operations (`read_file`, `write_file`, `stat`, `list_dir`, etc.) are now
available through the Tools service's generic `TOOL_CALL` dispatch.

---

The filesystem service provides filesystem metadata and I/O operations
inside the guest: working directory, stat, directory listing, and
small-file read/write.

## Message Types

| Type | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x30` | `GET_CWD` | H→G | *(none)* |
| `0x31` | `CWD_RESULT` | G→H | `string cwd` |
| `0x32` | `STAT` | H→G | `string path` |
| `0x33` | `STAT_RESULT` | G→H | `uint64 size`, `uint64 mode`, `int64 mtime_ns`, `bool exists` |
| `0x34` | `LIST_DIR` | H→G | `string path` |
| `0x35` | `LIST_DIR_RESULT` | G→H | `uint16 count` + N × (`string name`, `bool is_dir`) |
| `0x36` | `READ_FILE` | H→G | `string path`, `uint64 offset`, `uint32 max_bytes` |
| `0x37` | `READ_FILE_RESULT` | G→H | `bytes data`, `bool eof` |
| `0x38` | `WRITE_FILE` | H→G | `string path`, `bytes data`, `uint64 offset`, `bool truncate` |
| `0x39` | `WRITE_FILE_RESULT` | G→H | `uint64 written_bytes`, `bool ok` |
| `0x3A`–`0x3F` | *(reserved)* | — | — |

## GET_CWD / CWD_RESULT

Get the current working directory in the guest.

**CWD_RESULT** payload:

| Field | Encoding | Description |
|-------|----------|-------------|
| `cwd` | `string` | Absolute path of the current working directory |

## STAT / STAT_RESULT

Query file or directory metadata.

**STAT_RESULT** payload:

| Field | Encoding | Description |
|-------|----------|-------------|
| `size` | `uint64` | File size in bytes (0 for directories) |
| `mode` | `uint64` | Unix file mode + type (`st_mode`) |
| `mtime_ns` | `int64` | Modification time, Unix nanoseconds |
| `exists` | `bool` | `false` if the path does not exist |

## LIST_DIR / LIST_DIR_RESULT

List directory contents.

**LIST_DIR_RESULT** payload:

| Field | Encoding | Description |
|-------|----------|-------------|
| `count` | `uint16` | Number of entries |
| `entries` | N × (`string name`, `bool is_dir`) | Directory entries |

## READ_FILE / READ_FILE_RESULT

Read a chunk of a file. For large files, use the File Transfer service
(port 9004, [file-transfer.md](file-transfer.md)) instead of repeated READ_FILE.

**READ_FILE_RESULT** payload:

| Field | Encoding | Description |
|-------|----------|-------------|
| `data` | `bytes` | File data starting at `offset`, up to `max_bytes` |
| `eof` | `bool` | `true` if the read reached end of file |

## WRITE_FILE / WRITE_FILE_RESULT

Write data to a file. For large files, use the File Transfer service
(port 9004, [file-transfer.md](file-transfer.md)) instead of repeated WRITE_FILE.

**WRITE_FILE_RESULT** payload:

| Field | Encoding | Description |
|-------|----------|-------------|
| `written_bytes` | `uint64` | Total bytes written |
| `ok` | `bool` | `true` if write succeeded |

### Write Semantics

- If `truncate` is `true`, the file is truncated to 0 before writing.
- If `offset` is non-zero, data is written at that position (pwrite).
- If `offset` is zero and `truncate` is false, data is appended.

---

See also:
- [file-transfer.md](file-transfer.md) for chunked streaming file import/export (large files).
- [execution.md](execution.md) for command execution with `cwd`.
