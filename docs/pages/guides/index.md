# Guides

Step-by-step tutorials for common Orloj workflows.

**Start here:** **[Your First Agent System in 5 Minutes](./five-minute-tutorial.md)** — install `orlojctl` and `orlojd`, scaffold a pipeline with `orlojctl init`, add an API key, apply manifests, and run a task end-to-end with the web console.

Other guides use real manifests from the `examples/` directory or walk through authoring resources by hand. For **ready-made scenario folders** (full YAML sets you can copy into your environment), see [examples/use-cases/](https://github.com/OrlojHQ/orloj/tree/main/examples/use-cases).

If you have not installed Orloj yet, begin with [Install](../getting-started/install.md). Developers building from a clone may prefer the [Quickstart](../getting-started/quickstart.md) (`go run` + checked-in blueprints).

## Available Guides

**[Your First Agent System in 5 Minutes](./five-minute-tutorial.md)**
*The fastest path from zero to a running multi-agent pipeline with a real model.*

**[Deploy Your First Pipeline](./deploy-pipeline.md)**
*For platform engineers who want to run a multi-agent pipeline end-to-end.*
Walk through the pipeline blueprint: define three agents (planner, researcher, writer), wire them into a sequential graph, submit a task, and inspect the results.

**[Set Up Multi-Agent Governance](./setup-governance.md)**
*For platform engineers who need to enforce tool authorization and model constraints.*
Create policies, roles, and tool permissions. Deploy a governed agent system and verify that unauthorized tool calls are denied.

**[Configure Model Routing](./configure-model-routing.md)**
*For platform engineers who need to route agents to different model providers.*
Set up ModelEndpoints for OpenAI and Anthropic, bind agents to endpoints by reference, and verify that requests route correctly.

**[Connect an MCP Server](./connect-mcp-server.md)**
*For platform engineers who want to integrate MCP-compatible tool servers.*
Register an MCP server (stdio or HTTP), verify tool discovery, filter imported tools, and assign them to agents.

**[Build a Custom Tool](./build-custom-tool.md)**
*For developers who need to extend agent capabilities with external tools.*
Implement the Tool Contract v1, register the tool as a resource, configure isolation and retry, and validate with the conformance harness.
