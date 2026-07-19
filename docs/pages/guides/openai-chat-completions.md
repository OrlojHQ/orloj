# Call AgentSystems via OpenAI Chat Completions

Orloj exposes an OpenAI-compatible `POST /v1/chat/completions` facade so existing
OpenAI client libraries can invoke an AgentSystem by setting `base_url` and
`model`.

Under the hood Orloj creates a Task, waits for a terminal phase, and maps
`status.output` into an OpenAI-shaped completion.

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
- Message text is flattened into the Task input key `topic`

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
immediately, sends keepalives while the Task runs, emits the final content, and
then sends `data: [DONE]`. This is not token streaming from the model loop.

## How the answer is chosen

On success, assistant content is taken from Task `status.output` in this order:

1. `last_output` (message-driven runs)
2. `response`
3. `result` when it is not the sentinel value `executed`
4. highest `agent.N.message_content` (sequential pipeline handoffs)

If none of those keys contain usable text, the request fails even when the Task
phase is `Succeeded`.

## Limitations

- Multimodal `content` arrays (for example `image_url` parts) are rejected; use string content only
- OpenAI fields such as `tools`, `tool_choice`, `temperature`, and `n` are ignored
- Multi-agent systems return a best-effort final output (usually `last_output`)
- Tasks that enter `WaitingApproval` return `409 Conflict` instead of hanging
- Failed / dead-letter tasks return an OpenAI-style error body with the task error

For Orloj-native control (templates, approvals, message inspection), use the
Task API (`POST /v1/tasks`) or the SDKs instead of this facade.
