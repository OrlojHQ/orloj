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
        { text: 'Production Checklist', link: '/getting-started/production-checklist' },
      ],
    },
    {
      text: 'Deployment',
      items: [
        { text: 'Overview', link: '/deployment/' },
        { text: 'Local Deployment', link: '/deployment/local' },
        { text: 'VPS Deployment', link: '/deployment/vps' },
        { text: 'Kubernetes Deployment', link: '/deployment/kubernetes' },
      ],
    },
    {
      text: 'Concepts',
      items: [
        { text: 'Overview', link: '/concepts/' },
        { text: 'Architecture Overview', link: '/architecture/overview' },
        { text: 'Agents and Agent Systems', link: '/concepts/agents-and-systems' },
        { text: 'Tasks and Scheduling', link: '/concepts/tasks-and-scheduling' },
        { text: 'Execution and Messaging', link: '/architecture/execution-model' },
        { text: 'Tools and Isolation', link: '/concepts/tools-and-isolation' },
        { text: 'Model Routing', link: '/concepts/model-routing' },
        { text: 'Governance and Policies', link: '/concepts/governance' },
        { text: 'Starter Blueprints', link: '/architecture/starter-blueprints' },
      ],
    },
    {
      text: 'Guides',
      items: [
        { text: 'Overview', link: '/guides/' },
        { text: 'Deploy Your First Pipeline', link: '/guides/deploy-pipeline' },
        { text: 'Set Up Multi-Agent Governance', link: '/guides/setup-governance' },
        { text: 'Configure Model Routing', link: '/guides/configure-model-routing' },
        { text: 'Build a Custom Tool', link: '/guides/build-custom-tool' },
      ],
    },
    {
      text: 'Operations',
      items: [
        { text: 'Overview', link: '/operations/' },
        { text: 'Runbook', link: '/operations/runbook' },
        { text: 'Configuration', link: '/operations/configuration' },
        { text: 'Troubleshooting', link: '/operations/troubleshooting' },
        { text: 'Upgrades and Rollbacks', link: '/operations/upgrades' },
        { text: 'Task Scheduling (Cron)', link: '/operations/task-scheduling' },
        { text: 'Webhook Triggers', link: '/operations/webhooks' },
        { text: 'Security and Isolation', link: '/operations/security' },
        { text: 'Load Testing', link: '/operations/load-testing' },
        { text: 'Monitoring and Alerts', link: '/operations/monitoring-alerts' },
        { text: 'Tool Runtime Conformance', link: '/operations/tool-runtime-conformance' },
        { text: 'Real Tool Validation', link: '/operations/real-tool-validation' },
      ],
    },
    {
      text: 'Reference',
      items: [
        { text: 'Overview', link: '/reference/' },
        { text: 'CLI Reference', link: '/reference/cli' },
        { text: 'API Reference', link: '/reference/api' },
        { text: 'Resource Reference', link: '/reference/crds' },
        { text: 'Extension Contracts', link: '/reference/extensions' },
        { text: 'Tool Contract v1', link: '/reference/tool-contract-v1' },
        { text: 'WASM Tool Module Contract v1', link: '/reference/wasm-tool-module-contract-v1' },
        { text: 'Glossary', link: '/reference/glossary' },
      ],
    },
    {
      text: 'Project',
      items: [
        { text: 'Overview', link: '/project/' },
        { text: 'Support', link: '/project/support' },
        { text: 'Governance', link: '/project/governance' },
        { text: 'Security Policy', link: '/project/security-policy' },
        { text: 'Versioning and Deprecation', link: '/project/versioning-and-deprecation' },
        { text: 'Release Process', link: '/project/release-process' },
        { text: 'Cloud Boundary', link: '/boundaries/agentops-cloud.BOUNDARY' },
        { text: 'Enterprise Boundary', link: '/boundaries/agentops-enterprise.BOUNDARY' },
        { text: 'Plugins Boundary', link: '/boundaries/agentops-plugins.BOUNDARY' },
      ],
    },
  ],
  socials: [{ icon: 'github', link: 'https://github.com/OrlojHQ/orloj', label: 'GitHub' }],
})
