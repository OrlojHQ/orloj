import { defineConfig } from 'vocs'

export default defineConfig({
  title: 'orloj Docs',
  description: 'Kubernetes-style control plane for agents, tools, policies, and task execution.',
  rootDir: 'docs',
  topNav: [
    { text: 'Getting Started', link: '/getting-started/' },
    { text: 'Concepts', link: '/concepts/' },
    { text: 'Operations', link: '/operations/' },
    { text: 'Reference', link: '/reference/' },
    { text: 'Project', link: '/project/' },
    { text: 'Roadmap', link: '/phases/roadmap' },
  ],
  sidebar: [
    { text: 'Overview', link: '/' },
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
      text: 'Concepts',
      items: [
        { text: 'Overview', link: '/concepts/' },
        { text: 'Architecture Overview', link: '/architecture/overview' },
        { text: 'Execution and Messaging', link: '/architecture/execution-model' },
        { text: 'Starter Blueprints', link: '/architecture/starter-blueprints' },
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
        { text: 'CRD Reference', link: '/reference/crds' },
        { text: 'Extension Contracts', link: '/reference/extensions' },
        { text: 'Tool Contract v1', link: '/reference/tool-contract-v1' },
        { text: 'WASM Tool Module Contract v1', link: '/reference/wasm-tool-module-contract-v1' },
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
        { text: 'Roadmap', link: '/phases/roadmap' },
        { text: 'Phase Log', link: '/phases/phase-log' },
      ],
    },
    {
      text: 'Repository Boundaries',
      items: [
        { text: 'Cloud Boundary', link: '/boundaries/agentops-cloud.BOUNDARY' },
        { text: 'Enterprise Boundary', link: '/boundaries/agentops-enterprise.BOUNDARY' },
        { text: 'Plugins Boundary', link: '/boundaries/agentops-plugins.BOUNDARY' },
      ],
    },
  ],
  socials: [{ icon: 'github', link: 'https://github.com/OrlojHQ/orloj', label: 'GitHub' }],
})
