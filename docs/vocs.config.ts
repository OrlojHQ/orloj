import { defineConfig } from "vocs";

// Optional: set for production deploys (e.g. GitHub Actions manual Pages workflow).
// VOCS_BASE_URL — public origin, e.g. https://docs.example.com (canonical / OG URLs).
// VOCS_BASE_PATH — path prefix if the site is not at the domain root, e.g. /orloj for
// https://org.github.io/orloj/. Leave unset for a custom domain serving the site at /.
const vocsBasePath = process.env.VOCS_BASE_PATH?.trim();
const vocsBaseUrl = process.env.VOCS_BASE_URL?.trim();

export default defineConfig({
  ...(vocsBasePath ? { basePath: vocsBasePath } : {}),
  ...(vocsBaseUrl ? { baseUrl: vocsBaseUrl } : {}),
  iconUrl: "/favicon.png",
  title: "Orloj Docs",
  description: "Runtime, governance, and orchestration for agent systems.",
  rootDir: ".",
  topNav: [
    { text: "Getting Started", link: "/getting-started/install" },
    { text: "Concepts", link: "/concepts/architecture" },
    { text: "Guides", link: "/guides/" },
    { text: "Deploy & Operate", link: "/deploy/" },
    { text: "Reference", link: "/reference/cli" },
  ],
  sidebar: [
    { text: "What is Orloj?", link: "/" },

    {
      text: "Getting Started",
      items: [
        { text: "Install", link: "/getting-started/install" },
        { text: "Quickstart", link: "/getting-started/quickstart" },
        { text: "Core Concepts", link: "/getting-started/core-concepts" },
      ],
    },

    {
      text: "Concepts",
      items: [
        { text: "Architecture Overview", link: "/concepts/architecture" },
        {
          text: "Execution & Messaging",
          link: "/concepts/execution-model",
        },
        {
          text: "Agents & Orchestration",
          collapsed: false,
          items: [
            { text: "Agent", link: "/concepts/agents/agent" },
            { text: "AgentSystem", link: "/concepts/agents/agent-system" },
          ],
        },
        {
          text: "Tasks & Triggers",
          collapsed: false,
          items: [
            { text: "Task", link: "/concepts/tasks/task" },
            { text: "TaskSchedule", link: "/concepts/tasks/task-schedule" },
            { text: "TaskWebhook", link: "/concepts/tasks/task-webhook" },
          ],
        },
        {
          text: "Tools & Models",
          collapsed: false,
          items: [
            { text: "Tool", link: "/concepts/tools/tool" },
            { text: "ModelEndpoint", link: "/concepts/tools/model-endpoint" },
            { text: "McpServer", link: "/concepts/tools/mcp-server" },
            { text: "Secret", link: "/concepts/tools/secret" },
          ],
        },
        {
          text: "Memory",
          collapsed: false,
          items: [
            { text: "Memory", link: "/concepts/memory/" },
            { text: "Memory Providers", link: "/concepts/memory/providers" },
          ],
        },
        {
          text: "Governance",
          collapsed: false,
          items: [
            { text: "Overview", link: "/concepts/governance/" },
            { text: "AgentPolicy", link: "/concepts/governance/agent-policy" },
            { text: "AgentRole", link: "/concepts/governance/agent-role" },
            {
              text: "ToolPermission",
              link: "/concepts/governance/tool-permission",
            },
            {
              text: "ToolApproval",
              link: "/concepts/governance/tool-approval",
            },
          ],
        },
        {
          text: "Infrastructure",
          collapsed: false,
          items: [
            { text: "Worker", link: "/concepts/infrastructure/worker" },
          ],
        },
      ],
    },

    {
      text: "Guides",
      items: [
        { text: "Overview", link: "/guides/" },
        {
          text: "Deploy Your First Pipeline",
          link: "/guides/deploy-pipeline",
        },
        {
          text: "Set Up Multi-Agent Governance",
          link: "/guides/setup-governance",
        },
        {
          text: "Configure Model Routing",
          link: "/guides/configure-model-routing",
        },
        { text: "Connect an MCP Server", link: "/guides/connect-mcp-server" },
        { text: "Build a Custom Tool", link: "/guides/build-custom-tool" },
        { text: "Starter Blueprints", link: "/guides/starter-blueprints" },
      ],
    },

    {
      text: "Deploy & Operate",
      items: [
        { text: "Overview", link: "/deploy/" },
        {
          text: "Deployment",
          collapsed: false,
          items: [
            { text: "Local Development", link: "/deploy/local" },
            { text: "VPS", link: "/deploy/vps" },
            { text: "Kubernetes", link: "/deploy/kubernetes" },
            {
              text: "Remote CLI & API Access",
              link: "/deploy/remote-cli-access",
            },
          ],
        },
        {
          text: "Day-to-Day",
          collapsed: false,
          items: [
            { text: "Configuration", link: "/operations/configuration" },
            { text: "Runbook", link: "/operations/runbook" },
            { text: "Security", link: "/operations/security" },
            {
              text: "Upgrades & Rollbacks",
              link: "/operations/upgrades",
            },
            {
              text: "Task Scheduling (Cron)",
              link: "/operations/task-scheduling",
            },
            { text: "Webhook Triggers", link: "/operations/webhooks" },
            { text: "Troubleshooting", link: "/operations/troubleshooting" },
          ],
        },
        {
          text: "Observability",
          collapsed: false,
          items: [
            { text: "Observability", link: "/operations/observability" },
            {
              text: "Monitoring & Alerts",
              link: "/operations/monitoring-alerts",
            },
            {
              text: "Backup & Restore",
              link: "/operations/backup-restore",
            },
          ],
        },
      ],
    },

    {
      text: "Reference",
      items: [
        { text: "CLI", link: "/reference/cli" },
        { text: "API", link: "/reference/api" },
        {
          text: "Resources",
          collapsed: false,
          items: [
            { text: "Overview", link: "/reference/resources/" },
            { text: "Agent", link: "/reference/resources/agent" },
            {
              text: "AgentSystem",
              link: "/reference/resources/agent-system",
            },
            { text: "Task", link: "/reference/resources/task" },
            {
              text: "TaskSchedule",
              link: "/reference/resources/task-schedule",
            },
            {
              text: "TaskWebhook",
              link: "/reference/resources/task-webhook",
            },
            { text: "Tool", link: "/reference/resources/tool" },
            {
              text: "ModelEndpoint",
              link: "/reference/resources/model-endpoint",
            },
            { text: "McpServer", link: "/reference/resources/mcp-server" },
            { text: "Memory", link: "/reference/resources/memory" },
            { text: "Secret", link: "/reference/resources/secret" },
            {
              text: "AgentPolicy",
              link: "/reference/resources/agent-policy",
            },
            { text: "AgentRole", link: "/reference/resources/agent-role" },
            {
              text: "ToolPermission",
              link: "/reference/resources/tool-permission",
            },
            {
              text: "ToolApproval",
              link: "/reference/resources/tool-approval",
            },
            { text: "Worker", link: "/reference/resources/worker" },
          ],
        },
        { text: "Glossary", link: "/reference/glossary" },
      ],
    },

  ],
  socials: [
    {
      icon: "github",
      link: "https://github.com/OrlojHQ/orloj",
      label: "GitHub",
    },
    {
      icon: "discord",
      link: "https://discord.gg/a6bJmPwGS",
      label: "Discord",
    },
  ],
});
