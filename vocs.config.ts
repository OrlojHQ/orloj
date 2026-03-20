import { defineConfig } from 'vocs'

export default defineConfig({
  title: 'orloj Docs',
  description: 'Lightweight orchestration plane for agents, tools, policies, and task execution.',
  rootDir: 'docs',
  vite: {
    server: {
      fs: {
        allow: ['..'],
      },
    },
  },
  topNav: [
    { text: 'Getting Started', link: '/getting-started/' },
    { text: 'Deployment', link: '/deployment/' },
    { text: 'Concepts', link: '/concepts/' },
    { text: 'Guides', link: '/guides/' },
    { text: 'Operations', link: '/operations/' },
    { text: 'Reference', link: '/reference/' },
    { text: 'Project', link: '/project/' },
  ],
  sidebar: [
    { text: 'What is Orloj?', link: '/' },
    {
      text: 'Getting Started',
      items: [
        { text: 'Overview', link: '/getting-started/' },
        { text: 'Install', link: '/getting-started/install' },
        { text: 'Quickstart', link: '/getting-started/quickstart' },
      ],
    },
    {
      text: 'Deployment',
      items: [
        { text: 'Overview', link: '/deployment/' },
        { text: 'Local Deployment', link: '/deployment/local' },
        { text: 'VPS Deployment', link: '/deployment/vps' },
        { text: 'Kubernetes Deployment', link: '/deployment/kubernetes' },
        { text: 'Remote CLI and API access', link: '/deployment/remote-cli-access' },
      ],
    },
    {
      text: 'Concepts',
      items: [
        { text: 'Overview', link: '/concepts/' },
        {
          text: 'Architecture',
          items: [
            { text: 'Architecture Overview', link: '/architecture/overview' },
            { text: 'Execution and Messaging', link: '/architecture/execution-model' },
            { text: 'Starter Blueprints', link: '/architecture/starter-blueprints' },
          ],
        },
        {
          text: 'Core resources',
          items: [
            { text: 'Agents and Agent Systems', link: '/concepts/agents-and-systems' },
            { text: 'Tasks and Scheduling', link: '/concepts/tasks-and-scheduling' },
            { text: 'Tools and Isolation', link: '/concepts/tools-and-isolation' },
            { text: 'Model Routing', link: '/concepts/model-routing' },
          ],
        },
        {
          text: 'Memory',
          items: [
            { text: 'Memory', link: '/concepts/memory/' },
            { text: 'Memory providers', link: '/concepts/memory/providers' },
          ],
        },
        { text: 'Governance and Policies', link: '/concepts/governance' },
      ],
    },
    {
      text: 'Guides',
      items: [
        { text: 'Overview', link: '/guides/' },
        { text: 'Deploy Your First Pipeline', link: '/guides/deploy-pipeline' },
        { text: 'Set Up Multi-Agent Governance', link: '/guides/setup-governance' },
        { text: 'Configure Model Routing', link: '/guides/configure-model-routing' },
        { text: 'Connect an MCP Server', link: '/guides/connect-mcp-server' },
        { text: 'Build a Custom Tool', link: '/guides/build-custom-tool' },
      ],
    },
    {
      text: 'Operations',
      items: [
        { text: 'Overview', link: '/operations/' },
        {
          text: 'Day-to-day',
          items: [
            { text: 'Runbook', link: '/operations/runbook' },
            { text: 'Configuration', link: '/operations/configuration' },
            { text: 'Troubleshooting', link: '/operations/troubleshooting' },
            { text: 'Upgrades and Rollbacks', link: '/operations/upgrades' },
            { text: 'Security and Isolation', link: '/operations/security' },
          ],
        },
        {
          text: 'Observability',
          items: [
            { text: 'Observability', link: '/operations/observability' },
            { text: 'Monitoring and Alerts', link: '/operations/monitoring-alerts' },
            { text: 'Backup and Restore', link: '/operations/backup-restore' },
          ],
        },
        {
          text: 'Validation',
          items: [
            { text: 'Tool Runtime Conformance', link: '/operations/tool-runtime-conformance' },
            { text: 'Real Tool Validation', link: '/operations/real-tool-validation' },
            { text: 'Load Testing', link: '/operations/load-testing' },
            { text: 'Live Validation Matrix', link: '/operations/live-validation-matrix' },
          ],
        },
        {
          text: 'Triggers',
          items: [
            { text: 'Task Scheduling (Cron)', link: '/operations/task-scheduling' },
            { text: 'Webhook Triggers', link: '/operations/webhooks' },
          ],
        },
      ],
    },
    {
      text: 'Reference',
      items: [
        { text: 'Overview', link: '/reference/' },
        {
          text: 'API and resources',
          items: [
            { text: 'CLI Reference', link: '/reference/cli' },
            { text: 'API Reference', link: '/reference/api' },
            { text: 'Resource Reference', link: '/reference/resources' },
          ],
        },
        {
          text: 'Contracts',
          items: [
            { text: 'Extension Contracts', link: '/reference/extensions' },
            { text: 'Tool Contract v1', link: '/reference/tool-contract-v1' },
            { text: 'WASM Tool Module Contract v1', link: '/reference/wasm-tool-module-contract-v1' },
          ],
        },
        { text: 'Glossary', link: '/reference/glossary' },
      ],
    },
    {
      text: 'Project',
      items: [
        { text: 'Overview', link: '/project/' },
        {
          text: 'Community',
          items: [
            { text: 'Support', link: '/project/support' },
            { text: 'Project governance', link: '/project/governance' },
            { text: 'Security Policy', link: '/project/security-policy' },
          ],
        },
        {
          text: 'Development',
          items: [
            { text: 'Versioning and Deprecation', link: '/project/versioning-and-deprecation' },
            { text: 'Release Process', link: '/project/release-process' },
            { text: 'Roadmap', link: '/phases/roadmap' },
            { text: 'Phase log', link: '/phases/phase-log' },
          ],
        },
        {
          text: 'Boundaries',
          items: [
            { text: 'Cloud Boundary', link: '/boundaries/agentops-cloud.BOUNDARY' },
            { text: 'Enterprise Boundary', link: '/boundaries/agentops-enterprise.BOUNDARY' },
            { text: 'Plugins Boundary', link: '/boundaries/agentops-plugins.BOUNDARY' },
          ],
        },
      ],
    },
  ],
  socials: [{ icon: 'github', link: 'https://github.com/OrlojHQ/orloj', label: 'GitHub' }],
})
