# Assistant protocol v2.5

The v2 endpoints and fields remain unchanged except for the additions below.
The client executes the thirteen v1 deployment tools listed in
[assistant-tools-v1.json](assistant-tools-v1.json) and described in the
[Assistant guide](assistant.md#tools-and-limits). The service alone executes
`read_core`.

## Step context

`POST /v1/turns/{turnId}/steps` receives `{ "context": …, "messages": [ … ] }`.
The v2.5 client must send context at every step. The service continues to accept
an absent context for compatibility with existing tests.

```json
{
  "nixorium": "3.3.0",
  "core": {
    "owner": "giovantenne",
    "repo": "nixorium",
    "rev": "<40 lowercase hexadecimal characters>"
  },
  "files": ["flake.nix", "modules/clients.nix", "…"]
}
```

`core` is optional. Both client and service validate `owner` and `repo` against
`^[A-Za-z0-9_.-]{1,100}$` and `rev` against `^[0-9a-f]{40}$`. No additional
fields are allowed inside `core`. The client computes the context once per
turn, at the beginning of `Run`, and sends the same object at every step of
that turn.

## Capabilities

Step requests may carry the optional `X-Nixorium-Capabilities` HTTP header, a
comma-separated list drawn from `prepare_npm_v1`, `host_diagnostics_v1`,
`git_history_v1`, `prefetch_v1`, `settings_v1`, `home_files_v1` and
`system_files_v1`. The request
JSON is unchanged and sharing routes never carry the header. The service offers
a capability-gated tool only to a controller that lists its capability:
`prefetch` requires `prefetch_v1` and `read_system_file` requires
`system_files_v1`. `settings_v1` adds no tool: it tells the
service that `write_file` of `lab-settings.json` accepts changes to the
writable settings listed in the [Assistant guide](assistant.md#laboratory-settings)
and refuses any other with `settings_field_read_only` or `settings_invalid`.
Older services ignore the header; older controllers never send it.

## Service messages event

The service may send `service_messages` once per step, immediately before the
terminal `tool_calls` or `final` event:

```text
event: service_messages
data: {"messages":[{"role":"assistant","content":"","toolCalls":[{"id":"call_1","name":"read_core","arguments":{"path":"modules/client-internet.nix"}}]},{"role":"tool","toolCallId":"call_1","content":"{\"coreRef\":{\"rev\":\"<rev>\",\"sha256\":\"<64 hex>\"}}"}]}
```

The messages form a sequence of blocks. Each block starts with an `assistant`
message whose `content` is empty and whose `toolCalls` contain only `read_core`.
It is followed by one `tool` message for each call, in the same order and with
the matching `toolCallId`.

Serialized call arguments must not exceed 4096 bytes. Each tool message's
content must not exceed 1024 bytes and must be a JSON object with only the
`coreRef` key. That object contains exactly `rev` (40 hexadecimal characters)
and `sha256` (64 hexadecimal characters). An event contains at most 256 messages.

The client adds these messages to history before the step's assistant message,
which contains local tool calls or the final answer. If the step ends with an
error or an interruption, the client discards the service messages. A second
`service_messages` event in the same step, or one after a terminal event, is a
`protocol_error`.

## Client history

History may contain assistant messages with `read_core` calls and their
corresponding tool messages with `coreRef`. The client sends these messages
back unchanged. It never executes `read_core` and rejects a `tool_calls` event
containing `read_core` with `protocol_error`.
