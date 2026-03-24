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
    { text: "Getting Started", link: "/getting-started/" },
    { text: "Deployment", link: "/deployment/" },
    { text: "Concepts", link: "/concepts/" },
    { text: "Guides", link: "/guides/" },
    { text: "Operations", link: "/operations/" },
    { text: "Reference", link: "/reference/" },
  ],
  sidebar: [
    { text: "What is Orloj?", link: "/" },
    {
      text: "Getting Started",
      items: [
        { text: "Overview", link: "/getting-started/" },
        { text: "Install", link: "/getting-started/install" },
        { text: "Quickstart", link: "/getting-started/quickstart" },
      ],
    },
    {
      text: "Deployment",
      items: [
        { text: "Overview", link: "/deployment/" },
        { text: "Local Deployment", link: "/deployment/local" },
        { text: "VPS Deployment", link: "/deployment/vps" },
        { text: "Kubernetes Deployment", link: "/deployment/kubernetes" },
        {
          text: "Remote CLI and API access",
          link: "/deployment/remote-cli-access",
        },
      ],
    },
    {
      text: "Concepts",
      items: [
        { text: "Overview", link: "/concepts/" },
        {
          text: "Architecture",
          items: [
            { text: "Architecture Overview", link: "/architecture/overview" },
            {
              text: "Execution and Messaging",
              link: "/architecture/execution-model",
            },
            {
              text: "Starter Blueprints",
              link: "/architecture/starter-blueprints",
            },
          ],
        },
        {
          text: "Core resources",
          items: [
            {
              text: "Agents and Agent Systems",
              link: "/concepts/agents-and-systems",
            },
            {
              text: "Tasks and Scheduling",
              link: "/concepts/tasks-and-scheduling",
            },
            {
              text: "Tools and Isolation",
              link: "/concepts/tools-and-isolation",
            },
            { text: "Model Routing", link: "/concepts/model-routing" },
          ],
        },
        {
          text: "Memory",
          items: [
            { text: "Memory", link: "/concepts/memory/" },
            { text: "Memory providers", link: "/concepts/memory/providers" },
          ],
        },
        { text: "Governance and Policies", link: "/concepts/governance" },
      ],
    },
    {
      text: "Guides",
      items: [
        { text: "Overview", link: "/guides/" },
        { text: "Deploy Your First Pipeline", link: "/guides/deploy-pipeline" },
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
      ],
    },
    {
      text: "Operations",
      items: [
        { text: "Overview", link: "/operations/" },
        {
          text: "Day-to-day",
          items: [
            { text: "Runbook", link: "/operations/runbook" },
            { text: "Configuration", link: "/operations/configuration" },
            { text: "Troubleshooting", link: "/operations/troubleshooting" },
            { text: "Upgrades and Rollbacks", link: "/operations/upgrades" },
            { text: "Security and Isolation", link: "/operations/security" },
          ],
        },
        {
          text: "Observability",
          items: [
            { text: "Observability", link: "/operations/observability" },
            {
              text: "Monitoring and Alerts",
              link: "/operations/monitoring-alerts",
            },
            { text: "Backup and Restore", link: "/operations/backup-restore" },
          ],
        },
        {
          text: "Validation",
          items: [
            {
              text: "Tool Runtime Conformance",
              link: "/operations/tool-runtime-conformance",
            },
            {
              text: "Real Tool Validation",
              link: "/operations/real-tool-validation",
            },
            { text: "Load Testing", link: "/operations/load-testing" },
            {
              text: "Live Validation Matrix",
              link: "/operations/live-validation-matrix",
            },
          ],
        },
        {
          text: "Triggers",
          items: [
            {
              text: "Task Scheduling (Cron)",
              link: "/operations/task-scheduling",
            },
            { text: "Webhook Triggers", link: "/operations/webhooks" },
          ],
        },
      ],
    },
    {
      text: "Reference",
      items: [
        { text: "Overview", link: "/reference/" },
        {
          text: "API and resources",
          items: [
            { text: "CLI Reference", link: "/reference/cli" },
            { text: "API Reference", link: "/reference/api" },
            { text: "Resource Reference", link: "/reference/resources" },
          ],
        },
        {
          text: "Contracts",
          items: [
            { text: "Extension Contracts", link: "/reference/extensions" },
            { text: "Tool Contract v1", link: "/reference/tool-contract-v1" },
            {
              text: "WASM Tool Module Contract v1",
              link: "/reference/wasm-tool-module-contract-v1",
            },
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
