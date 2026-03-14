SHELL := /bin/zsh

AGENTCTL ?= go run ./cmd/orlojctl
API_BASE ?= http://localhost:8080
SCENARIOS_REAL_DIR ?= testing/scenarios-real

PIPELINE_SCENARIO := 01-openai-pipeline
HIER_SCENARIO := 02-openai-hierarchical
LOOP_SCENARIO := 03-openai-loop-max-turns
TOOL_SCENARIO := 04-tool-call-smoke
ANTHROPIC_DECISION_SCENARIO := 05-anthropic-tool-decision

PIPELINE_NS := rr-real-pipeline
HIER_NS := rr-real-hier
LOOP_NS := rr-real-loop
TOOL_NS := rr-real-tool
ANTHROPIC_DECISION_NS := rr-real-anthropic-tool-decision

PIPELINE_TASK := rr-real-pipeline-task
HIER_TASK := rr-real-hier-task
LOOP_TASK := rr-real-loop-task
TOOL_TASK := rr-real-tool-task
ANTHROPIC_USE_TASK := rr-anthropic-tool-use-task
ANTHROPIC_NO_USE_TASK := rr-anthropic-tool-no-use-task

REAL_GATE_TIMEOUT_SECONDS ?= 240
REAL_GATE_POLL_INTERVAL_SECONDS ?= 2

.PHONY: help ui-install ui-dev ui-build \
	real-help real-apply real-apply-all \
	real-apply-pipeline real-apply-hier real-apply-loop real-apply-tool real-apply-anthropic-tool-decision \
	real-get real-messages real-metrics real-check \
	real-check-pipeline real-check-hier real-check-loop real-check-tool \
	real-check-anthropic-tool-use real-check-anthropic-tool-no-use \
	real-gate-anthropic-tool-decision \
	real-wait-task-succeeded \
	real-check-all

help: real-help

ui-install:
	cd frontend && npm install

ui-dev:
	cd frontend && npm run dev

ui-build:
	cd frontend && npm run build

real-help:
	@echo "UI migration scaffold:"
	@echo "  make ui-install   # install frontend dependencies"
	@echo "  make ui-dev       # run React dev server on http://127.0.0.1:5173"
	@echo "  make ui-build     # build frontend into frontend/dist for Go embedding"
	@echo ""
	@echo "Runtime startup reference:"
	@echo "  Terminal 1 (control plane):"
	@echo "    go run ./cmd/orlojd --task-execution-mode=message-driven --agent-message-bus-backend=memory"
	@echo "  Terminal 2 (worker, OpenAI):"
	@echo "    go run ./cmd/orlojworker --task-execution-mode=message-driven --agent-message-bus-backend=memory --agent-message-consume --model-gateway-provider=openai"
	@echo "  Terminal 2 for tool smoke (container isolation):"
	@echo "    go run ./cmd/orlojworker --task-execution-mode=message-driven --agent-message-bus-backend=memory --agent-message-consume --model-gateway-provider=openai --tool-isolation-backend=container"
	@echo "  Terminal 2 for Anthropic tool-decision gate:"
	@echo "    go run ./cmd/orlojworker --task-execution-mode=message-driven --agent-message-bus-backend=memory --agent-message-consume --model-gateway-provider=anthropic --tool-isolation-backend=container --tool-container-network=bridge"
	@echo ""
	@echo "Real-model scenario shortcuts:"
	@echo "  make real-apply-pipeline"
	@echo "  make real-apply-hier"
	@echo "  make real-apply-loop"
	@echo "  make real-apply-tool"
	@echo "  make real-apply-anthropic-tool-decision"
	@echo ""
	@echo "  make real-check-pipeline"
	@echo "  make real-check-hier"
	@echo "  make real-check-loop"
	@echo "  make real-check-tool"
	@echo "  make real-check-anthropic-tool-use"
	@echo "  make real-check-anthropic-tool-no-use"
	@echo "  make real-gate-anthropic-tool-decision"
	@echo "  make real-check-all"
	@echo ""
	@echo "Generic variants:"
	@echo "  make real-apply SCENARIO=01-openai-pipeline"
	@echo "  make real-check NS=rr-real-pipeline TASK=rr-real-pipeline-task"
	@echo "  make real-get NS=... TASK=..."
	@echo "  make real-messages NS=... TASK=..."
	@echo "  make real-metrics NS=... TASK=..."
	@echo ""
	@echo "Optional overrides:"
	@echo "  AGENTCTL='go run ./cmd/orlojctl'"
	@echo "  API_BASE='http://localhost:8080'"

real-apply:
	@if [ -z "$(SCENARIO)" ]; then \
		echo "SCENARIO is required. Example: make real-apply SCENARIO=$(PIPELINE_SCENARIO)"; \
		exit 1; \
	fi
	@if [ ! -d "$(SCENARIOS_REAL_DIR)/$(SCENARIO)" ]; then \
		echo "Scenario directory not found: $(SCENARIOS_REAL_DIR)/$(SCENARIO)"; \
		exit 1; \
	fi
	@if rg -n 'value:[[:space:]]*replace-me' "$(SCENARIOS_REAL_DIR)/$(SCENARIO)" >/dev/null; then \
		echo "Secret placeholder detected in $(SCENARIOS_REAL_DIR)/$(SCENARIO). Replace spec.stringData.value first."; \
		exit 1; \
	fi
	@find "$(SCENARIOS_REAL_DIR)/$(SCENARIO)" -name '*.yaml' -print | sort | xargs -I{} $(AGENTCTL) apply -f {}

real-apply-pipeline:
	@$(MAKE) real-apply SCENARIO=$(PIPELINE_SCENARIO)

real-apply-hier:
	@$(MAKE) real-apply SCENARIO=$(HIER_SCENARIO)

real-apply-loop:
	@$(MAKE) real-apply SCENARIO=$(LOOP_SCENARIO)

real-apply-tool:
	@$(MAKE) real-apply SCENARIO=$(TOOL_SCENARIO)

real-apply-anthropic-tool-decision:
	@$(MAKE) real-apply SCENARIO=$(ANTHROPIC_DECISION_SCENARIO)

real-apply-all: real-apply-pipeline real-apply-hier real-apply-loop real-apply-tool

real-get:
	@if [ -z "$(NS)" ] || [ -z "$(TASK)" ]; then \
		echo "NS and TASK are required. Example: make real-get NS=$(PIPELINE_NS) TASK=$(PIPELINE_TASK)"; \
		exit 1; \
	fi
	@$(AGENTCTL) get task "$(TASK)" --namespace "$(NS)"

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
		last_phase=$$(echo "$$task_json" | jq -r '.status.phase // ""'); \
		if [ "$$last_phase" = "Succeeded" ]; then \
			echo "task $(TASK) phase=$$last_phase"; \
			break; \
		fi; \
		if [ "$$last_phase" = "Failed" ] || [ "$$last_phase" = "DeadLetter" ] || [ "$$last_phase" = "Error" ]; then \
			echo "task $(TASK) terminal phase=$$last_phase (expected Succeeded)"; \
			echo "$$task_json" | jq '.'; \
			exit 1; \
		fi; \
		sleep "$$interval"; \
	done

real-check-anthropic-tool-use:
	@$(MAKE) real-wait-task-succeeded NS=$(ANTHROPIC_DECISION_NS) TASK=$(ANTHROPIC_USE_TASK)
	@$(MAKE) real-check NS=$(ANTHROPIC_DECISION_NS) TASK=$(ANTHROPIC_USE_TASK)
	@set -eu; \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(ANTHROPIC_USE_TASK)?namespace=$(ANTHROPIC_DECISION_NS)"); \
	phase=$$(echo "$$task_json" | jq -r '.status.phase // ""'); \
	tool_calls=$$(echo "$$task_json" | jq -r '(.status.output["agent.1.tool_calls"] | tonumber?) // 0'); \
	trace_tool_calls=$$(echo "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	last_event=$$(echo "$$task_json" | jq -r '.status.output["agent.1.last_event"] // ""'); \
	[ "$$phase" = "Succeeded" ] || { echo "expected phase Succeeded, got $$phase"; exit 1; }; \
	[ "$$tool_calls" -ge 1 ] || { echo "expected agent.1.tool_calls >= 1, got $$tool_calls"; exit 1; }; \
	[ "$$trace_tool_calls" -ge 1 ] || { echo "expected at least one tool_call trace event, got $$trace_tool_calls"; exit 1; }; \
	echo "$$last_event" | grep -qi 'TOOL_USED:[[:space:]]*yes' || { echo "missing TOOL_USED: yes marker in agent output"; exit 1; }; \
	echo "$$last_event" | grep -q 'EVIDENCE:' || { echo "missing EVIDENCE marker in agent output"; exit 1; }; \
	echo "anthropic tool-use gate passed"

real-check-anthropic-tool-no-use:
	@$(MAKE) real-wait-task-succeeded NS=$(ANTHROPIC_DECISION_NS) TASK=$(ANTHROPIC_NO_USE_TASK)
	@$(MAKE) real-check NS=$(ANTHROPIC_DECISION_NS) TASK=$(ANTHROPIC_NO_USE_TASK)
	@set -eu; \
	task_json=$$(curl -sSf "$(API_BASE)/v1/tasks/$(ANTHROPIC_NO_USE_TASK)?namespace=$(ANTHROPIC_DECISION_NS)"); \
	phase=$$(echo "$$task_json" | jq -r '.status.phase // ""'); \
	tool_calls=$$(echo "$$task_json" | jq -r '(.status.output["agent.1.tool_calls"] | tonumber?) // 0'); \
	trace_tool_calls=$$(echo "$$task_json" | jq -r '[.status.trace[]? | select((.type // "") == "tool_call")] | length'); \
	last_event=$$(echo "$$task_json" | jq -r '.status.output["agent.1.last_event"] // ""'); \
	[ "$$phase" = "Succeeded" ] || { echo "expected phase Succeeded, got $$phase"; exit 1; }; \
	[ "$$tool_calls" -eq 0 ] || { echo "expected agent.1.tool_calls == 0, got $$tool_calls"; exit 1; }; \
	[ "$$trace_tool_calls" -eq 0 ] || { echo "expected no tool_call trace events, got $$trace_tool_calls"; exit 1; }; \
	echo "$$last_event" | grep -qi 'TOOL_USED:[[:space:]]*no' || { echo "missing TOOL_USED: no marker in agent output"; exit 1; }; \
	echo "$$last_event" | grep -qi 'EVIDENCE:[[:space:]]*self-contained-input' || { echo "missing self-contained EVIDENCE marker in agent output"; exit 1; }; \
	echo "anthropic no-tool gate passed"

real-gate-anthropic-tool-decision: real-check-anthropic-tool-use real-check-anthropic-tool-no-use
	@echo "anthropic tool-decision gate passed"

real-check-all: real-check-pipeline real-check-hier real-check-loop real-check-tool
