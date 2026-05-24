#!/usr/bin/env python3
"""Regenerate openapi/openapi.yaml from structured path definitions."""
from __future__ import annotations

import json
import pathlib
import subprocess


def schema_ref(path: str) -> dict:
    return {"$ref": path}


def json_body(ref_path: str) -> dict:
    return {"application/json": {"schema": schema_ref(ref_path)}}


def text_plain_error() -> dict:
    return {
        "text/plain": {
            "schema": schema_ref("./schemas/common.yaml#/components/schemas/PlainTextError")
        }
    }


SEC_READER = [{"bearerAuth": []}, {"sessionCookie": []}]
SEC_WRITER = [{"bearerAuth": []}, {"sessionCookie": []}]
SEC_ADMIN = [{"bearerAuth": []}, {"sessionCookie": []}]
SEC_CTRL = [{"bearerAuth": []}, {"sessionCookie": []}]
SEC_A2A = [{"bearerAuth": []}]


def list_op(tag: str, list_ref: str) -> dict:
    return {
        "tags": [tag],
        "parameters": [
            {"name": "namespace", "in": "query", "schema": {"type": "string"}},
            {
                "name": "limit",
                "in": "query",
                "schema": {"type": "integer", "minimum": 1, "maximum": 1000},
            },
            {"name": "after", "in": "query", "schema": {"type": "string"}},
            {"name": "labelSelector", "in": "query", "schema": {"type": "string"}},
        ],
        "security": SEC_READER,
        "responses": {
            "200": {"description": "OK", "content": json_body(list_ref)},
            "503": {"description": "Store unavailable", "content": text_plain_error()},
            "default": {"description": "Error", "content": text_plain_error()},
        },
    }


def list_tasks(tag: str, list_ref: str) -> dict:
    op = list_op(tag, list_ref)
    op["parameters"].insert(
        3,
        {"name": "offset", "in": "query", "schema": {
            "type": "integer", "minimum": 0}},
    )
    return op


def post_create(tag: str, item_ref: str) -> dict:
    return {
        "tags": [tag],
        "security": SEC_WRITER,
        "requestBody": {"required": True, "content": json_body(item_ref)},
        "responses": {
            "201": {"description": "Created", "content": json_body(item_ref)},
            "400": {"description": "Bad request", "content": text_plain_error()},
            "503": {"description": "Store unavailable", "content": text_plain_error()},
            "default": {"description": "Error", "content": text_plain_error()},
        },
    }


def get_one(tag: str, item_ref: str) -> dict:
    return {
        "tags": [tag],
        "parameters": [
            {"name": "name", "in": "path", "required": True,
                "schema": {"type": "string"}},
            {"name": "namespace", "in": "query", "schema": {"type": "string"}},
        ],
        "security": SEC_READER,
        "responses": {
            "200": {"description": "OK", "content": json_body(item_ref)},
            "404": {"description": "Not found", "content": text_plain_error()},
            "503": {"description": "Store unavailable", "content": text_plain_error()},
            "default": {"description": "Error", "content": text_plain_error()},
        },
    }


def put_one(tag: str, item_ref: str) -> dict:
    return {
        "tags": [tag],
        "parameters": [
            {"name": "name", "in": "path", "required": True,
                "schema": {"type": "string"}},
            {"name": "namespace", "in": "query", "schema": {"type": "string"}},
            {
                "name": "If-Match",
                "in": "header",
                "required": False,
                "schema": {"type": "string"},
            },
        ],
        "security": SEC_WRITER,
        "requestBody": {"required": True, "content": json_body(item_ref)},
        "responses": {
            "200": {"description": "OK", "content": json_body(item_ref)},
            "400": {"description": "Bad request", "content": text_plain_error()},
            "404": {"description": "Not found", "content": text_plain_error()},
            "409": {"description": "Conflict", "content": text_plain_error()},
            "503": {"description": "Store unavailable", "content": text_plain_error()},
            "default": {"description": "Error", "content": text_plain_error()},
        },
    }


def delete_one(tag: str) -> dict:
    return {
        "tags": [tag],
        "parameters": [
            {"name": "name", "in": "path", "required": True,
                "schema": {"type": "string"}},
            {"name": "namespace", "in": "query", "schema": {"type": "string"}},
        ],
        "security": SEC_WRITER,
        "responses": {
            "204": {"description": "Deleted"},
            "404": {"description": "Not found", "content": text_plain_error()},
            "503": {"description": "Store unavailable", "content": text_plain_error()},
            "default": {"description": "Error", "content": text_plain_error()},
        },
    }


def status_get_put(tag: str) -> tuple[dict, dict]:
    base_params = [
        {"name": "name", "in": "path", "required": True,
            "schema": {"type": "string"}},
        {"name": "namespace", "in": "query", "schema": {"type": "string"}},
    ]
    env = "./schemas/common.yaml#/components/schemas/StatusEnvelope"
    get_op = {
        "tags": [tag],
        "parameters": base_params,
        "security": SEC_READER,
        "responses": {
            "200": {"description": "OK", "content": json_body(env)},
            "404": {"description": "Not found", "content": text_plain_error()},
            "503": {"description": "Store unavailable", "content": text_plain_error()},
            "default": {"description": "Error", "content": text_plain_error()},
        },
    }
    put_op = {
        "tags": [tag],
        "parameters": base_params
        + [
            {
                "name": "If-Match",
                "in": "header",
                "required": False,
                "schema": {"type": "string"},
            }
        ],
        "security": SEC_CTRL,
        "requestBody": {"required": True, "content": json_body(env)},
        "responses": {
            "200": {"description": "OK", "content": json_body(env)},
            "400": {"description": "Bad request", "content": text_plain_error()},
            "404": {"description": "Not found", "content": text_plain_error()},
            "409": {"description": "Conflict", "content": text_plain_error()},
            "503": {"description": "Store unavailable", "content": text_plain_error()},
            "default": {"description": "Error", "content": text_plain_error()},
        },
    }
    return get_op, put_op


def sse_response(desc: str) -> dict:
    return {
        "200": {
            "description": desc,
            "content": {"text/event-stream": {"schema": {"type": "string"}}},
        },
        "default": {"description": "Error", "content": text_plain_error()},
    }


def watch_params() -> list:
    return [
        {"name": "resourceVersion", "in": "query", "schema": {"type": "string"}},
        {"name": "name", "in": "query", "schema": {"type": "string"}},
        {"name": "namespace", "in": "query", "schema": {"type": "string"}},
    ]


def add_crud(
    paths: dict,
    base: str,
    tag: str,
    list_ref: str,
    item_ref: str,
    *,
    with_status: bool = False,
) -> None:
    paths[base] = {"get": list_op(tag, list_ref),
                   "post": post_create(tag, item_ref)}
    if with_status:
        g, p = status_get_put(tag)
        paths[f"{base}/{{name}}/status"] = {"get": g, "put": p}
    paths[f"{base}/{{name}}"] = {
        "get": get_one(tag, item_ref),
        "put": put_one(tag, item_ref),
        "delete": delete_one(tag),
    }


def build() -> dict:
    paths: dict = {}

    paths["/v1/agents"] = {
        "get": list_op("agents", "./schemas/agent.yaml#/components/schemas/AgentList"),
        "post": post_create("agents", "./schemas/agent.yaml#/components/schemas/Agent"),
    }
    paths["/v1/agents/watch"] = {
        "get": {
            "tags": ["agents"],
            "security": SEC_READER,
            "parameters": watch_params(),
            "responses": sse_response(
                "SSE; `event: resource` with WatchResourceEvent JSON in data"
            ),
        }
    }
    g, p = status_get_put("agents")
    paths["/v1/agents/{name}/status"] = {"get": g, "put": p}
    paths["/v1/agents/{name}/logs"] = {
        "get": {
            "tags": ["agents"],
            "security": SEC_READER,
            "parameters": [
                {
                    "name": "name",
                    "in": "path",
                    "required": True,
                    "schema": {"type": "string"},
                },
                {"name": "namespace", "in": "query", "schema": {"type": "string"}},
            ],
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/NamedLogsResponse"
                    ),
                },
                "404": {"description": "Not found", "content": text_plain_error()},
                "503": {"description": "Store unavailable", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/agents/{name}"] = {
        "get": get_one("agents", "./schemas/agent.yaml#/components/schemas/Agent"),
        "put": put_one("agents", "./schemas/agent.yaml#/components/schemas/Agent"),
        "delete": delete_one("agents"),
    }

    add_crud(
        paths,
        "/v1/agent-systems",
        "agent-systems",
        "./schemas/agent-system.yaml#/components/schemas/AgentSystemList",
        "./schemas/agent-system.yaml#/components/schemas/AgentSystem",
        with_status=True,
    )
    add_crud(
        paths,
        "/v1/model-endpoints",
        "model-endpoints",
        "./schemas/model-endpoint.yaml#/components/schemas/ModelEndpointList",
        "./schemas/model-endpoint.yaml#/components/schemas/ModelEndpoint",
        with_status=True,
    )
    mep = paths["/v1/model-endpoints/{name}"]
    mep["put"] = {
        **mep["put"],
        "summary": "Replace a model endpoint (supports rename)",
        "description": (
            "Updates the model endpoint identified by the path `name`. If `metadata.name` in the "
            "body differs from the path,\nthe stored resource is renamed to match the body "
            "(HTTP 409 if that name already exists in the namespace)."
        ),
    }
    add_crud(
        paths,
        "/v1/tools",
        "tools",
        "./schemas/tool.yaml#/components/schemas/ToolList",
        "./schemas/tool.yaml#/components/schemas/Tool",
        with_status=True,
    )
    add_crud(
        paths,
        "/v1/secrets",
        "secrets",
        "./schemas/secret.yaml#/components/schemas/SecretList",
        "./schemas/secret.yaml#/components/schemas/Secret",
        with_status=False,
    )
    sec = paths["/v1/secrets/{name}"]
    sec["get"] = {
        **sec["get"],
        "summary": "Get a secret",
        "description": (
            "Returns the secret with `spec.data` values redacted as `***`; "
            "`spec.stringData` is omitted (see **secrets** tag)."
        ),
    }
    sec["get"]["responses"]["200"]["description"] = (
        "OK (sensitive values redacted in `spec.data`; `spec.stringData` omitted)"
    )
    sec["put"] = {
        **sec["put"],
        "summary": "Replace a secret (supports rename)",
        "description": (
            "See the **secrets** tag for redaction (`***`) and PUT merge behavior; "
            "PUT rename follows the same rules as other namespaced resources."
        ),
    }
    add_crud(
        paths,
        "/v1/sealed-secrets",
        "sealed-secrets",
        "./schemas/sealed-secret.yaml#/components/schemas/SealedSecretList",
        "./schemas/sealed-secret.yaml#/components/schemas/SealedSecret",
        with_status=False,
    )
    paths["/v1/sealing-key/public"] = {
        "get": {
            "tags": ["sealed-secrets"],
            "summary": "Get the active sealing public key",
            "description": (
                "Returns the active public key used to seal `SealedSecret` manifests. "
                "When no active sealing key is configured, the server returns HTTP 503."
            ),
            "security": SEC_READER,
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/sealed-secret.yaml#/components/schemas/SealedSecretPublicKeyResponse"
                    ),
                },
                "503": {"description": "Sealing key unavailable", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    add_crud(
        paths,
        "/v1/memories",
        "memories",
        "./schemas/memory.yaml#/components/schemas/MemoryList",
        "./schemas/memory.yaml#/components/schemas/Memory",
        with_status=True,
    )

    paths["/v1/memories/{name}/entries"] = {
        "get": {
            "tags": ["memories"],
            "security": SEC_READER,
            "parameters": [
                {
                    "name": "name",
                    "in": "path",
                    "required": True,
                    "schema": {"type": "string"},
                },
                {"name": "namespace", "in": "query", "schema": {"type": "string"}},
                {"name": "q", "in": "query", "schema": {"type": "string"}},
                {"name": "prefix", "in": "query", "schema": {"type": "string"}},
                {"name": "limit", "in": "query", "schema": {"type": "integer"}},
            ],
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/MemoryEntriesResponse"
                    ),
                },
                "404": {"description": "Not found", "content": text_plain_error()},
                "500": {"description": "Backend error", "content": text_plain_error()},
                "503": {"description": "Store unavailable", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }

    add_crud(
        paths,
        "/v1/agent-policies",
        "agent-policies",
        "./schemas/agent-policy.yaml#/components/schemas/AgentPolicyList",
        "./schemas/agent-policy.yaml#/components/schemas/AgentPolicy",
        with_status=True,
    )
    add_crud(
        paths,
        "/v1/agent-roles",
        "agent-roles",
        "./schemas/agent-role.yaml#/components/schemas/AgentRoleList",
        "./schemas/agent-role.yaml#/components/schemas/AgentRole",
        with_status=False,
    )
    add_crud(
        paths,
        "/v1/tool-permissions",
        "tool-permissions",
        "./schemas/tool-permission.yaml#/components/schemas/ToolPermissionList",
        "./schemas/tool-permission.yaml#/components/schemas/ToolPermission",
        with_status=False,
    )

    paths["/v1/tool-approvals"] = {
        "get": list_op(
            "tool-approvals",
            "./schemas/tool-approval.yaml#/components/schemas/ToolApprovalList",
        ),
        "post": post_create(
            "tool-approvals",
            "./schemas/tool-approval.yaml#/components/schemas/ToolApproval",
        ),
    }
    paths["/v1/tool-approvals/{name}"] = {
        "get": get_one(
            "tool-approvals",
            "./schemas/tool-approval.yaml#/components/schemas/ToolApproval",
        ),
        "delete": delete_one("tool-approvals"),
        "put": {
            "tags": ["tool-approvals"],
            "summary": "Not supported",
            "parameters": [
                {"name": "name", "in": "path", "required": True,
                    "schema": {"type": "string"}}
            ],
            "security": SEC_WRITER,
            "responses": {
                "405": {
                    "description": "Method not allowed",
                    "content": text_plain_error(),
                },
                "default": {"description": "Error", "content": text_plain_error()},
            },
        },
    }
    for suf, title in [
        ("approve", "Approve pending tool invocation"),
        ("deny", "Deny pending tool invocation"),
    ]:
        paths[f"/v1/tool-approvals/{{name}}/{suf}"] = {
            "post": {
                "tags": ["tool-approvals"],
                "summary": title,
                "parameters": [
                    {
                        "name": "name",
                        "in": "path",
                        "required": True,
                        "schema": {"type": "string"},
                    },
                    {"name": "namespace", "in": "query",
                        "schema": {"type": "string"}},
                ],
                "security": SEC_WRITER,
                "requestBody": {
                    "required": False,
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/ToolApprovalDecisionRequest"
                    ),
                },
                "responses": {
                    "200": {
                        "description": "OK",
                        "content": json_body(
                            "./schemas/tool-approval.yaml#/components/schemas/ToolApproval"
                        ),
                    },
                    "404": {"description": "Not found", "content": text_plain_error()},
                    "409": {"description": "Conflict", "content": text_plain_error()},
                    "503": {
                        "description": "Store unavailable",
                        "content": text_plain_error(),
                    },
                    "default": {"description": "Error", "content": text_plain_error()},
                },
            }
        }

    paths["/v1/task-approvals"] = {
        "get": list_op(
            "task-approvals",
            "./schemas/task-approval.yaml#/components/schemas/TaskApprovalList",
        ),
        "post": post_create(
            "task-approvals",
            "./schemas/task-approval.yaml#/components/schemas/TaskApproval",
        ),
    }
    paths["/v1/task-approvals/{name}"] = {
        "get": get_one(
            "task-approvals",
            "./schemas/task-approval.yaml#/components/schemas/TaskApproval",
        ),
        "delete": delete_one("task-approvals"),
        "put": {
            "tags": ["task-approvals"],
            "summary": "Not supported",
            "parameters": [
                {"name": "name", "in": "path", "required": True,
                    "schema": {"type": "string"}}
            ],
            "security": SEC_WRITER,
            "responses": {
                "405": {
                    "description": "Method not allowed",
                    "content": text_plain_error(),
                },
                "default": {"description": "Error", "content": text_plain_error()},
            },
        },
    }
    for suf, title in [
        ("approve", "Approve pending task review"),
        ("deny", "Deny pending task review"),
    ]:
        paths[f"/v1/task-approvals/{{name}}/{suf}"] = {
            "post": {
                "tags": ["task-approvals"],
                "summary": title,
                "parameters": [
                    {
                        "name": "name",
                        "in": "path",
                        "required": True,
                        "schema": {"type": "string"},
                    },
                    {"name": "namespace", "in": "query",
                        "schema": {"type": "string"}},
                ],
                    "security": SEC_WRITER,
                    "requestBody": {
                        "required": False,
                        "content": json_body(
                            "./schemas/common.yaml#/components/schemas/TaskApprovalDecisionRequest"
                    ),
                },
                "responses": {
                    "200": {
                        "description": "OK",
                        "content": json_body(
                            "./schemas/task-approval.yaml#/components/schemas/TaskApproval"
                        ),
                    },
                    "404": {"description": "Not found", "content": text_plain_error()},
                    "409": {"description": "Conflict", "content": text_plain_error()},
                    "503": {
                        "description": "Store unavailable",
                        "content": text_plain_error(),
                    },
                    "default": {"description": "Error", "content": text_plain_error()},
                },
            }
        }

    paths["/v1/task-approvals/{name}/request-changes"] = {
        "post": {
            "tags": ["task-approvals"],
            "summary": "Request changes on pending task review",
            "description": (
                "Reruns the producing agent with reviewer feedback. "
                "Requires `comment` or the legacy `reason` field and returns `409 Conflict` "
                "when the checkpoint disables request_changes or has reached max_review_cycles."
            ),
            "parameters": [
                {
                    "name": "name",
                    "in": "path",
                    "required": True,
                    "schema": {"type": "string"},
                },
                {"name": "namespace", "in": "query", "schema": {"type": "string"}},
            ],
            "security": SEC_WRITER,
            "requestBody": {
                "required": True,
                "content": json_body(
                    "./schemas/common.yaml#/components/schemas/TaskApprovalRequestChangesRequest"
                ),
            },
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/task-approval.yaml#/components/schemas/TaskApproval"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "404": {"description": "Not found", "content": text_plain_error()},
                "409": {"description": "Conflict", "content": text_plain_error()},
                "503": {
                    "description": "Store unavailable",
                    "content": text_plain_error(),
                },
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }

    paths["/v1/tasks"] = {
        "get": list_tasks("tasks", "./schemas/task.yaml#/components/schemas/TaskList"),
        "post": post_create("tasks", "./schemas/task.yaml#/components/schemas/Task"),
    }
    paths["/v1/tasks/watch"] = {
        "get": {
            "tags": ["tasks"],
            "security": SEC_READER,
            "parameters": watch_params(),
            "responses": sse_response("SSE resource watch for tasks"),
        }
    }
    tg, tp = status_get_put("tasks")
    paths["/v1/tasks/{name}/status"] = {"get": tg, "put": tp}

    msg_params = [
        {
            "name": "name",
            "in": "path",
            "required": True,
            "schema": {"type": "string"},
        },
        {"name": "namespace", "in": "query", "schema": {"type": "string"}},
        {"name": "phase", "in": "query", "schema": {"type": "string"}},
        {"name": "lifecycle", "in": "query", "schema": {"type": "string"}},
        {"name": "from_agent", "in": "query", "schema": {"type": "string"}},
        {"name": "to_agent", "in": "query", "schema": {"type": "string"}},
        {"name": "branch_id", "in": "query", "schema": {"type": "string"}},
        {"name": "trace_id", "in": "query", "schema": {"type": "string"}},
        {"name": "limit", "in": "query", "schema": {
            "type": "integer", "minimum": 0}},
    ]

    paths["/v1/tasks/{name}/logs"] = {
        "get": {
            "tags": ["tasks"],
            "security": SEC_READER,
            "parameters": msg_params[:2],
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/NamedLogsResponse"
                    ),
                },
                "404": {"description": "Not found", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/tasks/{name}/messages"] = {
        "get": {
            "tags": ["tasks"],
            "security": SEC_READER,
            "parameters": msg_params,
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/TaskMessageListResponse"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "404": {"description": "Not found", "content": text_plain_error()},
                "503": {"description": "Store unavailable", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/tasks/{name}/metrics"] = {
        "get": {
            "tags": ["tasks"],
            "security": SEC_READER,
            "parameters": msg_params,
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/TaskMessageMetricsResponse"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "404": {"description": "Not found", "content": text_plain_error()},
                "503": {"description": "Store unavailable", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/tasks/{name}"] = {
        "get": get_one("tasks", "./schemas/task.yaml#/components/schemas/Task"),
        "put": put_one("tasks", "./schemas/task.yaml#/components/schemas/Task"),
        "delete": delete_one("tasks"),
    }

    add_crud(
        paths,
        "/v1/task-schedules",
        "task-schedules",
        "./schemas/task-schedule.yaml#/components/schemas/TaskScheduleList",
        "./schemas/task-schedule.yaml#/components/schemas/TaskSchedule",
        with_status=True,
    )
    paths["/v1/task-schedules/watch"] = {
        "get": {
            "tags": ["task-schedules"],
            "security": SEC_READER,
            "parameters": watch_params(),
            "responses": sse_response("SSE resource watch for task schedules"),
        }
    }

    paths["/v1/task-webhooks"] = {
        "get": {
            "tags": ["task-webhooks"],
            "security": SEC_READER,
            "parameters": [
                {"name": "namespace", "in": "query", "schema": {"type": "string"}},
                {"name": "labelSelector", "in": "query",
                    "schema": {"type": "string"}},
            ],
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/task-webhook.yaml#/components/schemas/TaskWebhookList"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "503": {"description": "Store unavailable", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        },
        "post": post_create(
            "task-webhooks",
            "./schemas/task-webhook.yaml#/components/schemas/TaskWebhook",
        ),
    }
    paths["/v1/task-webhooks/watch"] = {
        "get": {
            "tags": ["task-webhooks"],
            "security": SEC_READER,
            "parameters": watch_params(),
            "responses": sse_response("SSE resource watch for task webhooks"),
        }
    }
    wg, wp = status_get_put("task-webhooks")
    paths["/v1/task-webhooks/{name}/status"] = {"get": wg, "put": wp}
    paths["/v1/task-webhooks/{name}"] = {
        "get": get_one(
            "task-webhooks",
            "./schemas/task-webhook.yaml#/components/schemas/TaskWebhook",
        ),
        "put": put_one(
            "task-webhooks",
            "./schemas/task-webhook.yaml#/components/schemas/TaskWebhook",
        ),
        "delete": delete_one("task-webhooks"),
    }

    add_crud(
        paths,
        "/v1/workers",
        "workers",
        "./schemas/worker.yaml#/components/schemas/WorkerList",
        "./schemas/worker.yaml#/components/schemas/Worker",
        with_status=True,
    )

    paths["/v1/mcp-servers"] = {
        "get": list_op(
            "mcp-servers",
            "./schemas/mcp-server.yaml#/components/schemas/McpServerList",
        ),
        "post": {
            **post_create(
                "mcp-servers",
                "./schemas/mcp-server.yaml#/components/schemas/McpServer",
            ),
            "security": SEC_ADMIN,
        },
    }
    paths["/v1/mcp-servers/{name}"] = {
        "get": get_one(
            "mcp-servers",
            "./schemas/mcp-server.yaml#/components/schemas/McpServer",
        ),
        "put": {
            **put_one(
                "mcp-servers",
                "./schemas/mcp-server.yaml#/components/schemas/McpServer",
            ),
            "security": SEC_ADMIN,
        },
        "delete": {
            "tags": ["mcp-servers"],
            "security": SEC_ADMIN,
            "parameters": [
                {
                    "name": "name",
                    "in": "path",
                    "required": True,
                    "schema": {"type": "string"},
                },
                {"name": "namespace", "in": "query", "schema": {"type": "string"}},
            ],
            "responses": {
                "200": {
                    "description": "Deleted",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/McpServerDeletedResponse"
                    ),
                },
                "404": {"description": "Not found", "content": text_plain_error()},
                "503": {"description": "Store unavailable", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        },
    }

    wh_body = {
        "required": True,
        "content": {
            "application/json": {
                "schema": {"type": "object", "additionalProperties": True},
            },
            "application/octet-stream": {
                "schema": {"type": "string", "format": "binary"},
            },
        },
    }
    paths["/v1/webhook-deliveries/{endpoint_id}"] = {
        "post": {
            "tags": ["task-webhooks"],
            "summary": "Inbound webhook (verified per TaskWebhook auth profile generic|github|hmac|shared_token)",
            "security": SEC_WRITER,
            "parameters": [
                {
                    "name": "endpoint_id",
                    "in": "path",
                    "required": True,
                    "schema": {"type": "string"},
                }
            ],
            "requestBody": wh_body,
            "responses": {
                "202": {
                    "description": "Accepted",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/WebhookDeliveryResponse"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "401": {
                    "description": "Signature verification failed",
                    "content": text_plain_error(),
                },
                "404": {"description": "Unknown endpoint", "content": text_plain_error()},
                "409": {"description": "Suspended or conflict", "content": text_plain_error()},
                "500": {"description": "Server error", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }

    paths["/v1/events/watch"] = {
        "get": {
            "tags": ["events"],
            "security": SEC_READER,
            "parameters": [
                {"name": "since", "in": "query", "schema": {"type": "string"}},
                {"name": "source", "in": "query", "schema": {"type": "string"}},
                {"name": "type", "in": "query", "schema": {"type": "string"}},
                {"name": "kind", "in": "query", "schema": {"type": "string"}},
                {"name": "name", "in": "query", "schema": {"type": "string"}},
                {"name": "namespace", "in": "query", "schema": {"type": "string"}},
            ],
            "responses": {
                "200": {
                    "description": "SSE; `event: event` with BusEvent JSON",
                    "content": {"text/event-stream": {"schema": {"type": "string"}}},
                },
                "503": {
                    "description": "Event bus unavailable",
                    "content": text_plain_error(),
                },
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }

    paths["/v1/namespaces"] = {
        "get": {
            "tags": ["system"],
            "security": SEC_READER,
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/NamespacesResponse"
                    ),
                },
                "503": {"description": "Store unavailable", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/capabilities"] = {
        "get": {
            "tags": ["system"],
            "security": SEC_READER,
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/CapabilitySnapshot"
                    ),
                },
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }

    # --- A2A Protocol ---

    a2a_card_ref = "./schemas/a2a.yaml#/components/schemas/AgentCard"
    a2a_req_ref = "./schemas/a2a.yaml#/components/schemas/A2AJsonRpcRequest"
    a2a_resp_ref = "./schemas/a2a.yaml#/components/schemas/A2AJsonRpcResponse"
    a2a_registry_ref = "./schemas/a2a.yaml#/components/schemas/A2ARegistryResponse"

    paths["/.well-known/agent-card.json"] = {
        "get": {
            "tags": ["a2a"],
            "summary": "Get the default Agent Card",
            "description": (
                "Returns the A2A Agent Card when exactly one AgentSystem has "
                "spec.a2a.enabled=true. Use the registry or per-system card URLs when "
                "multiple systems are exposed."
            ),
            "security": [],
            "responses": {
                "200": {"description": "OK", "content": json_body(a2a_card_ref)},
                "404": {"description": "No single default AgentSystem available", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    system_card_get = {
        "get": {
            "tags": ["a2a"],
            "summary": "Get Agent Card for a specific AgentSystem",
            "description": (
                "Returns the A2A Agent Card for the named AgentSystem when "
                "spec.a2a.enabled=true."
            ),
            "parameters": [
                {
                    "name": "name",
                    "in": "path",
                    "required": True,
                    "schema": {"type": "string"},
                },
            ],
            "security": [],
            "responses": {
                "200": {"description": "OK", "content": json_body(a2a_card_ref)},
                "404": {"description": "AgentSystem not found or not A2A-enabled", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/agent-systems/{name}/.well-known/agent-card.json"] = system_card_get
    paths["/v1/agents/{name}/.well-known/agent-card.json"] = {
        "get": {
            **system_card_get["get"],
            "summary": "Get Agent Card for a specific AgentSystem (legacy path)",
            "description": "Legacy alias for /v1/agent-systems/{name}/.well-known/agent-card.json.",
        }
    }

    a2a_rpc_post = {
        "tags": ["a2a"],
        "summary": "A2A JSON-RPC endpoint",
        "description": (
            "JSON-RPC 2.0 endpoint implementing the A2A protocol. Supports methods: "
            "tasks/send, tasks/get, tasks/cancel, tasks/sendSubscribe."
        ),
        "security": SEC_A2A,
        "requestBody": {"required": True, "content": json_body(a2a_req_ref)},
        "responses": {
            "200": {"description": "JSON-RPC response", "content": json_body(a2a_resp_ref)},
            "400": {"description": "Bad request", "content": text_plain_error()},
            "default": {"description": "Error", "content": text_plain_error()},
        },
    }
    paths["/a2a"] = {"post": a2a_rpc_post}
    system_a2a_post = {
        "post": {
            **a2a_rpc_post,
            "summary": "A2A JSON-RPC endpoint for a specific AgentSystem",
            "description": (
                "JSON-RPC 2.0 endpoint for the named AgentSystem. The system name in the path "
                "determines routing; no target field is needed in request params."
            ),
            "parameters": [
                {
                    "name": "name",
                    "in": "path",
                    "required": True,
                    "schema": {"type": "string"},
                },
            ],
        }
    }
    paths["/v1/agent-systems/{name}/a2a"] = system_a2a_post
    paths["/v1/agents/{name}/a2a"] = {
        "post": {
            **system_a2a_post["post"],
            "summary": "A2A JSON-RPC endpoint for a specific AgentSystem (legacy path)",
            "description": "Legacy alias for /v1/agent-systems/{name}/a2a.",
        }
    }

    paths["/v1/a2a/agents"] = {
        "get": {
            "tags": ["a2a"],
            "summary": "List A2A agent registry",
            "description": (
                "Returns locally exposed A2A-enabled AgentSystem cards visible to the "
                "bearer token plus configured remote agent entries with cache metadata."
            ),
            "security": SEC_A2A,
            "responses": {
                "200": {"description": "OK", "content": json_body(a2a_registry_ref)},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }

    paths["/healthz"] = {
        "get": {
            "tags": ["system"],
            "security": [],
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/HealthResponse"
                    ),
                }
            },
        }
    }
    paths["/metrics"] = {
        "get": {
            "tags": ["system"],
            "security": SEC_READER,
            "responses": {
                "200": {
                    "description": "Prometheus text exposition",
                    "content": {"text/plain": {"schema": {"type": "string"}}},
                },
                "401": {"description": "Unauthorized", "content": text_plain_error()},
                "403": {"description": "Forbidden", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }

    paths["/v1/auth/config"] = {
        "get": {
            "tags": ["auth"],
            "security": [],
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/AuthConfigResponse"
                    ),
                },
                "500": {"description": "Server error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/auth/setup"] = {
        "post": {
            "tags": ["auth"],
            "security": [],
            "requestBody": {
                "required": True,
                "content": json_body(
                    "./schemas/common.yaml#/components/schemas/AuthCredentialsRequest"
                ),
            },
            "responses": {
                "201": {
                    "description": "Created session",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/AuthMeResponse"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "403": {"description": "Forbidden", "content": text_plain_error()},
                "409": {"description": "Already configured", "content": text_plain_error()},
                "429": {"description": "Rate limited", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/auth/login"] = {
        "post": {
            "tags": ["auth"],
            "security": [],
            "requestBody": {
                "required": True,
                "content": json_body(
                    "./schemas/common.yaml#/components/schemas/AuthCredentialsRequest"
                ),
            },
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/AuthMeResponse"
                    ),
                },
                "401": {"description": "Invalid credentials", "content": text_plain_error()},
                "409": {"description": "Setup required", "content": text_plain_error()},
                "429": {"description": "Rate limited", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/auth/logout"] = {
        "post": {
            "tags": ["auth"],
            "security": [],
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/OkStatusMessage"
                    ),
                },
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/auth/me"] = {
        "get": {
            "tags": ["auth"],
            "security": [],
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/AuthMeResponse"
                    ),
                }
            },
        }
    }
    pwd_status = {
        "type": "object",
        "properties": {"status": {"type": "string"}},
    }
    paths["/v1/auth/change-password"] = {
        "post": {
            "tags": ["auth"],
            "security": SEC_READER,
            "requestBody": {
                "required": True,
                "content": json_body(
                    "./schemas/common.yaml#/components/schemas/AuthChangePasswordRequest"
                ),
            },
            "responses": {
                "200": {
                    "description": "OK",
                    "content": {"application/json": {"schema": pwd_status}},
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "401": {"description": "Unauthorized", "content": text_plain_error()},
                "429": {"description": "Rate limited", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/auth/admin/reset-password"] = {
        "post": {
            "tags": ["auth"],
            "security": SEC_ADMIN,
            "requestBody": {
                "required": True,
                "content": json_body(
                    "./schemas/common.yaml#/components/schemas/AuthResetPasswordRequest"
                ),
            },
            "responses": {
                "200": {
                    "description": "OK",
                    "content": {"application/json": {"schema": pwd_status}},
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "403": {"description": "Forbidden", "content": text_plain_error()},
                "429": {"description": "Rate limited", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/auth/users"] = {
        "get": {
            "tags": ["auth"],
            "security": SEC_ADMIN,
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/AuthUserListResponse"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "401": {"description": "Unauthorized", "content": text_plain_error()},
                "403": {"description": "Forbidden", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        },
        "post": {
            "tags": ["auth"],
            "security": SEC_ADMIN,
            "requestBody": {
                "required": True,
                "content": json_body(
                    "./schemas/common.yaml#/components/schemas/AuthUserCreateRequest"
                ),
            },
            "responses": {
                "201": {
                    "description": "Created",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/AuthUserCreateResponse"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "401": {"description": "Unauthorized", "content": text_plain_error()},
                "403": {"description": "Forbidden", "content": text_plain_error()},
                "409": {"description": "Conflict", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        },
    }
    paths["/v1/auth/users/{username}"] = {
        "delete": {
            "tags": ["auth"],
            "security": SEC_ADMIN,
            "parameters": [
                {
                    "name": "username",
                    "in": "path",
                    "required": True,
                    "schema": {"type": "string"},
                }
            ],
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/OkStatusMessage"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "401": {"description": "Unauthorized", "content": text_plain_error()},
                "403": {"description": "Forbidden", "content": text_plain_error()},
                "404": {"description": "Not found", "content": text_plain_error()},
                "409": {"description": "Conflict", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }
    paths["/v1/tokens"] = {
        "get": {
            "tags": ["auth"],
            "security": SEC_ADMIN,
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/TokenListResponse"
                    ),
                },
                "401": {"description": "Unauthorized", "content": text_plain_error()},
                "403": {"description": "Forbidden", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        },
        "post": {
            "tags": ["auth"],
            "security": SEC_ADMIN,
            "requestBody": {
                "required": True,
                "content": json_body(
                    "./schemas/common.yaml#/components/schemas/TokenCreateRequest"
                ),
            },
            "responses": {
                "201": {
                    "description": "Created",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/TokenCreateResponse"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "401": {"description": "Unauthorized", "content": text_plain_error()},
                "403": {"description": "Forbidden", "content": text_plain_error()},
                "409": {"description": "Conflict", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        },
    }
    paths["/v1/tokens/{name}"] = {
        "delete": {
            "tags": ["auth"],
            "security": SEC_ADMIN,
            "parameters": [
                {
                    "name": "name",
                    "in": "path",
                    "required": True,
                    "schema": {"type": "string"},
                }
            ],
            "responses": {
                "200": {
                    "description": "OK",
                    "content": json_body(
                        "./schemas/common.yaml#/components/schemas/OkStatusMessage"
                    ),
                },
                "400": {"description": "Bad request", "content": text_plain_error()},
                "401": {"description": "Unauthorized", "content": text_plain_error()},
                "403": {"description": "Forbidden", "content": text_plain_error()},
                "404": {"description": "Not found", "content": text_plain_error()},
                "default": {"description": "Error", "content": text_plain_error()},
            },
        }
    }

    return {
        "openapi": "3.1.0",
        "servers": [
            {
                "url": "/",
                "description": "Server root (set host/port when calling the API)",
            }
        ],
        "info": {
            "title": "Orloj API",
            "version": "1.0.0",
            "license": {"name": "Apache-2.0", "identifier": "Apache-2.0"},
            "description": (
                "Control plane HTTP API for Orloj (v1).\n\n"
                "Most error responses use `text/plain` bodies. A future release may "
                "standardize errors as JSON for clients and tooling.\n\n"
                "Authentication: bearer token (`Authorization: Bearer ...`) and/or "
                "session cookie (`orloj_session`, or `__Host-orloj_session` over HTTPS). "
                "Effective requirements depend on server configuration "
                "(`ORLOJ_API_TOKEN` / `ORLOJ_API_TOKENS`, native auth mode, etc.).\n\n"
                "Namespaced PUT behavior: for supported replacement-style "
                "`/v1/<resource>/{name}` endpoints, request-body `metadata.name` "
                "is authoritative. When it differs from the path, the server "
                "renames the stored resource within the same namespace "
                "(HTTP 409 if the destination name already exists)."
            ),
        },
        "tags": [
            {"name": "agents"},
            {"name": "agent-systems"},
            {"name": "model-endpoints"},
            {"name": "tools"},
            {
                "name": "secrets",
                "description": (
                    "GET responses redact `spec.data` entry values as the literal string `***` and "
                    "omit `spec.stringData`. PUT merges `***` placeholders for existing keys "
                    "(from `spec.data`, and from `spec.stringData` when present) so clients can "
                    "rename or edit metadata without re-entering unchanged secrets; new keys still "
                    "require real base64 (`spec.data`) or plaintext (`spec.stringData`). "
                    "PUT may rename the secret when `metadata.name` differs from the path (HTTP 409 if the target name exists)."
                ),
            },
            {"name": "sealed-secrets"},
            {"name": "memories"},
            {"name": "agent-policies"},
            {"name": "agent-roles"},
            {"name": "tool-permissions"},
            {"name": "tool-approvals"},
            {"name": "task-approvals"},
            {"name": "tasks"},
            {"name": "task-schedules"},
            {"name": "task-webhooks"},
            {"name": "workers"},
            {"name": "mcp-servers"},
            {"name": "a2a", "description": (
                "Agent-to-Agent (A2A) protocol endpoints. Provides Agent Card discovery, "
                "JSON-RPC task operations, and a registry of local and remote A2A agents."
            )},
            {"name": "auth"},
            {"name": "system"},
            {"name": "events"},
        ],
        "paths": paths,
        "components": {
            "securitySchemes": {
                "bearerAuth": {"type": "http", "scheme": "bearer"},
                "sessionCookie": {
                    "type": "apiKey",
                    "in": "cookie",
                    "name": "orloj_session",
                    "description": "Session cookie; `__Host-orloj_session` is used for secure sites.",
                },
            }
        },
    }


def main() -> None:
    root = pathlib.Path(__file__).resolve().parent
    doc = build()
    tmp = root / "openapi.tmp.json"
    tmp.write_text(json.dumps(doc, indent=2))
    yaml_path = root / "openapi.yaml"
    subprocess.run(
        [
            "ruby",
            "-rjson",
            "-ryaml",
            "-e",
            "File.write(ARGV[1], YAML.dump(JSON.parse(File.read(ARGV[0]))))",
            str(tmp),
            str(yaml_path),
        ],
        check=True,
    )
    tmp.unlink()
    print(f"Wrote {yaml_path}")


if __name__ == "__main__":
    main()
