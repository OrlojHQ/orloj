package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	blueprint := fs.String("blueprint", "pipeline", "blueprint to scaffold: pipeline, hierarchical, swarm-loop")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return errors.New("usage: orlojctl init <name> [--blueprint pipeline|hierarchical|swarm-loop]")
	}
	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		return errors.New("name cannot be empty")
	}

	bp := strings.ToLower(strings.TrimSpace(*blueprint))
	var files map[string]string
	switch bp {
	case "pipeline":
		files = pipelineBlueprint(name)
	case "hierarchical":
		files = hierarchicalBlueprint(name)
	case "swarm-loop":
		files = swarmLoopBlueprint(name)
	default:
		return fmt.Errorf("unknown blueprint %q (expected: pipeline, hierarchical, swarm-loop)", bp)
	}

	outDir := name
	agentsDir := filepath.Join(outDir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", outDir, err)
	}

	for path, content := range files {
		fullPath := filepath.Join(outDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Printf("  created %s/%s\n", outDir, path)
	}

	fmt.Printf("\nScaffolded %s blueprint %q in %s/\n", bp, name, outDir)
	fmt.Printf("\nTo apply:\n  orlojctl apply -f %s --run\n", outDir)
	return nil
}

func pipelineBlueprint(prefix string) map[string]string {
	p := prefix
	return map[string]string{
		"agents/planner_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-planner-agent
spec:
  model_ref: openai-default
  prompt: |
    You are the planning stage.
    Break the task into concrete research and writing requirements.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"agents/research_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-research-agent
spec:
  model_ref: openai-default
  prompt: |
    You are the research stage.
    Produce concise, verifiable findings for the writer.
  limits:
    max_steps: 6
    timeout: 30s
`, p),
		"agents/writer_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-writer-agent
spec:
  model_ref: openai-default
  prompt: |
    You are the writing stage.
    Synthesize prior handoffs into a polished final output.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"model-endpoint.yaml": `apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: openai-default
spec:
  provider: openai
  base_url: https://api.openai.com/v1
  default_model: gpt-4o
  auth:
    secretRef: openai-api-key
`,
		"agent-system.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: %s-system
  labels:
    orloj.dev/pattern: pipeline
spec:
  agents:
    - %s-planner-agent
    - %s-research-agent
    - %s-writer-agent
  graph:
    %s-planner-agent:
      edges:
        - to: %s-research-agent
    %s-research-agent:
      edges:
        - to: %s-writer-agent
`, p, p, p, p, p, p, p, p),
		"task.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: %s-task
spec:
  system: %s-system
  input:
    topic: your topic here
  priority: high
  retry:
    max_attempts: 2
    backoff: 2s
  message_retry:
    max_attempts: 2
    backoff: 250ms
    max_backoff: 2s
    jitter: full
`, p, p),
	}
}

func hierarchicalBlueprint(prefix string) map[string]string {
	p := prefix
	return map[string]string{
		"agents/manager_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-manager-agent
spec:
  model_ref: openai-default
  prompt: |
    You are the manager. Decompose the task into sub-tasks
    and delegate to your lead agents.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"agents/research_lead_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-research-lead-agent
spec:
  model_ref: openai-default
  prompt: |
    You lead the research track.
    Coordinate your worker to gather findings.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"agents/research_worker_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-research-worker-agent
spec:
  model_ref: openai-default
  prompt: |
    You are a research worker.
    Produce concise, verifiable findings.
  limits:
    max_steps: 6
    timeout: 30s
`, p),
		"agents/social_lead_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-social-lead-agent
spec:
  model_ref: openai-default
  prompt: |
    You lead the social/comms track.
    Coordinate your worker to produce messaging.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"agents/social_worker_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-social-worker-agent
spec:
  model_ref: openai-default
  prompt: |
    You are a social/comms worker.
    Draft messaging and positioning materials.
  limits:
    max_steps: 6
    timeout: 30s
`, p),
		"agents/editor_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-editor-agent
spec:
  model_ref: openai-default
  prompt: |
    You are the editor. Synthesize all track outputs
    into a cohesive final deliverable.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"model-endpoint.yaml": `apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: openai-default
spec:
  provider: openai
  base_url: https://api.openai.com/v1
  default_model: gpt-4o
  auth:
    secretRef: openai-api-key
`,
		"agent-system.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: %s-system
  labels:
    orloj.dev/pattern: hierarchical
spec:
  agents:
    - %s-manager-agent
    - %s-research-lead-agent
    - %s-research-worker-agent
    - %s-social-lead-agent
    - %s-social-worker-agent
    - %s-editor-agent
  graph:
    %s-manager-agent:
      edges:
        - to: %s-research-lead-agent
        - to: %s-social-lead-agent
    %s-research-lead-agent:
      edges:
        - to: %s-research-worker-agent
    %s-social-lead-agent:
      edges:
        - to: %s-social-worker-agent
    %s-research-worker-agent:
      edges:
        - to: %s-editor-agent
    %s-social-worker-agent:
      edges:
        - to: %s-editor-agent
    %s-editor-agent:
      join:
        mode: wait_for_all
`, p, p, p, p, p, p, p, p, p, p, p, p, p, p, p, p, p, p, p),
		"task.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: %s-task
spec:
  system: %s-system
  input:
    topic: your topic here
  priority: high
  retry:
    max_attempts: 2
    backoff: 2s
  message_retry:
    max_attempts: 2
    backoff: 250ms
    max_backoff: 2s
    jitter: full
`, p, p),
	}
}

func swarmLoopBlueprint(prefix string) map[string]string {
	p := prefix
	return map[string]string{
		"agents/coordinator_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-coordinator-agent
spec:
  model_ref: openai-default
  prompt: |
    You are the swarm coordinator. Dispatch exploration tasks
    to scouts and synthesize their findings across iterations.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"agents/scout_alpha_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-scout-alpha-agent
spec:
  model_ref: openai-default
  prompt: |
    You are scout alpha. Explore your assigned area
    and report findings back to the coordinator.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"agents/scout_beta_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-scout-beta-agent
spec:
  model_ref: openai-default
  prompt: |
    You are scout beta. Explore your assigned area
    and report findings back to the coordinator.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"agents/scout_gamma_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-scout-gamma-agent
spec:
  model_ref: openai-default
  prompt: |
    You are scout gamma. Explore your assigned area
    and report findings back to the coordinator.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"agents/synthesizer_agent.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: %s-synthesizer-agent
spec:
  model_ref: openai-default
  prompt: |
    You are the synthesizer. Produce a final summary
    from the coordinator's accumulated findings.
  limits:
    max_steps: 4
    timeout: 20s
`, p),
		"model-endpoint.yaml": `apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: openai-default
spec:
  provider: openai
  base_url: https://api.openai.com/v1
  default_model: gpt-4o
  auth:
    secretRef: openai-api-key
`,
		"agent-system.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: %s-system
  labels:
    orloj.dev/pattern: swarm-loop
spec:
  agents:
    - %s-coordinator-agent
    - %s-scout-alpha-agent
    - %s-scout-beta-agent
    - %s-scout-gamma-agent
    - %s-synthesizer-agent
  graph:
    %s-coordinator-agent:
      edges:
        - to: %s-scout-alpha-agent
        - to: %s-scout-beta-agent
        - to: %s-scout-gamma-agent
        - to: %s-synthesizer-agent
    %s-scout-alpha-agent:
      edges:
        - to: %s-coordinator-agent
    %s-scout-beta-agent:
      edges:
        - to: %s-coordinator-agent
    %s-scout-gamma-agent:
      edges:
        - to: %s-coordinator-agent
`, p, p, p, p, p, p, p, p, p, p, p, p, p, p, p, p, p),
		"task.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: %s-task
spec:
  system: %s-system
  input:
    topic: your topic here
  max_turns: 4
  priority: high
  retry:
    max_attempts: 2
    backoff: 2s
  message_retry:
    max_attempts: 2
    backoff: 250ms
    max_backoff: 2s
    jitter: full
`, p, p),
	}
}
