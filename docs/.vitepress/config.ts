import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Memcached Operator',
  description: 'A Kubernetes operator for managing Memcached clusters, built with Operator SDK and controller-runtime',
  base: '/memcached-operator/',

  themeConfig: {
    search: {
      provider: 'local'
    },

    nav: [
      { text: 'Home', link: '/' },
      { text: 'Explanation', link: '/explanation/architecture-overview' },
      { text: 'How-To', link: '/how-to/installation' },
      { text: 'Reference', link: '/reference/crd-reference' }
    ],

    sidebar: [
      {
        text: 'Explanation',
        items: [
          { text: 'Architecture Overview', link: '/explanation/architecture-overview' }
        ]
      },
      {
        text: 'How-To Guides',
        items: [
          { text: 'Installation', link: '/how-to/installation' },
          { text: 'Upgrade', link: '/how-to/upgrade' },
          { text: 'Troubleshooting', link: '/how-to/troubleshooting' },
          { text: 'Examples', link: '/how-to/examples' }
        ]
      },
      {
        text: 'Reference',
        items: [
          { text: 'CRD Reference', link: '/reference/crd-reference' },
          {
            text: 'Backend Reference',
            collapsed: true,
            items: [
              { text: 'Project Structure', link: '/reference/backend/project-structure' },
              { text: 'Memcached CRD Types', link: '/reference/backend/memcached-crd-types' },
              { text: 'CRD Generation & Registration', link: '/reference/backend/crd-generation-registration' },
              { text: 'Reconciler Scaffold & Watches', link: '/reference/backend/reconciler-scaffold-watches' },
              { text: 'Deployment Reconciliation', link: '/reference/backend/deployment-reconciliation' },
              { text: 'Headless Service Reconciliation', link: '/reference/backend/headless-service-reconciliation' },
              { text: 'PDB Reconciliation', link: '/reference/backend/pdb-reconciliation' },
              { text: 'HPA Reconciliation', link: '/reference/backend/hpa-reconciliation' },
              { text: 'ServiceMonitor Reconciliation', link: '/reference/backend/servicemonitor-reconciliation' },
              { text: 'NetworkPolicy Reconciliation', link: '/reference/backend/networkpolicy-reconciliation' },
              { text: 'Status Conditions & ObservedGeneration', link: '/reference/backend/status-conditions-observedgeneration' },
              { text: 'Idempotent Create-or-Update', link: '/reference/backend/idempotent-create-or-update' },
              { text: 'Pod Anti-Affinity Presets', link: '/reference/backend/pod-anti-affinity-presets' },
              { text: 'Topology Spread Constraints', link: '/reference/backend/topology-spread-constraints' },
              { text: 'Graceful Shutdown', link: '/reference/backend/graceful-shutdown' },
              { text: 'Exporter Sidecar Injection', link: '/reference/backend/exporter-sidecar-injection' },
              { text: 'Operator Metrics Server', link: '/reference/backend/operator-metrics-server' },
              { text: 'Pod Security Contexts', link: '/reference/backend/pod-security-contexts' },
              { text: 'TLS Encryption', link: '/reference/backend/tls-encryption' },
              { text: 'Operator RBAC (Least Privilege)', link: '/reference/backend/operator-rbac-least-privilege' },
              { text: 'Defaulting Webhook', link: '/reference/backend/defaulting-webhook' },
              { text: 'Validation Webhook', link: '/reference/backend/validation-webhook' },
              { text: 'Webhook Certificate Management', link: '/reference/backend/webhook-certificate-management' },
              { text: 'Webhook Tests', link: '/reference/backend/webhook-tests' },
              { text: 'envtest Integration Tests', link: '/reference/backend/envtest-integration-tests' },
              { text: 'Chainsaw E2E Tests', link: '/reference/backend/chainsaw-e2e-tests' },
              { text: 'CI Pipeline', link: '/reference/backend/ci-pipeline' },
              { text: 'Release Workflow', link: '/reference/backend/release-workflow' },
              { text: 'Multi-Stage Dockerfile', link: '/reference/backend/multi-stage-dockerfile' },
              { text: 'Kustomize Deployment Manifests', link: '/reference/backend/kustomize-deployment-manifests' }
            ]
          }
        ]
      }
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/c5c3/memcached-operator' }
    ]
  }
})
