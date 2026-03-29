SHELL := /bin/zsh

# Canonical CLI command variable; AGENTCTL is a deprecated compatibility alias.
ORLOJCTL ?=
ifeq ($(strip $(ORLOJCTL)),)
ifneq ($(strip $(AGENTCTL)),)
ORLOJCTL := $(AGENTCTL)
else
ORLOJCTL := go run ./cmd/orlojctl
endif
endif
AGENTCTL ?= $(ORLOJCTL)
API_BASE ?= http://localhost:8080
SCENARIOS_REAL_DIR ?= testing/scenarios-real
REAL_ARTIFACTS_DIR ?= testing/artifacts/real

REAL_TOOL_STUB_ADDR ?= 127.0.0.1:18080
REAL_TOOL_STUB_BASE ?= http://127.0.0.1:18080
REAL_TOOL_STUB_CONTAINER_BASE ?= http://host.docker.internal:18080
REAL_WEBHOOK_SECRET ?= rr-webhook-secret

PIPELINE_SCENARIO := 01-pipeline
HIER_SCENARIO := 02-hierarchical
LOOP_SCENARIO := 03-loop-max-turns
TOOL_SCENARIO := 04-tool-call-smoke
DECISION_SCENARIO := 05-tool-decision
MEMORY_SHARED_SCENARIO := 06-memory-shared-handoff
MEMORY_REUSE_SCENARIO := 07-memory-persistent-reuse
TOOL_AUTH_SCENARIO := 08-tool-auth-and-contract
GOV_DENY_SCENARIO := 09-governance-real-deny
TOOL_RETRY_SCENARIO := 10-tool-retry-recovery
WEBHOOK_SCENARIO := 11-webhook-live-flow
SCHEDULE_SCENARIO := 12-schedule-live-flow
MCP_SCENARIO := 14-mcp-tool-smoke
HIER_TOOL_SCENARIO := 15-hierarchical-incident-tools
KITCHEN_SCENARIO := 16-kitchen-sink
APPROVAL_SCENARIO := 17-tool-approval-live

PIPELINE_NS := rr-real-pipeline
HIER_NS := rr-real-hier
LOOP_NS := rr-real-loop
TOOL_NS := rr-real-tool
DECISION_NS := rr-real-tool-decision
MEMORY_SHARED_NS := rr-real-memory-shared
MEMORY_REUSE_NS := rr-real-memory-reuse
TOOL_AUTH_NS := rr-real-tool-auth
GOV_DENY_NS := rr-real-gov-deny
TOOL_RETRY_NS := rr-real-tool-retry
WEBHOOK_NS := rr-real-webhook
SCHEDULE_NS := rr-real-schedule
MCP_NS := rr-real-mcp
HIER_TOOL_NS := rr-real-hier-tool
KITCHEN_NS := rr-real-kitchen
APPROVAL_NS := rr-real-tool-approval

PIPELINE_TASK := rr-real-pipeline-task
HIER_TASK := rr-real-hier-task
LOOP_TASK := rr-real-loop-task
TOOL_TASK := rr-real-tool-task
DECISION_USE_TASK := rr-tool-use-task
DECISION_NO_USE_TASK := rr-tool-no-use-task
MEMORY_SHARED_TASK := rr-real-memory-shared-task
MEMORY_REUSE_SEED_TASK := rr-real-memory-seed-task
MEMORY_REUSE_QUERY_TASK := rr-real-memory-query-task
TOOL_AUTH_TASK := rr-real-tool-auth-task
GOV_DENY_TASK := rr-real-gov-deny-task
TOOL_RETRY_TASK := rr-real-tool-retry-task

MEMORY_SHARED_NAME := rr-real-memory-shared-store
MEMORY_REUSE_NAME := rr-real-memory-reuse-store
WEBHOOK_MEMORY_NAME := rr-real-webhook-memory
SCHEDULE_MEMORY_NAME := rr-real-schedule-memory

MCP_TASK := rr-real-mcp-task
HIER_TOOL_TASK := rr-real-hier-tool-task
KITCHEN_PRIMARY_TASK := rr-real-kitchen-primary-task
KITCHEN_MEMORY_SEED_TASK := rr-real-kitchen-memory-seed-task
KITCHEN_MEMORY_QUERY_TASK := rr-real-kitchen-memory-query-task
KITCHEN_WEBHOOK_TEMPLATE_TASK := rr-real-kitchen-webhook-template-task
KITCHEN_SCHEDULE_TEMPLATE_TASK := rr-real-kitchen-schedule-template-task
MCP_SERVER_NAME := rr-real-mcp-everything
KITCHEN_MCP_SERVER_NAME := rr-real-kitchen-mcp

WEBHOOK_NAME := rr-real-webhook-ingest
SCHEDULE_NAME := rr-real-minute-digest
KITCHEN_WEBHOOK_NAME := rr-real-kitchen-webhook-ingest
KITCHEN_SCHEDULE_NAME := rr-real-kitchen-minute-digest

APPROVAL_TASK := rr-real-approval-task

KITCHEN_WEBHOOK_MEMORY := rr-real-kitchen-webhook-memory
KITCHEN_SCHEDULE_MEMORY := rr-real-kitchen-schedule-memory
KITCHEN_REUSE_MEMORY := rr-real-kitchen-reuse-store

REAL_GATE_TIMEOUT_SECONDS ?= 240
REAL_KITCHEN_GATE_TIMEOUT_SECONDS ?= 480
REAL_APPROVAL_GATE_TIMEOUT_SECONDS ?= 240
REAL_GATE_POLL_INTERVAL_SECONDS ?= 2
REAL_SCHEDULE_TIMEOUT_SECONDS ?= 120

.PHONY: build help ui-install ui-dev ui-build \
	real-help real-tool-stub real-repeat \
	real-delete-task real-delete-tool-approvals-in-ns real-capture \
	real-apply real-apply-all \
	real-apply-pipeline real-apply-hier real-apply-loop real-apply-tool real-apply-tool-decision real-apply-anthropic-tool-decision \
	real-apply-memory-shared real-apply-memory-reuse real-apply-memory-reuse-query \
	real-apply-tool-auth real-apply-governance-deny real-apply-tool-retry \
	real-apply-webhook real-apply-schedule real-apply-mcp real-apply-hier-tool \
	real-apply-kitchen real-apply-kitchen-memory-seed real-apply-kitchen-memory-query \
	real-apply-tool-approval \
	real-get real-messages real-metrics real-check \
	real-check-pipeline real-check-hier real-check-loop real-check-tool real-check-hier-tool \
	real-check-tool-use real-check-tool-no-use real-check-anthropic-tool-use real-check-anthropic-tool-no-use \
	real-check-memory-shared real-check-memory-reuse real-check-tool-auth \
	real-check-governance-deny real-check-tool-retry real-check-webhook real-check-schedule \
	real-check-kitchen real-check-tool-approval \
	real-wait-task-succeeded real-wait-task-terminal real-wait-task-phase \
	real-gate-pipeline real-gate-hier real-gate-loop real-gate-tool \
	real-gate-tool-decision real-gate-anthropic-tool-decision real-gate-memory-shared real-gate-memory-reuse \
	real-gate-tool-auth real-gate-governance-deny real-gate-tool-retry \
	real-gate-webhook real-gate-schedule real-gate-mcp real-gate-hier-tool \
	real-gate-kitchen real-gate-tool-approval real-gate-tool-approval-ci \
	real-gate-wave0 real-gate-wave1 real-gate-wave2 real-gate-wave3 real-gate-wave4 real-gate-wave5 \
	real-check-all

build:
	go build ./cmd/...

help: real-help

ui-install:
	cd frontend && bun install

ui-dev:
	cd frontend && bun run dev

ui-build:
	cd frontend && bun run build

real-help:
	@echo "Build:"
	@echo "  make build        # compile all binaries (go build ./cmd/...)"
	@echo ""
	@echo "UI:"
	@echo "  make ui-install   # install frontend dependencies"
	@echo "  make ui-dev       # run React dev server on http://127.0.0.1:5173"
	@echo "  make ui-build     # build frontend into frontend/dist for Go embedding"
	@echo ""
	@echo "Live-validation prerequisites:"
	@echo "  1. Terminal 1 (control plane):"
	@echo "     go run ./cmd/orlojd --task-execution-mode=message-driven --agent-message-bus-backend=memory"
	@echo "  2. Terminal 2 (Anthropic worker for model-only scenarios):"
	@echo "     go run ./cmd/orlojworker --task-execution-mode=message-driven --agent-message-bus-backend=memory --agent-message-consume --model-gateway-provider=anthropic"
	@echo "  3. Terminal 2 for tool-backed Anthropic scenarios:"
	@echo "     go run ./cmd/orlojworker --task-execution-mode=message-driven --agent-message-bus-backend=memory --agent-message-consume --model-gateway-provider=anthropic --tool-isolation-backend=container --tool-container-network=bridge"
	@echo "  4. Terminal 3 (deterministic local stub tool service):"
	@echo "     make real-tool-stub"
	@echo ""
	@echo "Wave 0 apply targets:"
	@echo "  make real-apply-pipeline"
	@echo "  make real-apply-hier"
	@echo "  make real-apply-loop"
	@echo "  make real-apply-tool"
	@echo "  make real-apply-tool-decision"
	@echo "  make real-apply-hier-tool"
	@echo ""
	@echo "Wave 1+ apply targets:"
	@echo "  make real-apply-memory-shared"
	@echo "  make real-apply-memory-reuse"
	@echo "  make real-apply-tool-auth"
	@echo "  make real-apply-governance-deny"
	@echo "  make real-apply-tool-retry"
	@echo "  make real-apply-webhook"
	@echo "  make real-apply-schedule"
	@echo "  make real-apply-mcp"
	@echo "  make real-apply-kitchen"
	@echo "  make real-apply-tool-approval"
	@echo ""
	@echo "Scenario gates:"
	@echo "  make real-gate-pipeline"
	@echo "  make real-gate-hier"
	@echo "  make real-gate-loop"
	@echo "  make real-gate-tool"
	@echo "  make real-gate-tool-decision"
	@echo "  make real-gate-memory-shared"
	@echo "  make real-gate-memory-reuse"
	@echo "  make real-gate-tool-auth"
	@echo "  make real-gate-governance-deny"
	@echo "  make real-gate-tool-retry"
	@echo "  make real-gate-webhook"
	@echo "  make real-gate-schedule"
	@echo "  make real-gate-mcp"
	@echo "  make real-gate-hier-tool"
	@echo "  make real-gate-kitchen"
	@echo "  make real-gate-tool-approval       # waits for you to approve in UI (or curl)"
	@echo "  make real-gate-tool-approval-ci    # same but POSTs /approve (for CI / wave5)"
	@echo ""
	@echo "Grouped gates:"
	@echo "  make real-gate-wave0"
	@echo "  make real-gate-wave1"
	@echo "  make real-gate-wave2"
	@echo "  make real-gate-wave3"
	@echo "  make real-gate-wave4"
	@echo "  make real-gate-wave5   # kitchen sink + tool approval (long soak)"
	@echo ""
	@echo "Repeated runs:"
	@echo "  make real-repeat TARGET=real-gate-pipeline COUNT=3"
	@echo "  make real-repeat TARGET=real-gate-governance-deny COUNT=5"
	@echo ""
	@echo "Artifacts:"
	@echo "  make real-capture NS=$(PIPELINE_NS) TASK=$(PIPELINE_TASK) VERDICT=passed"
	@echo "  output root: $(REAL_ARTIFACTS_DIR)"
	@echo ""
	@echo "Tool stub defaults:"
	@echo "  host endpoint: $(REAL_TOOL_STUB_BASE)"
	@echo "  container endpoint: $(REAL_TOOL_STUB_CONTAINER_BASE)"

real-tool-stub:
	go run ./testing/stubs/live_tool_stub --listen="$(REAL_TOOL_STUB_ADDR)"

real-repeat:
	@if [ -z "$(TARGET)" ] || [ -z "$(COUNT)" ]; then \
		echo "TARGET and COUNT are required. Example: make real-repeat TARGET=real-gate-pipeline COUNT=3"; \
		exit 1; \
	fi
	@set -eu; \
	for run in $$(seq 1 $(COUNT)); do \
		echo "==> $$run/$(COUNT): $(TARGET)"; \
		$(MAKE) $(TARGET); \
	done

real-delete-task:
	@if [ -z "$(NS)" ] || [ -z "$(TASK)" ]; then \
		echo "NS and TASK are required. Example: make real-delete-task NS=$(PIPELINE_NS) TASK=$(PIPELINE_TASK)"; \
		exit 1; \
	fi
	@set -eu; \
	http_code=$$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$(API_BASE)/v1/tasks/$(TASK)?namespace=$(NS)"); \
	case "$$http_code" in \
		204|404) ;; \
		*) echo "unexpected delete status=$$http_code for task $(NS)/$(TASK)"; exit 1 ;; \
	esac

real-capture:
	@if [ -z "$(NS)" ] || [ -z "$(TASK)" ] || [ -z "$(VERDICT)" ]; then \
		echo "NS, TASK, and VERDICT are required. Example: make real-capture NS=$(PIPELINE_NS) TASK=$(PIPELINE_TASK) VERDICT=passed"; \
		exit 1; \
	fi
	@API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(NS)" "$(TASK)" "$(VERDICT)" $(MEMORIES)

real-apply:
	@if [ -z "$(SCENARIO)" ]; then \
		echo "SCENARIO is required. Example: make real-apply SCENARIO=$(PIPELINE_SCENARIO)"; \
		exit 1; \
	fi
	@if [ ! -d "$(SCENARIOS_REAL_DIR)/$(SCENARIO)" ]; then \
		echo "Scenario directory not found: $(SCENARIOS_REAL_DIR)/$(SCENARIO)"; \
		exit 1; \
	fi
	@set -eu; \
	if command -v rg >/dev/null 2>&1; then \
		if rg -n 'value:[[:space:]]*replace-me' "$(SCENARIOS_REAL_DIR)/$(SCENARIO)" >/dev/null; then \
			echo "Secret placeholder detected in $(SCENARIOS_REAL_DIR)/$(SCENARIO). Replace spec.stringData.value first."; \
			exit 1; \
		fi; \
	else \
		if grep -RIn 'value:[[:space:]]*replace-me' "$(SCENARIOS_REAL_DIR)/$(SCENARIO)" >/dev/null; then \
			echo "Secret placeholder detected in $(SCENARIOS_REAL_DIR)/$(SCENARIO). Replace spec.stringData.value first."; \
			exit 1; \
		fi; \
	fi
	@set -eu; \
	non_task_files=$$(find "$(SCENARIOS_REAL_DIR)/$(SCENARIO)" -name '*.yaml' ! -name 'task*.yaml' -print | sort); \
	if [ -n "$$non_task_files" ]; then \
		printf '%s\n' "$$non_task_files" | while IFS= read -r file; do \
			[ -n "$$file" ] || continue; \
			$(ORLOJCTL) apply -f "$$file"; \
		done; \
	fi; \
	task_files=$$(find "$(SCENARIOS_REAL_DIR)/$(SCENARIO)" -name 'task*.yaml' -print | sort); \
	if [ -n "$$task_files" ]; then \
		printf '%s\n' "$$task_files" | while IFS= read -r file; do \
			[ -n "$$file" ] || continue; \
			$(ORLOJCTL) apply -f "$$file"; \
		done; \
	fi

real-apply-pipeline:
	@$(MAKE) real-delete-task NS=$(PIPELINE_NS) TASK=$(PIPELINE_TASK)
	@$(MAKE) real-apply SCENARIO=$(PIPELINE_SCENARIO)

real-apply-hier:

	@$(MAKE) real-apply SCENARIO=$(HIER_SCENARIO)

real-apply-loop:
	@$(MAKE) real-delete-task NS=$(LOOP_NS) TASK=$(LOOP_TASK)
	@$(MAKE) real-apply SCENARIO=$(LOOP_SCENARIO)

real-apply-tool:
	@$(MAKE) real-delete-task NS=$(TOOL_NS) TASK=$(TOOL_TASK)
	@$(MAKE) real-apply SCENARIO=$(TOOL_SCENARIO)

real-apply-hier-tool:
	@$(MAKE) real-delete-task NS=$(HIER_TOOL_NS) TASK=$(HIER_TOOL_TASK)
	@$(MAKE) real-apply SCENARIO=$(HIER_TOOL_SCENARIO)

real-apply-kitchen:
	@set -eu; \
	if command -v rg >/dev/null 2>&1; then \
		if rg -n 'value:[[:space:]]*replace-me' "$(SCENARIOS_REAL_DIR)/$(KITCHEN_SCENARIO)" >/dev/null; then \
			echo "Secret placeholder detected in $(SCENARIOS_REAL_DIR)/$(KITCHEN_SCENARIO). Replace spec.stringData.value first."; \
			exit 1; \
		fi; \
	else \
		if grep -RIn 'value:[[:space:]]*replace-me' "$(SCENARIOS_REAL_DIR)/$(KITCHEN_SCENARIO)" >/dev/null; then \
			echo "Secret placeholder detected in $(SCENARIOS_REAL_DIR)/$(KITCHEN_SCENARIO). Replace spec.stringData.value first."; \
			exit 1; \
		fi; \
	fi
	@set -eu; \
	curl -s -o /dev/null -w "%{http_code}" -X DELETE "$(API_BASE)/v1/task-webhooks/$(KITCHEN_WEBHOOK_NAME)?namespace=$(KITCHEN_NS)" >/dev/null || true
	@set -eu; \
	curl -s -o /dev/null -w "%{http_code}" -X DELETE "$(API_BASE)/v1/task-schedules/$(KITCHEN_SCHEDULE_NAME)?namespace=$(KITCHEN_NS)" >/dev/null || true
	@$(MAKE) real-delete-task NS=$(KITCHEN_NS) TASK=$(KITCHEN_PRIMARY_TASK)
	@$(MAKE) real-delete-task NS=$(KITCHEN_NS) TASK=$(KITCHEN_MEMORY_SEED_TASK)
	@$(MAKE) real-delete-task NS=$(KITCHEN_NS) TASK=$(KITCHEN_MEMORY_QUERY_TASK)
	@$(MAKE) real-delete-task NS=$(KITCHEN_NS) TASK=$(KITCHEN_WEBHOOK_TEMPLATE_TASK)
	@$(MAKE) real-delete-task NS=$(KITCHEN_NS) TASK=$(KITCHEN_SCHEDULE_TEMPLATE_TASK)
	@set -eu; \
	find "$(SCENARIOS_REAL_DIR)/$(KITCHEN_SCENARIO)" -name '*.yaml' ! -name 'task*.yaml' -print | sort | while IFS= read -r file; do \
		[ -n "$$file" ] || continue; \
		$(ORLOJCTL) apply -f "$$file"; \
	done; \
	echo "waiting for kitchen MCP server to discover tools..."; \
	deadline=$$(( $$(date +%s) + 90 )); \
	while true; do \
		mcp_phase=$$(curl -sf "$(API_BASE)/v1/mcp-servers/$(KITCHEN_MCP_SERVER_NAME)?namespace=$(KITCHEN_NS)" | jq -r '.status.phase // ""' 2>/dev/null || echo ""); \
		if [ "$$mcp_phase" = "Ready" ]; then \
			echo "kitchen MCP server ready"; \
			break; \
		fi; \
		if [ $$(date +%s) -gt $$deadline ]; then \
			echo "timeout waiting for kitchen MCP server Ready (phase=$$mcp_phase)"; \
			exit 1; \
		fi; \
		sleep 2; \
	done; \
	find "$(SCENARIOS_REAL_DIR)/$(KITCHEN_SCENARIO)" -name 'task*.yaml' ! -name 'task_memory_seed.yaml' ! -name 'task_memory_query.yaml' -print | sort | while IFS= read -r file; do \
		[ -n "$$file" ] || continue; \
		$(ORLOJCTL) apply -f "$$file"; \
	done

real-apply-kitchen-memory-seed:
	@$(MAKE) real-delete-task NS=$(KITCHEN_NS) TASK=$(KITCHEN_MEMORY_SEED_TASK)
	@$(ORLOJCTL) apply -f "$(SCENARIOS_REAL_DIR)/$(KITCHEN_SCENARIO)/task_memory_seed.yaml"

real-apply-kitchen-memory-query:
	@$(MAKE) real-delete-task NS=$(KITCHEN_NS) TASK=$(KITCHEN_MEMORY_QUERY_TASK)
	@$(ORLOJCTL) apply -f "$(SCENARIOS_REAL_DIR)/$(KITCHEN_SCENARIO)/task_memory_query.yaml"

# Remove all ToolApproval rows in a namespace so UI/lists are not confused by prior gate runs (gate POSTs /approve).
real-delete-tool-approvals-in-ns:
	@set -eu; \
	ns="$(TOOL_APPROVAL_CLEANUP_NS)"; \
	[ -n "$$ns" ] || { echo "TOOL_APPROVAL_CLEANUP_NS is required"; exit 1; }; \
	json=$$(curl -sf "$(API_BASE)/v1/tool-approvals?namespace=$$ns" || echo '{"items":[]}'); \
	printf '%s\n' "$$json" | jq -r '.items[]? | .metadata.name // empty' | while IFS= read -r n; do \
		[ -n "$$n" ] || continue; \
		curl -sf -X DELETE "$(API_BASE)/v1/tool-approvals/$$n?namespace=$$ns" >/dev/null || true; \
	done

real-apply-tool-approval:
	@$(MAKE) real-delete-task NS=$(APPROVAL_NS) TASK=$(APPROVAL_TASK)
	@$(MAKE) real-delete-tool-approvals-in-ns TOOL_APPROVAL_CLEANUP_NS=$(APPROVAL_NS)
	@$(MAKE) real-apply SCENARIO=$(APPROVAL_SCENARIO)

real-apply-tool-decision:
	@$(MAKE) real-delete-task NS=$(DECISION_NS) TASK=$(DECISION_USE_TASK)
	@$(MAKE) real-delete-task NS=$(DECISION_NS) TASK=$(DECISION_NO_USE_TASK)
	@$(MAKE) real-apply SCENARIO=$(DECISION_SCENARIO)

real-apply-anthropic-tool-decision: real-apply-tool-decision

real-apply-memory-shared:
	@$(MAKE) real-delete-task NS=$(MEMORY_SHARED_NS) TASK=$(MEMORY_SHARED_TASK)
	@$(MAKE) real-apply SCENARIO=$(MEMORY_SHARED_SCENARIO)

real-apply-memory-reuse:
	@$(MAKE) real-delete-task NS=$(MEMORY_REUSE_NS) TASK=$(MEMORY_REUSE_SEED_TASK)
	@$(MAKE) real-delete-task NS=$(MEMORY_REUSE_NS) TASK=$(MEMORY_REUSE_QUERY_TASK)
	@set -eu; \
	if command -v rg >/dev/null 2>&1; then \
		if rg -n 'value:[[:space:]]*replace-me' "$(SCENARIOS_REAL_DIR)/$(MEMORY_REUSE_SCENARIO)" >/dev/null; then \
			echo "Secret placeholder detected in $(SCENARIOS_REAL_DIR)/$(MEMORY_REUSE_SCENARIO). Replace spec.stringData.value first."; \
			exit 1; \
		fi; \
	else \
		if grep -RIn 'value:[[:space:]]*replace-me' "$(SCENARIOS_REAL_DIR)/$(MEMORY_REUSE_SCENARIO)" >/dev/null; then \
			echo "Secret placeholder detected in $(SCENARIOS_REAL_DIR)/$(MEMORY_REUSE_SCENARIO). Replace spec.stringData.value first."; \
			exit 1; \
		fi; \
	fi
	@find "$(SCENARIOS_REAL_DIR)/$(MEMORY_REUSE_SCENARIO)" -name '*.yaml' ! -name 'task_query.yaml' -print | sort | xargs -I{} $(ORLOJCTL) apply -f {}

real-apply-memory-reuse-query:
	@$(MAKE) real-delete-task NS=$(MEMORY_REUSE_NS) TASK=$(MEMORY_REUSE_QUERY_TASK)
	@$(ORLOJCTL) apply -f "$(SCENARIOS_REAL_DIR)/$(MEMORY_REUSE_SCENARIO)/task_query.yaml"

real-apply-tool-auth:
	@$(MAKE) real-delete-task NS=$(TOOL_AUTH_NS) TASK=$(TOOL_AUTH_TASK)
	@$(MAKE) real-apply SCENARIO=$(TOOL_AUTH_SCENARIO)

real-apply-governance-deny:
	@$(MAKE) real-delete-task NS=$(GOV_DENY_NS) TASK=$(GOV_DENY_TASK)
	@$(MAKE) real-apply SCENARIO=$(GOV_DENY_SCENARIO)

real-apply-tool-retry:
	@$(MAKE) real-delete-task NS=$(TOOL_RETRY_NS) TASK=$(TOOL_RETRY_TASK)
	@$(MAKE) real-apply SCENARIO=$(TOOL_RETRY_SCENARIO)

real-apply-webhook:
	@set -eu; \
	curl -s -o /dev/null -w "%{http_code}" -X DELETE "$(API_BASE)/v1/task-webhooks/$(WEBHOOK_NAME)?namespace=$(WEBHOOK_NS)" >/dev/null || true
	@$(MAKE) real-apply SCENARIO=$(WEBHOOK_SCENARIO)

real-apply-schedule:
	@set -eu; \
	curl -s -o /dev/null -w "%{http_code}" -X DELETE "$(API_BASE)/v1/task-schedules/$(SCHEDULE_NAME)?namespace=$(SCHEDULE_NS)" >/dev/null || true
	@$(MAKE) real-apply SCENARIO=$(SCHEDULE_SCENARIO)

real-apply-mcp:
	@$(MAKE) real-delete-task NS=$(MCP_NS) TASK=$(MCP_TASK)
	@set -eu; \
	find "$(SCENARIOS_REAL_DIR)/$(MCP_SCENARIO)" -name '*.yaml' ! -name 'task*.yaml' -print | sort | while IFS= read -r file; do \
		[ -n "$$file" ] || continue; \
		$(ORLOJCTL) apply -f "$$file"; \
	done; \
	echo "waiting for MCP server to discover tools..."; \
	deadline=$$(( $$(date +%s) + 60 )); \
	while true; do \
		mcp_phase=$$(curl -sf "$(API_BASE)/v1/mcp-servers/$(MCP_SERVER_NAME)?namespace=$(MCP_NS)" | jq -r '.status.phase // ""' 2>/dev/null || echo ""); \
		if [ "$$mcp_phase" = "Ready" ]; then \
			echo "MCP server ready, tools generated"; \
			break; \
		fi; \
		if [ $$(date +%s) -gt $$deadline ]; then \
			echo "timeout waiting for MCP server to become Ready (phase=$$mcp_phase)"; \
			exit 1; \
		fi; \
		sleep 2; \
	done; \
	find "$(SCENARIOS_REAL_DIR)/$(MCP_SCENARIO)" -name 'task*.yaml' -print | sort | while IFS= read -r file; do \
		[ -n "$$file" ] || continue; \
		$(ORLOJCTL) apply -f "$$file"; \
	done

real-apply-all: \
	real-apply-pipeline real-apply-hier real-apply-loop real-apply-tool real-apply-tool-decision \
	real-apply-hier-tool \
	real-apply-memory-shared real-apply-memory-reuse real-apply-tool-auth real-apply-governance-deny \
	real-apply-tool-retry real-apply-webhook real-apply-schedule real-apply-mcp

real-get:
	@if [ -z "$(NS)" ] || [ -z "$(TASK)" ]; then \
		echo "NS and TASK are required. Example: make real-get NS=$(PIPELINE_NS) TASK=$(PIPELINE_TASK)"; \
		exit 1; \
	fi
	@$(ORLOJCTL) get task "$(TASK)" --namespace "$(NS)"

real-messages:
	@if [ -z "$(NS)" ] || [ -z "$(TASK)" ]; then \
		echo "NS and TASK are required. Example: make real-messages NS=$(PIPELINE_NS) TASK=$(PIPELINE_TASK)"; \
		exit 1; \
	fi
	@curl -s "$(API_BASE)/v1/tasks/$(TASK)/messages?namespace=$(NS)" | jq .

real-metrics:
	@if [ -z "$(NS)" ] || [ -z "$(TASK)" ]; then \
		echo "NS and TASK are required. Example: make real-metrics NS=$(PIPELINE_NS) TASK=$(PIPELINE_TASK)"; \
		exit 1; \
	fi
	@curl -s "$(API_BASE)/v1/tasks/$(TASK)/metrics?namespace=$(NS)" | jq .

real-check: real-get real-messages real-metrics

real-check-pipeline:
	@$(MAKE) real-check NS=$(PIPELINE_NS) TASK=$(PIPELINE_TASK)

real-check-hier:
	@$(MAKE) real-check NS=$(HIER_NS) TASK=$(HIER_TASK)

real-check-loop:
	@$(MAKE) real-check NS=$(LOOP_NS) TASK=$(LOOP_TASK)

real-check-tool:
	@$(MAKE) real-check NS=$(TOOL_NS) TASK=$(TOOL_TASK)

real-check-hier-tool:
	@$(MAKE) real-check NS=$(HIER_TOOL_NS) TASK=$(HIER_TOOL_TASK)

real-check-memory-shared:
	@$(MAKE) real-check NS=$(MEMORY_SHARED_NS) TASK=$(MEMORY_SHARED_TASK)

real-check-memory-reuse:
	@$(MAKE) real-check NS=$(MEMORY_REUSE_NS) TASK=$(MEMORY_REUSE_SEED_TASK)
	@$(MAKE) real-check NS=$(MEMORY_REUSE_NS) TASK=$(MEMORY_REUSE_QUERY_TASK)

real-check-tool-auth:
	@$(MAKE) real-check NS=$(TOOL_AUTH_NS) TASK=$(TOOL_AUTH_TASK)

real-check-governance-deny:
	@$(MAKE) real-check NS=$(GOV_DENY_NS) TASK=$(GOV_DENY_TASK)

real-check-tool-retry:
	@$(MAKE) real-check NS=$(TOOL_RETRY_NS) TASK=$(TOOL_RETRY_TASK)

real-check-webhook:
	@set -eu; \
	task_scoped=$$(curl -sSf "$(API_BASE)/v1/task-webhooks/$(WEBHOOK_NAME)?namespace=$(WEBHOOK_NS)" | jq -r '.status.lastTriggeredTask // ""'); \
	[ -n "$$task_scoped" ] || { echo "webhook has not created a task yet"; exit 1; }; \
	task_name=$${task_scoped##*/}; \
	$(MAKE) real-check NS=$(WEBHOOK_NS) TASK=$$task_name

real-check-schedule:
	@set -eu; \
	task_scoped=$$(curl -sSf "$(API_BASE)/v1/task-schedules/$(SCHEDULE_NAME)?namespace=$(SCHEDULE_NS)" | jq -r '.status.lastTriggeredTask // ""'); \
	[ -n "$$task_scoped" ] || { echo "schedule has not created a task yet"; exit 1; }; \
	task_name=$${task_scoped##*/}; \
	$(MAKE) real-check NS=$(SCHEDULE_NS) TASK=$$task_name

real-wait-task-succeeded:
	@if [ -z "$(NS)" ] || [ -z "$(TASK)" ]; then \
		echo "NS and TASK are required. Example: make real-wait-task-succeeded NS=$(TOOL_NS) TASK=$(TOOL_TASK)"; \
		exit 1; \
	fi
	@set -eu; \
	timeout="$(REAL_GATE_TIMEOUT_SECONDS)"; \
	interval="$(REAL_GATE_POLL_INTERVAL_SECONDS)"; \
	deadline=$$(( $$(date +%s) + timeout )); \
	last_phase=""; \
	while true; do \
		now=$$(date +%s); \
		if [ $$now -ge $$deadline ]; then \
			echo "timeout waiting for task $(TASK) in namespace $(NS) to succeed (last phase=$$last_phase)"; \
			exit 1; \
		fi; \
		set +e; \
		task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(TASK)?namespace=$(NS)"); \
		curl_rc=$$?; \
		set -e; \
		if [ $$curl_rc -ne 0 ]; then \
			sleep "$$interval"; \
			continue; \
		fi; \
		last_phase=$$(printf '%s\n' "$$task_json" | jq -r '.status.phase // ""'); \
		if [ "$$last_phase" = "Succeeded" ]; then \
			echo "task $(TASK) phase=$$last_phase"; \
			break; \
		fi; \
		if [ "$$last_phase" = "Failed" ] || [ "$$last_phase" = "DeadLetter" ] || [ "$$last_phase" = "Error" ]; then \
			echo "task $(TASK) terminal phase=$$last_phase (expected Succeeded)"; \
			printf '%s\n' "$$task_json" | jq '.'; \
			exit 1; \
		fi; \
		sleep "$$interval"; \
	done

real-wait-task-terminal:
	@if [ -z "$(NS)" ] || [ -z "$(TASK)" ]; then \
		echo "NS and TASK are required. Example: make real-wait-task-terminal NS=$(GOV_DENY_NS) TASK=$(GOV_DENY_TASK)"; \
		exit 1; \
	fi
	@set -eu; \
	timeout="$(REAL_GATE_TIMEOUT_SECONDS)"; \
	interval="$(REAL_GATE_POLL_INTERVAL_SECONDS)"; \
	deadline=$$(( $$(date +%s) + timeout )); \
	while true; do \
		now=$$(date +%s); \
		if [ $$now -ge $$deadline ]; then \
			echo "timeout waiting for task $(TASK) in namespace $(NS) to reach a terminal phase"; \
			exit 1; \
		fi; \
		set +e; \
		task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(TASK)?namespace=$(NS)"); \
		curl_rc=$$?; \
		set -e; \
		if [ $$curl_rc -ne 0 ]; then \
			sleep "$$interval"; \
			continue; \
		fi; \
		phase=$$(printf '%s\n' "$$task_json" | jq -r '.status.phase // ""'); \
		case "$$phase" in \
			Succeeded|Failed|DeadLetter|Error) \
				echo "task $(TASK) phase=$$phase"; \
				break ;; \
		esac; \
		sleep "$$interval"; \
	done

real-wait-task-phase:
	@if [ -z "$(NS)" ] || [ -z "$(TASK)" ] || [ -z "$(PHASE)" ]; then \
		echo "NS, TASK, and PHASE are required. Example: make real-wait-task-phase NS=$(APPROVAL_NS) TASK=$(APPROVAL_TASK) PHASE=WaitingApproval"; \
		exit 1; \
	fi
	@set -eu; \
	timeout="$(REAL_GATE_TIMEOUT_SECONDS)"; \
	interval="$(REAL_GATE_POLL_INTERVAL_SECONDS)"; \
	deadline=$$(( $$(date +%s) + timeout )); \
	want="$(PHASE)"; \
	while true; do \
		now=$$(date +%s); \
		if [ $$now -ge $$deadline ]; then \
			echo "timeout waiting for task $(TASK) in namespace $(NS) to reach phase $$want"; \
			exit 1; \
		fi; \
		set +e; \
		task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(TASK)?namespace=$(NS)"); \
		curl_rc=$$?; \
		set -e; \
		if [ $$curl_rc -ne 0 ]; then \
			sleep "$$interval"; \
			continue; \
		fi; \
		phase=$$(printf '%s\n' "$$task_json" | jq -r '.status.phase // ""'); \
		if [ "$$phase" = "$$want" ]; then \
			echo "task $(TASK) phase=$$phase"; \
			break; \
		fi; \
		if [ "$$phase" = "Failed" ] || [ "$$phase" = "DeadLetter" ] || [ "$$phase" = "Error" ]; then \
			echo "task $(TASK) terminal phase=$$phase (waiting for $$want)"; \
			printf '%s\n' "$$task_json" | jq '.'; \
			exit 1; \
		fi; \
		sleep "$$interval"; \
	done

real-check-kitchen:
	@$(MAKE) real-check NS=$(KITCHEN_NS) TASK=$(KITCHEN_PRIMARY_TASK)

real-check-tool-approval:
	@$(MAKE) real-check NS=$(APPROVAL_NS) TASK=$(APPROVAL_TASK)

real-gate-pipeline:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(PIPELINE_NS)" "$(PIPELINE_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-pipeline; \
	$(MAKE) real-wait-task-succeeded NS=$(PIPELINE_NS) TASK=$(PIPELINE_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(PIPELINE_TASK)?namespace=$(PIPELINE_NS)"); \
	messages_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.messages[]?] | length'); \
	processed_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "agent_message_processed")] | length'); \
	model_call_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "model_call")] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	[ "$$messages_count" -ge 2 ] || fail "expected at least 2 messages, got $$messages_count"; \
	[ "$$processed_count" -ge 3 ] || fail "expected at least 3 agent_message_processed events, got $$processed_count"; \
	[ "$$model_call_count" -ge 3 ] || fail "expected at least 3 model_call events, got $$model_call_count"; \
	printf '%s\n' "$$last_output" | grep -q 'RECOMMENDATION:' || fail "missing RECOMMENDATION label"; \
	printf '%s\n' "$$last_output" | grep -q 'NEXT_ACTIONS:' || fail "missing NEXT_ACTIONS label"; \
	verdict="passed"; \
	echo "pipeline gate passed"

real-gate-hier:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(HIER_NS)" "$(HIER_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-hier; \
	$(MAKE) real-wait-task-succeeded NS=$(HIER_NS) TASK=$(HIER_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(HIER_TASK)?namespace=$(HIER_NS)"); \
	research_to_editor=$$(printf '%s\n' "$$task_json" | jq -r '[.status.messages[]? | select((.from_agent // "") == "rr-real-hier-research-worker-agent" and (.to_agent // "") == "rr-real-hier-editor-agent")] | length'); \
	social_to_editor=$$(printf '%s\n' "$$task_json" | jq -r '[.status.messages[]? | select((.from_agent // "") == "rr-real-hier-social-worker-agent" and (.to_agent // "") == "rr-real-hier-editor-agent")] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	[ "$$research_to_editor" -ge 1 ] || fail "missing research worker handoff to editor"; \
	[ "$$social_to_editor" -ge 1 ] || fail "missing social worker handoff to editor"; \
	printf '%s\n' "$$last_output" | grep -q 'MERGED_BRANCHES: research,social' || fail "missing merged branches marker"; \
	printf '%s\n' "$$last_output" | grep -q 'FINAL_ACTIONS:' || fail "missing FINAL_ACTIONS label"; \
	verdict="passed"; \
	echo "hierarchical gate passed"

real-gate-loop:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(LOOP_NS)" "$(LOOP_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-loop; \
	$(MAKE) real-wait-task-succeeded NS=$(LOOP_NS) TASK=$(LOOP_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(LOOP_TASK)?namespace=$(LOOP_NS)"); \
	agent_msg_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "agent_message")] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	[ "$$agent_msg_count" -ge 4 ] || fail "expected at least 4 agent_message trace events, got $$agent_msg_count"; \
	printf '%s\n' "$$last_output" | grep -q 'LOOP_STATE:' || fail "missing LOOP_STATE label"; \
	verdict="passed"; \
	echo "loop gate passed"

real-gate-tool:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(TOOL_NS)" "$(TOOL_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-tool; \
	$(MAKE) real-wait-task-succeeded NS=$(TOOL_NS) TASK=$(TOOL_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(TOOL_TASK)?namespace=$(TOOL_NS)"); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	[ "$$trace_tool_calls" -ge 1 ] || fail "expected at least one tool_call trace event"; \
	printf '%s\n' "$$last_output" | grep -qi 'TOOL_USED:[[:space:]]*yes' || fail "missing TOOL_USED yes marker"; \
	printf '%s\n' "$$last_output" | grep -q 'STUB_ENDPOINT: smoke' || fail "missing smoke endpoint marker"; \
	printf '%s\n' "$$last_output" | grep -q 'HEALTH: healthy' || fail "missing HEALTH marker"; \
	verdict="passed"; \
	echo "tool smoke gate passed"

real-gate-hier-tool:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(HIER_TOOL_NS)" "$(HIER_TOOL_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-hier-tool; \
	$(MAKE) real-wait-task-succeeded NS=$(HIER_TOOL_NS) TASK=$(HIER_TOOL_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(HIER_TOOL_TASK)?namespace=$(HIER_TOOL_NS)"); \
	knowledge_to_editor=$$(printf '%s\n' "$$task_json" | jq -r '[.status.messages[]? | select((.from_agent // "") == "rr-real-hier-tool-knowledge-worker-agent" and (.to_agent // "") == "rr-real-hier-tool-editor-agent")] | length'); \
	analytics_to_editor=$$(printf '%s\n' "$$task_json" | jq -r '[.status.messages[]? | select((.from_agent // "") == "rr-real-hier-tool-analytics-worker-agent" and (.to_agent // "") == "rr-real-hier-tool-editor-agent")] | length'); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	lookup_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and (.agent // "") == "rr-real-hier-tool-knowledge-worker-agent" and (.tool // "") == "rr-real-hier-tool-lookup-tool")] | length'); \
	calc_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and (.agent // "") == "rr-real-hier-tool-analytics-worker-agent" and (.tool // "") == "rr-real-hier-tool-calculate-tool")] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	merged_lo=$$(printf '%s\n' "$$last_output" | tr '[:upper:]' '[:lower:]'); \
	[ "$$knowledge_to_editor" -ge 1 ] || fail "missing knowledge worker handoff to editor"; \
	[ "$$analytics_to_editor" -ge 1 ] || fail "missing analytics worker handoff to editor"; \
	[ "$$trace_tool_calls" -ge 2 ] || fail "expected at least 2 tool_call trace events, got $$trace_tool_calls"; \
	[ "$$lookup_calls" -ge 1 ] || fail "expected knowledge worker lookup tool_call, got $$lookup_calls"; \
	[ "$$calc_calls" -ge 1 ] || fail "expected analytics worker calculate tool_call, got $$calc_calls"; \
	printf '%s\n' "$$merged_lo" | grep -Eq 'merged_branches:.*knowledge.*analytics|merged_branches:.*analytics.*knowledge' || fail "missing merged branches marker (expected MERGED_BRANCHES line with knowledge and analytics)"; \
	printf '%s\n' "$$last_output" | grep -qi 'customer_summary:' || fail "missing CUSTOMER_SUMMARY label"; \
	printf '%s\n' "$$last_output" | grep -q 'TOP_RESULT=item-7842' || fail "missing lookup stub marker"; \
	printf '%s\n' "$$last_output" | grep -q 'COMPUTED_RESULT=42' || fail "missing calculate stub marker"; \
	verdict="passed"; \
	echo "hierarchical tool gate passed"

real-check-tool-use:
	@$(MAKE) real-wait-task-succeeded NS=$(DECISION_NS) TASK=$(DECISION_USE_TASK)
	@$(MAKE) real-check NS=$(DECISION_NS) TASK=$(DECISION_USE_TASK)
	@set -eu; \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(DECISION_USE_TASK)?namespace=$(DECISION_NS)"); \
	phase=$$(printf '%s\n' "$$task_json" | jq -r '.status.phase // ""'); \
	tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '(.status.output["agent.1.tool_calls"] | tonumber?) // (.status.output["last_tool_calls"] | tonumber?) // 0'); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	last_event=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["agent.1.last_event"] // .status.output["last_output"] // ""'); \
	[ "$$phase" = "Succeeded" ] || { echo "expected phase Succeeded, got $$phase"; exit 1; }; \
	[ "$$tool_calls" -ge 1 ] || { echo "expected tool calls >= 1, got $$tool_calls"; exit 1; }; \
	[ "$$trace_tool_calls" -ge 1 ] || { echo "expected at least one tool_call trace event, got $$trace_tool_calls"; exit 1; }; \
	printf '%s\n' "$$last_event" | grep -qi 'TOOL_USED:[[:space:]]*yes' || { echo "missing TOOL_USED: yes marker in agent output"; exit 1; }; \
	printf '%s\n' "$$last_event" | grep -q 'EVIDENCE:' || { echo "missing EVIDENCE marker in agent output"; exit 1; }; \
	echo "tool-use gate passed"

real-check-anthropic-tool-use: real-check-tool-use

real-check-tool-no-use:
	@$(MAKE) real-wait-task-succeeded NS=$(DECISION_NS) TASK=$(DECISION_NO_USE_TASK)
	@$(MAKE) real-check NS=$(DECISION_NS) TASK=$(DECISION_NO_USE_TASK)
	@set -eu; \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(DECISION_NO_USE_TASK)?namespace=$(DECISION_NS)"); \
	phase=$$(printf '%s\n' "$$task_json" | jq -r '.status.phase // ""'); \
	tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '(.status.output["agent.1.tool_calls"] | tonumber?) // (.status.output["last_tool_calls"] | tonumber?) // 0'); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	last_event=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["agent.1.last_event"] // .status.output["last_output"] // ""'); \
	[ "$$phase" = "Succeeded" ] || { echo "expected phase Succeeded, got $$phase"; exit 1; }; \
	[ "$$tool_calls" -eq 0 ] || { echo "expected tool calls == 0, got $$tool_calls"; exit 1; }; \
	[ "$$trace_tool_calls" -eq 0 ] || { echo "expected no tool_call trace events, got $$trace_tool_calls"; exit 1; }; \
	printf '%s\n' "$$last_event" | grep -qi 'TOOL_USED:[[:space:]]*no' || { echo "missing TOOL_USED: no marker in agent output"; exit 1; }; \
	printf '%s\n' "$$last_event" | grep -qi 'EVIDENCE:[[:space:]]*self-contained-input' || { echo "missing self-contained EVIDENCE marker in agent output"; exit 1; }; \
	echo "no-tool gate passed"

real-check-anthropic-tool-no-use: real-check-tool-no-use

real-gate-tool-decision: real-apply-tool-decision real-check-tool-use real-check-tool-no-use
	@echo "tool-decision gate passed"

real-gate-anthropic-tool-decision: real-gate-tool-decision
	@echo "anthropic tool-decision gate passed"

real-gate-memory-shared:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(MEMORY_SHARED_NS)" "$(MEMORY_SHARED_TASK)" "$$verdict" "$(MEMORY_SHARED_NAME)" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-memory-shared; \
	$(MAKE) real-wait-task-succeeded NS=$(MEMORY_SHARED_NS) TASK=$(MEMORY_SHARED_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(MEMORY_SHARED_TASK)?namespace=$(MEMORY_SHARED_NS)"); \
	mem_json=$$(curl -sSf "$(API_BASE)/v1/memories/$(MEMORY_SHARED_NAME)/entries?namespace=$(MEMORY_SHARED_NS)&limit=100"); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	last_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '(.status.output["last_tool_calls"] | tonumber?) // 0'); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	trace_tool_errors=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_error")] | length'); \
	research_write_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and (.agent // "") == "rr-real-memory-shared-research-agent" and (.tool // "") == "memory.write")] | length'); \
	writer_search_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and (.agent // "") == "rr-real-memory-shared-writer-agent" and (.tool // "") == "memory.search")] | length'); \
	writer_read_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and (.agent // "") == "rr-real-memory-shared-writer-agent" and (.tool // "") == "memory.read")] | length'); \
	mem_count=$$(printf '%s\n' "$$mem_json" | jq -r '.count // 0'); \
	[ "$$last_tool_calls" -ge 2 ] || fail "expected final agent to make at least 2 tool calls, got $$last_tool_calls"; \
	[ "$$trace_tool_calls" -ge 3 ] || fail "expected at least 3 tool_call trace events across the flow, got $$trace_tool_calls"; \
	[ "$$trace_tool_errors" -eq 0 ] || fail "expected zero tool_error trace events, got $$trace_tool_errors"; \
	[ "$$research_write_calls" -ge 1 ] || fail "expected research to call memory.write at least once, got $$research_write_calls"; \
	[ "$$writer_search_calls" -ge 1 ] || fail "expected writer to call memory.search at least once, got $$writer_search_calls"; \
	[ "$$writer_read_calls" -ge 1 ] || fail "expected writer to call memory.read at least once, got $$writer_read_calls"; \
	[ -n "$$(printf '%s' "$$last_output" | tr -d '[:space:]')" ] || fail "missing final writer output"; \
	[ "$$mem_count" -ge 1 ] || fail "expected at least 1 memory entry, got $$mem_count"; \
	printf '%s\n' "$$mem_json" | jq -e '.entries[]? | select((.key // "") == "triage/recommendation")' >/dev/null || fail "missing triage/recommendation memory entry"; \
	printf '%s\n' "$$mem_json" | jq -e '.entries[]? | select(((.value // "") | ascii_downcase) | contains("rollback completed at 14:17 utc"))' >/dev/null || fail "memory entries did not preserve the rollback fact"; \
	verdict="passed"; \
	echo "memory shared gate passed"

real-gate-memory-reuse:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(MEMORY_REUSE_NS)" "$(MEMORY_REUSE_QUERY_TASK)" "$$verdict" "$(MEMORY_REUSE_NAME)" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-memory-reuse; \
	$(MAKE) real-wait-task-succeeded NS=$(MEMORY_REUSE_NS) TASK=$(MEMORY_REUSE_SEED_TASK); \
	seed_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(MEMORY_REUSE_SEED_TASK)?namespace=$(MEMORY_REUSE_NS)"); \
	seed_output=$$(printf '%s\n' "$$seed_json" | jq -r '.status.output["last_output"] // ""'); \
	printf '%s\n' "$$seed_output" | grep -q 'MODE: seed' || fail "missing seed mode marker"; \
	$(MAKE) real-apply-memory-reuse-query; \
	$(MAKE) real-wait-task-succeeded NS=$(MEMORY_REUSE_NS) TASK=$(MEMORY_REUSE_QUERY_TASK); \
	query_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(MEMORY_REUSE_QUERY_TASK)?namespace=$(MEMORY_REUSE_NS)"); \
	mem_json=$$(curl -sSf "$(API_BASE)/v1/memories/$(MEMORY_REUSE_NAME)/entries?namespace=$(MEMORY_REUSE_NS)&limit=100"); \
	query_output=$$(printf '%s\n' "$$query_json" | jq -r '.status.output["last_output"] // ""'); \
	printf '%s\n' "$$query_output" | grep -q 'MODE: query' || fail "missing query mode marker"; \
	printf '%s\n' "$$query_output" | grep -q 'USED_MEMORY: yes' || fail "missing USED_MEMORY marker"; \
	printf '%s\n' "$$query_output" | grep -q 'RUNBOOK_QUOTE: pause the rollout and roll back release 2026.03.18.2' || fail "missing runbook quote marker"; \
	printf '%s\n' "$$mem_json" | jq -e '.entries[]? | select((.key // "") == "runbook/primary_action")' >/dev/null || fail "missing runbook/primary_action memory entry"; \
	verdict="passed"; \
	echo "memory reuse gate passed"

real-gate-tool-auth:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(TOOL_AUTH_NS)" "$(TOOL_AUTH_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-tool-auth; \
	$(MAKE) real-wait-task-succeeded NS=$(TOOL_AUTH_NS) TASK=$(TOOL_AUTH_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(TOOL_AUTH_TASK)?namespace=$(TOOL_AUTH_NS)"); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	trace_tool_errors=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_error")] | length'); \
	isolation_unavailable_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_error" and ((.message // "") | contains("tool_code=isolation_unavailable")))] | length'); \
	secret_resolution_failed_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_error" and ((.message // "") | contains("tool_code=secret_resolution_failed")))] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	[ "$$isolation_unavailable_count" -eq 0 ] || fail "tool isolation unavailable; start tool-backed worker and local stub (make real-tool-stub)"; \
	[ "$$secret_resolution_failed_count" -eq 0 ] || fail "tool auth secret resolution failed; verify Secret rr-real-tool-auth-key exists in namespace rr-real-tool-auth with spec.stringData.value set"; \
	[ "$$trace_tool_calls" -ge 1 ] || fail "expected at least one tool_call trace event"; \
	[ "$$trace_tool_errors" -eq 0 ] || fail "expected zero tool_error trace events, got $$trace_tool_errors"; \
	printf '%s\n' "$$last_output" | grep -q 'AUTH_PATH: ok' || fail "missing AUTH_PATH marker"; \
	printf '%s\n' "$$last_output" | grep -q 'CONTRACT_PATH: ok' || fail "missing CONTRACT_PATH marker"; \
	printf '%s\n' "$$last_output" | grep -q 'EVIDENCE: AUTH_STATUS=ok' || fail "missing auth evidence marker"; \
	verdict="passed"; \
	echo "tool auth gate passed"

real-gate-governance-deny:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(GOV_DENY_NS)" "$(GOV_DENY_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-governance-deny; \
	$(MAKE) real-wait-task-terminal NS=$(GOV_DENY_NS) TASK=$(GOV_DENY_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(GOV_DENY_TASK)?namespace=$(GOV_DENY_NS)"); \
	phase=$$(printf '%s\n' "$$task_json" | jq -r '.status.phase // ""'); \
	trace_denied_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_permission_denied")] | length'); \
	message_denied_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select(((.type // "") == "agent_message_non_retryable" or (.type // "") == "agent_message_deadletter") and (((.message // "") | ascii_downcase) | contains("tool_reason=tool_permission_denied") or contains("tool permission denied")))] | length'); \
	denied_count=$$((trace_denied_count + message_denied_count)); \
	tool_call_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	[ "$$phase" != "Succeeded" ] || fail "expected non-succeeded terminal phase for governance deny"; \
	[ "$$denied_count" -ge 1 ] || fail "expected governance deny evidence (tool_permission_denied trace or deadletter/non-retryable deny message)"; \
	[ "$$tool_call_count" -eq 0 ] || fail "expected zero successful tool_call trace events"; \
	verdict="passed"; \
	echo "governance deny gate passed"

real-gate-tool-retry:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(TOOL_RETRY_NS)" "$(TOOL_RETRY_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-tool-retry; \
	$(MAKE) real-wait-task-succeeded NS=$(TOOL_RETRY_NS) TASK=$(TOOL_RETRY_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(TOOL_RETRY_TASK)?namespace=$(TOOL_RETRY_NS)"); \
	tool_error_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_error")] | length'); \
	tool_call_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	successful_second_call_count=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and ((.message // "") | contains("/s002/") and contains("success")))] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	[ "$$tool_error_count" -ge 1 ] || fail "expected at least one tool_error trace event"; \
	[ "$$tool_call_count" -ge 1 ] || fail "expected a successful tool_call after retry"; \
	if printf '%s\n' "$$last_output" | grep -q 'RECOVERY_STATUS: recovered' && printf '%s\n' "$$last_output" | grep -q 'ATTEMPT=2'; then \
		:; \
	else \
		[ "$$successful_second_call_count" -ge 1 ] || fail "missing retry recovery evidence (expected RECOVERY_STATUS/ATTEMPT markers or successful second tool call)"; \
	fi; \
	verdict="passed"; \
	echo "tool retry gate passed"

real-gate-webhook:
	@set -eu; \
	verdict="failed"; \
	webhook_task=""; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	capture_task() { if [ -n "$$webhook_task" ]; then API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(WEBHOOK_NS)" "$$webhook_task" "$$verdict" "$(WEBHOOK_MEMORY_NAME)" >/dev/null || true; fi; }; \
	trap capture_task EXIT; \
	$(MAKE) real-apply-webhook; \
	hook_json=$$(curl -sSf "$(API_BASE)/v1/task-webhooks/$(WEBHOOK_NAME)?namespace=$(WEBHOOK_NS)"); \
	endpoint_path=$$(printf '%s\n' "$$hook_json" | jq -r '.status.endpointPath // ""'); \
	[ -n "$$endpoint_path" ] || fail "missing webhook endpoint path"; \
	body='{"event":"incident.triggered","service":"checkout","severity":"sev1"}'; \
	ts=$$(date +%s); \
	sig=$$(printf '%s' "$$ts.$$body" | openssl dgst -sha256 -hmac "$(REAL_WEBHOOK_SECRET)" -binary | xxd -p -c 256); \
	resp_file=$$(mktemp); \
	http_code=$$(curl -sS -o "$$resp_file" -w "%{http_code}" -X POST "$(API_BASE)$$endpoint_path" \
		-H "Content-Type: application/json" \
		-H "X-Timestamp: $$ts" \
		-H "X-Event-Id: evt-real-webhook-001" \
		-H "X-Signature: sha256=$$sig" \
		--data "$$body"); \
	[ "$$http_code" = "202" ] || fail "expected webhook delivery status 202, got $$http_code"; \
	webhook_task_scoped=$$(cat "$$resp_file" | jq -r '.task // ""'); \
	rm -f "$$resp_file"; \
	[ -n "$$webhook_task_scoped" ] || fail "webhook delivery response did not include task"; \
	webhook_task=$${webhook_task_scoped##*/}; \
	$(MAKE) real-wait-task-succeeded NS=$(WEBHOOK_NS) TASK=$$webhook_task; \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$$webhook_task?namespace=$(WEBHOOK_NS)"); \
	mem_json=$$(curl -sSf "$(API_BASE)/v1/memories/$(WEBHOOK_MEMORY_NAME)/entries?namespace=$(WEBHOOK_NS)&limit=100"); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	printf '%s\n' "$$mem_json" | jq -e '.entries[]? | select((.key // "") | startswith("webhook/"))' >/dev/null || fail "missing webhook memory entry"; \
	if printf '%s\n' "$$last_output" | grep -q 'WEBHOOK_FLOW: accepted' && printf '%s\n' "$$last_output" | grep -q 'USED_MEMORY: yes'; then \
		:; \
	else \
		[ "$$trace_tool_calls" -ge 1 ] || fail "missing webhook evidence (expected WEBHOOK_FLOW/USED_MEMORY markers or at least one tool_call trace event)"; \
	fi; \
	verdict="passed"; \
	echo "webhook gate passed (task=$$webhook_task)"

real-gate-schedule:
	@set -eu; \
	verdict="failed"; \
	schedule_task=""; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	capture_task() { if [ -n "$$schedule_task" ]; then API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(SCHEDULE_NS)" "$$schedule_task" "$$verdict" "$(SCHEDULE_MEMORY_NAME)" >/dev/null || true; fi; }; \
	trap capture_task EXIT; \
	$(MAKE) real-apply-schedule; \
	deadline=$$(( $$(date +%s) + $(REAL_SCHEDULE_TIMEOUT_SECONDS) )); \
	while true; do \
		now=$$(date +%s); \
		[ $$now -lt $$deadline ] || fail "timeout waiting for schedule to trigger"; \
		schedule_json=$$(curl -sSf "$(API_BASE)/v1/task-schedules/$(SCHEDULE_NAME)?namespace=$(SCHEDULE_NS)"); \
		task_scoped=$$(printf '%s\n' "$$schedule_json" | jq -r '.status.lastTriggeredTask // ""'); \
		if [ -n "$$task_scoped" ]; then \
			schedule_task=$${task_scoped##*/}; \
			break; \
		fi; \
		sleep "$(REAL_GATE_POLL_INTERVAL_SECONDS)"; \
	done; \
	$(MAKE) real-wait-task-succeeded NS=$(SCHEDULE_NS) TASK=$$schedule_task; \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$$schedule_task?namespace=$(SCHEDULE_NS)"); \
	mem_json=$$(curl -sSf "$(API_BASE)/v1/memories/$(SCHEDULE_MEMORY_NAME)/entries?namespace=$(SCHEDULE_NS)&limit=100"); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	printf '%s\n' "$$mem_json" | jq -e '.entries[]? | select((.key // "") == "schedule/last-run")' >/dev/null || fail "missing schedule/last-run memory entry"; \
	if printf '%s\n' "$$last_output" | grep -q 'SCHEDULE_FLOW: accepted' && printf '%s\n' "$$last_output" | grep -q 'USED_MEMORY: yes'; then \
		:; \
	else \
		[ "$$trace_tool_calls" -ge 1 ] || fail "missing schedule evidence (expected SCHEDULE_FLOW/USED_MEMORY markers or at least one tool_call trace event)"; \
	fi; \
	verdict="passed"; \
	echo "schedule gate passed (task=$$schedule_task)"

real-gate-mcp:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(MCP_NS)" "$(MCP_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-mcp; \
	$(MAKE) real-wait-task-succeeded NS=$(MCP_NS) TASK=$(MCP_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(MCP_TASK)?namespace=$(MCP_NS)"); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	printf '%s\n' "$$last_output" | grep -qi 'ECHO_RESULT:' || fail "missing ECHO_RESULT marker"; \
	printf '%s\n' "$$last_output" | grep -qi 'mcp-smoke-test-marker' || fail "echo tool did not return expected marker"; \
	printf '%s\n' "$$last_output" | grep -qi 'SUM_RESULT:' || fail "missing SUM_RESULT marker"; \
	printf '%s\n' "$$last_output" | grep -q '42' || fail "get-sum tool did not return expected sum (42)"; \
	printf '%s\n' "$$last_output" | grep -qi 'MCP_SERVER:' || fail "missing MCP_SERVER marker"; \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	[ "$$trace_tool_calls" -ge 2 ] || fail "expected at least 2 tool_call trace events, got $$trace_tool_calls"; \
	tools_json=$$(curl -sSf "$(API_BASE)/v1/tools?namespace=$(MCP_NS)"); \
	echo_tool=$$(printf '%s\n' "$$tools_json" | jq -r '.items[] | select(.metadata.name == "rr-real-mcp-everything--echo") | .spec.type // ""'); \
	[ "$$echo_tool" = "mcp" ] || fail "expected echo tool type=mcp, got $$echo_tool"; \
	sum_tool=$$(printf '%s\n' "$$tools_json" | jq -r '.items[] | select(.metadata.name == "rr-real-mcp-everything--get-sum") | .spec.type // ""'); \
	[ "$$sum_tool" = "mcp" ] || fail "expected get-sum tool type=mcp, got $$sum_tool"; \
	total_tools=$$(printf '%s\n' "$$tools_json" | jq '[.items[] | select(.spec.type == "mcp")] | length'); \
	[ "$$total_tools" -eq 2 ] || fail "tool_filter should produce exactly 2 tools, got $$total_tools"; \
	verdict="passed"; \
	echo "mcp gate passed"

real-gate-kitchen:
	@set -eu; \
	verdict="failed"; \
	kitchen_webhook_task=""; \
	kitchen_schedule_task=""; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(KITCHEN_NS)" "$(KITCHEN_PRIMARY_TASK)" "$$verdict" "$(KITCHEN_REUSE_MEMORY)" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-kitchen; \
	REAL_GATE_TIMEOUT_SECONDS=$(REAL_KITCHEN_GATE_TIMEOUT_SECONDS) $(MAKE) real-wait-task-succeeded NS=$(KITCHEN_NS) TASK=$(KITCHEN_PRIMARY_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(KITCHEN_PRIMARY_TASK)?namespace=$(KITCHEN_NS)"); \
	kw_to_ed=$$(printf '%s\n' "$$task_json" | jq -r '[.status.messages[]? | select((.from_agent // "") == "rr-real-kitchen-knowledge-worker-agent" and (.to_agent // "") == "rr-real-kitchen-editor-agent")] | length'); \
	aw_to_ed=$$(printf '%s\n' "$$task_json" | jq -r '[.status.messages[]? | select((.from_agent // "") == "rr-real-kitchen-analytics-worker-agent" and (.to_agent // "") == "rr-real-kitchen-editor-agent")] | length'); \
	mw_to_ed=$$(printf '%s\n' "$$task_json" | jq -r '[.status.messages[]? | select((.from_agent // "") == "rr-real-kitchen-mcp-worker-agent" and (.to_agent // "") == "rr-real-kitchen-editor-agent")] | length'); \
	auth_to_ed=$$(printf '%s\n' "$$task_json" | jq -r '[.status.messages[]? | select((.from_agent // "") == "rr-real-kitchen-auth-worker-agent" and (.to_agent // "") == "rr-real-kitchen-editor-agent")] | length'); \
	retry_to_ed=$$(printf '%s\n' "$$task_json" | jq -r '[.status.messages[]? | select((.from_agent // "") == "rr-real-kitchen-retry-worker-agent" and (.to_agent // "") == "rr-real-kitchen-editor-agent")] | length'); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	lookup_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and (.agent // "") == "rr-real-kitchen-knowledge-worker-agent" and (.tool // "") == "rr-real-kitchen-lookup-tool")] | length'); \
	calc_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and (.agent // "") == "rr-real-kitchen-analytics-worker-agent" and (.tool // "") == "rr-real-kitchen-calculate-tool")] | length'); \
	mcp_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and (.agent // "") == "rr-real-kitchen-mcp-worker-agent")] | length'); \
	auth_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and (.agent // "") == "rr-real-kitchen-auth-worker-agent" and (.tool // "") == "rr-real-kitchen-auth-tool")] | length'); \
	retry_tool_errors=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_error" and (.agent // "") == "rr-real-kitchen-retry-worker-agent" and (.tool // "") == "rr-real-kitchen-retry-tool")] | length'); \
	retry_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call" and (.agent // "") == "rr-real-kitchen-retry-worker-agent" and (.tool // "") == "rr-real-kitchen-retry-tool")] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	merged_lo=$$(printf '%s\n' "$$last_output" | tr '[:upper:]' '[:lower:]'); \
	[ "$$kw_to_ed" -ge 1 ] || fail "missing knowledge worker handoff to editor"; \
	[ "$$aw_to_ed" -ge 1 ] || fail "missing analytics worker handoff to editor"; \
	[ "$$mw_to_ed" -ge 1 ] || fail "missing mcp worker handoff to editor"; \
	[ "$$auth_to_ed" -ge 1 ] || fail "missing auth worker handoff to editor"; \
	[ "$$retry_to_ed" -ge 1 ] || fail "missing retry worker handoff to editor"; \
	[ "$$trace_tool_calls" -ge 6 ] || fail "expected at least 6 tool_call trace events (lookup+calc+2 mcp+auth+retry), got $$trace_tool_calls"; \
	[ "$$lookup_calls" -ge 1 ] || fail "expected lookup tool_call"; \
	[ "$$calc_calls" -ge 1 ] || fail "expected calculate tool_call"; \
	[ "$$mcp_calls" -ge 2 ] || fail "expected at least 2 MCP tool_call events, got $$mcp_calls"; \
	[ "$$auth_calls" -ge 1 ] || fail "expected auth tool_call"; \
	[ "$$retry_calls" -ge 1 ] || fail "expected retry tool_call after recovery"; \
	if [ "$$retry_tool_errors" -lt 1 ]; then \
		printf '%s\n' "$$merged_lo" | grep -q 'attempt=2' || fail "expected retry tool_error trace or ATTEMPT=2 (HTTP runtime retry) in output"; \
	fi; \
	printf '%s\n' "$$merged_lo" | grep -Eq 'merged_branches:.*knowledge' || fail "missing knowledge in MERGED_BRANCHES"; \
	printf '%s\n' "$$merged_lo" | grep -Eq 'merged_branches:.*analytics' || fail "missing analytics in MERGED_BRANCHES"; \
	printf '%s\n' "$$merged_lo" | grep -Eq 'merged_branches:.*mcp' || fail "missing mcp in MERGED_BRANCHES"; \
	printf '%s\n' "$$merged_lo" | grep -Eq 'merged_branches:.*auth' || fail "missing auth in MERGED_BRANCHES"; \
	printf '%s\n' "$$merged_lo" | grep -Eq 'merged_branches:.*retry' || fail "missing retry in MERGED_BRANCHES"; \
	printf '%s\n' "$$last_output" | grep -q 'TOP_RESULT=item-7842' || fail "missing lookup stub marker"; \
	printf '%s\n' "$$last_output" | grep -q 'COMPUTED_RESULT=42' || fail "missing calculate stub marker"; \
	printf '%s\n' "$$last_output" | grep -q 'mcp-smoke-test-marker' || fail "missing MCP echo marker"; \
	printf '%s\n' "$$last_output" | grep -q 'AUTH_STATUS=ok' || fail "missing auth evidence marker"; \
	printf '%s\n' "$$last_output" | grep -q 'ATTEMPT=2' || fail "missing retry attempt marker"; \
	$(MAKE) real-apply-kitchen-memory-seed; \
	REAL_GATE_TIMEOUT_SECONDS=$(REAL_KITCHEN_GATE_TIMEOUT_SECONDS) $(MAKE) real-wait-task-succeeded NS=$(KITCHEN_NS) TASK=$(KITCHEN_MEMORY_SEED_TASK); \
	seed_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(KITCHEN_MEMORY_SEED_TASK)?namespace=$(KITCHEN_NS)"); \
	seed_output=$$(printf '%s\n' "$$seed_json" | jq -r '.status.output["last_output"] // ""'); \
	printf '%s\n' "$$seed_output" | grep -q 'MODE: seed' || fail "missing kitchen memory seed marker"; \
	$(MAKE) real-apply-kitchen-memory-query; \
	REAL_GATE_TIMEOUT_SECONDS=$(REAL_KITCHEN_GATE_TIMEOUT_SECONDS) $(MAKE) real-wait-task-succeeded NS=$(KITCHEN_NS) TASK=$(KITCHEN_MEMORY_QUERY_TASK); \
	query_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(KITCHEN_MEMORY_QUERY_TASK)?namespace=$(KITCHEN_NS)"); \
	mem_json=$$(curl -sSf "$(API_BASE)/v1/memories/$(KITCHEN_REUSE_MEMORY)/entries?namespace=$(KITCHEN_NS)&limit=100"); \
	query_output=$$(printf '%s\n' "$$query_json" | jq -r '.status.output["last_output"] // ""'); \
	printf '%s\n' "$$query_output" | grep -q 'MODE: query' || fail "missing kitchen memory query marker"; \
	printf '%s\n' "$$query_output" | grep -q 'USED_MEMORY: yes' || fail "missing USED_MEMORY marker"; \
	printf '%s\n' "$$query_output" | grep -q 'RUNBOOK_QUOTE: drain queue shard-7 then re-enable traffic' || fail "missing runbook quote marker"; \
	printf '%s\n' "$$mem_json" | jq -e '.entries[]? | select((.key // "") == "runbook/kitchen_action")' >/dev/null || fail "missing runbook/kitchen_action memory entry"; \
	hook_json=$$(curl -sSf "$(API_BASE)/v1/task-webhooks/$(KITCHEN_WEBHOOK_NAME)?namespace=$(KITCHEN_NS)"); \
	endpoint_path=$$(printf '%s\n' "$$hook_json" | jq -r '.status.endpointPath // ""'); \
	[ -n "$$endpoint_path" ] || fail "missing kitchen webhook endpoint path"; \
	body='{"event":"kitchen.webhook","service":"integration","severity":"sev2"}'; \
	ts=$$(date +%s); \
	sig=$$(printf '%s' "$$ts.$$body" | openssl dgst -sha256 -hmac "$(REAL_WEBHOOK_SECRET)" -binary | xxd -p -c 256); \
	resp_file=$$(mktemp); \
	http_code=$$(curl -sS -o "$$resp_file" -w "%{http_code}" -X POST "$(API_BASE)$$endpoint_path" \
		-H "Content-Type: application/json" \
		-H "X-Timestamp: $$ts" \
		-H "X-Event-Id: evt-kitchen-webhook-001" \
		-H "X-Signature: sha256=$$sig" \
		--data "$$body"); \
	[ "$$http_code" = "202" ] || fail "expected kitchen webhook status 202, got $$http_code"; \
	webhook_task_scoped=$$(cat "$$resp_file" | jq -r '.task // ""'); \
	rm -f "$$resp_file"; \
	[ -n "$$webhook_task_scoped" ] || fail "kitchen webhook response missing task"; \
	kitchen_webhook_task=$${webhook_task_scoped##*/}; \
	REAL_GATE_TIMEOUT_SECONDS=$(REAL_KITCHEN_GATE_TIMEOUT_SECONDS) $(MAKE) real-wait-task-succeeded NS=$(KITCHEN_NS) TASK=$$kitchen_webhook_task; \
	wh_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$$kitchen_webhook_task?namespace=$(KITCHEN_NS)"); \
	wh_mem=$$(curl -sSf "$(API_BASE)/v1/memories/$(KITCHEN_WEBHOOK_MEMORY)/entries?namespace=$(KITCHEN_NS)&limit=100"); \
	printf '%s\n' "$$wh_mem" | jq -e '.entries[]? | select((.key // "") | startswith("webhook/"))' >/dev/null || fail "missing kitchen webhook memory entry"; \
	deadline=$$(( $$(date +%s) + $(REAL_SCHEDULE_TIMEOUT_SECONDS) )); \
	while true; do \
		now=$$(date +%s); \
		[ $$now -lt $$deadline ] || fail "timeout waiting for kitchen schedule trigger"; \
		schedule_json=$$(curl -sSf "$(API_BASE)/v1/task-schedules/$(KITCHEN_SCHEDULE_NAME)?namespace=$(KITCHEN_NS)"); \
		task_scoped=$$(printf '%s\n' "$$schedule_json" | jq -r '.status.lastTriggeredTask // ""'); \
		if [ -n "$$task_scoped" ]; then \
			kitchen_schedule_task=$${task_scoped##*/}; \
			break; \
		fi; \
		sleep "$(REAL_GATE_POLL_INTERVAL_SECONDS)"; \
	done; \
	REAL_GATE_TIMEOUT_SECONDS=$(REAL_KITCHEN_GATE_TIMEOUT_SECONDS) $(MAKE) real-wait-task-succeeded NS=$(KITCHEN_NS) TASK=$$kitchen_schedule_task; \
	sch_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$$kitchen_schedule_task?namespace=$(KITCHEN_NS)"); \
	sch_mem=$$(curl -sSf "$(API_BASE)/v1/memories/$(KITCHEN_SCHEDULE_MEMORY)/entries?namespace=$(KITCHEN_NS)&limit=100"); \
	printf '%s\n' "$$sch_mem" | jq -e '.entries[]? | select((.key // "") == "schedule/last-run")' >/dev/null || fail "missing kitchen schedule memory entry"; \
	verdict="passed"; \
	echo "kitchen sink gate passed (webhook_task=$$kitchen_webhook_task schedule_task=$$kitchen_schedule_task)"

real-gate-tool-approval:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(APPROVAL_NS)" "$(APPROVAL_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-tool-approval; \
	REAL_GATE_TIMEOUT_SECONDS=$(REAL_APPROVAL_GATE_TIMEOUT_SECONDS) $(MAKE) real-wait-task-phase NS=$(APPROVAL_NS) TASK=$(APPROVAL_TASK) PHASE=WaitingApproval; \
	approval_name=$$(curl -sSf "$(API_BASE)/v1/tool-approvals?namespace=$(APPROVAL_NS)" | jq -r '(.items // [])[] | select((.status.phase // "") == "Pending") | .metadata.name' | head -n 1); \
	[ -n "$$approval_name" ] || fail "no pending ToolApproval in namespace $(APPROVAL_NS)"; \
	echo ""; \
	echo "Task is WaitingApproval — approve manually (timeout $(REAL_APPROVAL_GATE_TIMEOUT_SECONDS)s for task to finish):"; \
	echo "  • One approval covers this inbox message for the listed tool (the worker restarts execution after you approve)."; \
	echo "  • UI: Approvals → approve \"$$approval_name\""; \
	echo "  • curl: curl -sS -X POST \"$(API_BASE)/v1/tool-approvals/$$approval_name/approve?namespace=$(APPROVAL_NS)\" \\"; \
	echo "      -H 'Content-Type: application/json' -d '{\"decided_by\":\"manual\"}'"; \
	echo ""; \
	REAL_GATE_TIMEOUT_SECONDS=$(REAL_APPROVAL_GATE_TIMEOUT_SECONDS) $(MAKE) real-wait-task-succeeded NS=$(APPROVAL_NS) TASK=$(APPROVAL_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(APPROVAL_TASK)?namespace=$(APPROVAL_NS)"); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	[ "$$trace_tool_calls" -ge 1 ] || fail "expected at least one tool_call after approval"; \
	printf '%s\n' "$$last_output" | grep -q 'APPROVAL_FLOW: completed' || fail "missing APPROVAL_FLOW marker"; \
	printf '%s\n' "$$last_output" | grep -q 'STUB_ENDPOINT: smoke' || fail "missing smoke stub marker"; \
	verdict="passed"; \
	echo "tool approval gate passed"

real-gate-tool-approval-ci:
	@set -eu; \
	verdict="failed"; \
	fail() { verdict="failed: $$1"; echo "$$1"; exit 1; }; \
	trap 'API_BASE="$(API_BASE)" ARTIFACT_ROOT="$(REAL_ARTIFACTS_DIR)" testing/scenarios-real/capture.sh "$(APPROVAL_NS)" "$(APPROVAL_TASK)" "$$verdict" >/dev/null || true' EXIT; \
	$(MAKE) real-apply-tool-approval; \
	REAL_GATE_TIMEOUT_SECONDS=$(REAL_APPROVAL_GATE_TIMEOUT_SECONDS) $(MAKE) real-wait-task-phase NS=$(APPROVAL_NS) TASK=$(APPROVAL_TASK) PHASE=WaitingApproval; \
	approval_name=$$(curl -sSf "$(API_BASE)/v1/tool-approvals?namespace=$(APPROVAL_NS)" | jq -r '(.items // [])[] | select((.status.phase // "") == "Pending") | .metadata.name' | head -n 1); \
	[ -n "$$approval_name" ] || fail "no pending ToolApproval in namespace $(APPROVAL_NS)"; \
	http_code=$$(curl -sS -o /dev/null -w "%{http_code}" -X POST "$(API_BASE)/v1/tool-approvals/$$approval_name/approve?namespace=$(APPROVAL_NS)" \
		-H "Content-Type: application/json" \
		-d '{"decided_by":"make-real-gate-tool-approval-ci"}'); \
	[ "$$http_code" = "200" ] || fail "approve returned HTTP $$http_code"; \
	REAL_GATE_TIMEOUT_SECONDS=$(REAL_APPROVAL_GATE_TIMEOUT_SECONDS) $(MAKE) real-wait-task-succeeded NS=$(APPROVAL_NS) TASK=$(APPROVAL_TASK); \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(APPROVAL_TASK)?namespace=$(APPROVAL_NS)"); \
	trace_tool_calls=$$(printf '%s\n' "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	last_output=$$(printf '%s\n' "$$task_json" | jq -r '.status.output["last_output"] // ""'); \
	[ "$$trace_tool_calls" -ge 1 ] || fail "expected at least one tool_call after approval"; \
	printf '%s\n' "$$last_output" | grep -q 'APPROVAL_FLOW: completed' || fail "missing APPROVAL_FLOW marker"; \
	printf '%s\n' "$$last_output" | grep -q 'STUB_ENDPOINT: smoke' || fail "missing smoke stub marker"; \
	verdict="passed"; \
	echo "tool approval gate passed (ci auto-approve)"

real-gate-wave0: real-gate-pipeline real-gate-hier real-gate-loop real-gate-tool real-gate-tool-decision real-gate-hier-tool
	@echo "wave 0 gates passed"

real-gate-wave1: real-gate-memory-shared real-gate-memory-reuse
	@echo "wave 1 gates passed"

real-gate-wave2: real-gate-tool-auth real-gate-governance-deny real-gate-tool-retry
	@echo "wave 2 gates passed"

real-gate-wave3: real-gate-webhook real-gate-schedule
	@echo "wave 3 gates passed"

real-gate-wave4: real-gate-mcp
	@echo "wave 4 gates passed"

real-gate-wave5: real-gate-kitchen real-gate-tool-approval-ci
	@echo "wave 5 gates passed"

real-check-all: real-check-pipeline real-check-hier real-check-loop real-check-tool real-check-hier-tool
