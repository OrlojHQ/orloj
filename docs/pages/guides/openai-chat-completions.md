# Call AgentSystems via OpenAI Chat Completions

Orloj exposes an OpenAI-compatible `POST /v1/chat/completions` facade so existing
OpenAI client libraries can invoke an AgentSystem by setting `base_url` and
`model`.

Under the hood Orloj creates a one-turn Session. The Session executes through
the normal AgentSystem Task runtime and maps its completed assistant message
into an OpenAI-shaped completion.

## Prerequisites

- Orloj server (`orlojd`) running with a worker (for example `--embedded-worker`)
- An AgentSystem already applied
- A writer-capable API token (same auth as `POST /v1/tasks`)

## Quick start

Point any OpenAI-compatible client at your Orloj API:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:8080/v1"
export OPENAI_API_KEY="<orloj-writer-token>"

curl -sS "$OPENAI_BASE_URL/chat/completions" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "report-system",
    "messages": [
      {"role": "user", "content": "Summarize AI startups this week"}
    ]
  }'
```

- `model` is the **AgentSystem name**, not a foundation model id
- Namespace defaults to `default`; pass `?namespace=<ns>` when needed
- Message text is flattened into the Session turn content

With the official Python OpenAI SDK:

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8080/v1",
    api_key="<orloj-writer-token>",
)

completion = client.chat.completions.create(
    model="report-system",
    messages=[{"role": "user", "content": "Summarize AI startups this week"}],
)
print(completion.choices[0].message.content)
```

## Streaming

`stream: true` opens an OpenAI-shaped SSE (`text/event-stream`) response
immediately, forwards model text deltas as completion chunks, sends keepalives
while no output is available, and ends with `data: [DONE]`.
OpenAI-compatible and Anthropic endpoints stream natively; other model
gateways currently emit one content chunk after their blocking call completes.

## How the answer is chosen

The response uses the Session turn's durable `message.completed` event. For
streaming requests, `message.delta` events are forwarded as they arrive and the
completed event provides the authoritative final message. If durable completion
diverges from already-emitted text, the stream ends with an error because the
OpenAI SSE format cannot retract prior chunks.

## Limitations

- Multimodal `content` arrays (for example `image_url` parts) are rejected; use string content only
- OpenAI fields such as `tools`, `tool_choice`, `temperature`, and `n` are ignored
- Multi-agent systems return a best-effort final output (usually `last_output`)
- Runs that require an approval return `409 Conflict` instead of hanging
- Failed Sessions return an OpenAI-style error body with the runtime error

For Orloj-native control (templates, approvals, message inspection), use the
Session API (`POST /v1/sessions`) or the SDKs instead of this facade.
