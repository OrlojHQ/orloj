# Quickstart

This quickstart follows the production-style flow: start services, apply resources, and verify runtime behavior.

## Before You Begin

- Go `1.24+` is installed.
- You are in repository root.
- Two terminals are available (control plane and worker).

## 1. Start the Control Plane

```bash
go run ./cmd/orlojd \
  --storage-backend=memory \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory
```

## 2. Start a Worker

```bash
go run ./cmd/orlojworker \
  --storage-backend=memory \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory \
  --agent-message-consume \
  --model-gateway-provider=mock
```

## 3. Apply a Starter Blueprint

```bash
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/
```

## 4. Verify Execution

```bash
go run ./cmd/orlojctl get task bp-pipeline-task
curl -s http://127.0.0.1:8080/v1/tasks/bp-pipeline-task/messages?namespace=default | jq .
curl -s http://127.0.0.1:8080/v1/tasks/bp-pipeline-task/metrics?namespace=default | jq .
```

Expected result:

- Task reaches `Succeeded`.
- Message lifecycle and per-edge metrics are present.

## Next Steps

- [Deployment Overview](../deployment/index.md)
- [VPS Deployment](../deployment/vps.md)
- [Kubernetes Deployment](../deployment/kubernetes.md)
- [Production Checklist](./production-checklist.md)
- [Starter Blueprints](../architecture/starter-blueprints.md)
- [Execution and Messaging](../architecture/execution-model.md)
