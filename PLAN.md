# Memcached Kubernetes Operator (memcached.c5c3.io)

A Kubernetes operator for managing Memcached clusters, built with Operator SDK (Go) and controller-runtime.
Part of the CobaltCore (C5C3) ecosystem, it fills the gap of a production-ready Memcached operator by providing
declarative management of Memcached instances with automated Deployment, headless Service, PDB, ServiceMonitor,
and NetworkPolicy reconciliation, plus monitoring via memcached-exporter sidecar, high-availability primitives,
security features (SASL, TLS, NetworkPolicies), and validation/defaulting webhooks.

## Phase 1: Project Scaffolding & CRD Foundation

Initialize the Operator SDK project structure and define the Memcached CRD with all spec/status fields.

- **S001: Operator SDK Project Initialization** [high]
  Scaffold the Go operator project using Operator SDK / Kubebuilder with Go 1.24+, controller-runtime, API group memcached.c5c3.io, and initial version v1alpha1. Set up Go module, Makefile, Dockerfile, and directory structure (api/, internal/controller/, config/).
- **S002: Memcached CRD Types (v1alpha1)** [high] (depends on: S001)
  Define the Memcached custom resource types: MemcachedSpec (replicas, image, resources, memcached config block with maxMemoryMB/maxConnections/threads/maxItemSize/verbosity/extraArgs), MemcachedStatus (conditions, readyReplicas, observedGeneration). Include nested structs for highAvailability, monitoring, and security sections.
- **S003: CRD Manifest Generation & Registration** [high] (depends on: S002)
  Generate CRD YAML manifests via controller-gen, register the scheme, and ensure the CRD can be installed on a cluster. Includes kubebuilder markers for validation, defaults, and printer columns.

## Phase 2: Core Reconciliation Loop

Implement the main MemcachedReconciler that manages Deployments and headless Services, forming the operational backbone of the operator.

- **S004: MemcachedReconciler Scaffold & Watch Setup** [high] (depends on: S003)
  Create the MemcachedReconciler struct and SetupWithManager function. Configure watches for the primary Memcached CR and all owned resources (Deployment, Service, PDB, ServiceMonitor, NetworkPolicy) filtered by owner references.
- **S005: Deployment Reconciliation** [high] (depends on: S004)
  Reconcile a Deployment for the Memcached pods. Map CRD spec fields to container args (-m, -c, -t, -I, -v/-vv, extraArgs), set resource requests/limits, configure the memcached:1.6 container image (or user-specified), expose port 11211. Set owner references for garbage collection. Detect drift and update.
- **S006: Headless Service Reconciliation** [high] (depends on: S004)
  Create and reconcile a headless Service (clusterIP: None) on port 11211 for direct pod discovery. This is critical for Keystone's pymemcache backend which connects to individual pod addresses (e.g. memcached-0.memcached:11211).
- **S007: Status Conditions & ObservedGeneration** [high] (depends on: S005, S006)
  Update the Memcached CR status with standard conditions (Available, Progressing, Degraded), readyReplicas count, and observedGeneration. Follow Kubernetes API conventions for condition types and transitions.
- **S008: Idempotent Create-or-Update Logic** [high] (depends on: S004)
  Implement a generic create-or-update pattern (using controllerutil.CreateOrUpdate or equivalent) ensuring every reconciliation produces the same outcome. Handle resource version conflicts with retries. Ensure level-triggered reconciliation reacts to current state, not event sequences.

## Phase 3: High Availability & Operational Features

Add HA primitives including pod anti-affinity, topology spread constraints, PodDisruptionBudgets, and graceful shutdown to ensure production-grade resilience.

- **S009: Pod Anti-Affinity Presets** [medium] (depends on: S005)
  Implement soft (preferredDuringSchedulingIgnoredDuringExecution) and hard (requiredDuringSchedulingIgnoredDuringExecution) pod anti-affinity presets based on spec.highAvailability.antiAffinityPreset. Spread Memcached pods across nodes to avoid single points of failure.
- **S010: Topology Spread Constraints** [medium] (depends on: S005)
  Apply user-defined topologySpreadConstraints from spec.highAvailability.topologySpreadConstraints to the Deployment pod template. Supports zone-aware scheduling (e.g. maxSkew: 1, topologyKey: topology.kubernetes.io/zone).
- **S011: PodDisruptionBudget Reconciliation** [medium] (depends on: S004)
  Create and reconcile a PodDisruptionBudget when spec.highAvailability.podDisruptionBudget.enabled is true. Support minAvailable configuration (default: 1). Set owner references for cleanup.
- **S012: Graceful Shutdown** [medium] (depends on: S005)
  Configure a preStop lifecycle hook and appropriate terminationGracePeriodSeconds so Memcached processes can drain connections before pod termination, preventing cache stampedes during rolling updates.

## Phase 4: Monitoring & Observability

Integrate Prometheus monitoring via a memcached-exporter sidecar and ServiceMonitor, plus expose operator-level metrics.

- **S013: Memcached Exporter Sidecar Injection** [medium] (depends on: S005)
  When spec.monitoring.enabled is true, inject a prom/memcached-exporter:v0.15.4 sidecar container into the Deployment pod template. Expose metrics port 9150. Apply exporter-specific resource requests/limits from spec.monitoring.exporterResources.
- **S014: ServiceMonitor Reconciliation** [medium] (depends on: S006, S013)
  Create and reconcile a Prometheus ServiceMonitor resource when monitoring is enabled. Configure scrape interval, scrapeTimeout, and additionalLabels from spec.monitoring.serviceMonitor. Set owner references for cleanup.
- **S015: Operator Metrics Server** [medium] (depends on: S004)
  Expose operator-level metrics on :8443 /metrics endpoint via controller-runtime's built-in metrics server. Include standard controller metrics (reconciliation duration, queue depth, errors) and custom Memcached operator metrics.

## Phase 5: Security

Implement security hardening: pod security contexts, optional SASL authentication, optional TLS encryption, NetworkPolicy generation, and least-privilege RBAC.

- **S016: Pod Security Contexts** [high] (depends on: S005)
  Apply restrictive pod and container security contexts: runAsNonRoot, readOnlyRootFilesystem, drop ALL capabilities, seccompProfile RuntimeDefault. Follow Kubernetes Pod Security Standards (restricted profile).
- **S017: Optional SASL Authentication** [low] (depends on: S005)
  Support optional SASL authentication for Memcached. When enabled in the CRD spec, configure Memcached with -Y flag and mount credentials from a referenced Secret. Clients must provide SASL credentials to connect.
- **S018: Optional TLS Encryption** [low] (depends on: S005)
  Support optional TLS encryption for Memcached connections. When enabled, configure Memcached with TLS flags, mount TLS certificates from referenced Secrets or cert-manager Certificate resources. Configure port 11212 for TLS alongside or instead of 11211.
- **S019: NetworkPolicy Reconciliation** [medium] (depends on: S004, S006)
  Create and reconcile a NetworkPolicy when enabled in the CRD spec. Restrict ingress to Memcached port (11211) and exporter port (9150) from allowed sources. Set owner references for cleanup. The controller watches NetworkPolicy resources for drift detection.
- **S020: Operator RBAC (Least-Privilege)** [high] (depends on: S001)
  Define ClusterRole and ClusterRoleBinding with minimal permissions: CRUD on Memcached CRs, Deployments, Services, PDBs, ServiceMonitors, NetworkPolicies. Read-only on Secrets (for SASL/TLS). Generate RBAC manifests via kubebuilder markers.

## Phase 6: Webhooks

Implement defaulting and validation webhooks to ensure CRD values are correct and sensible before reconciliation.

- **S021: Defaulting Webhook** [medium] (depends on: S002)
  Implement a mutating admission webhook that sets sensible defaults for omitted fields: replicas=1, image=memcached:1.6, maxMemoryMB=64, maxConnections=1024, threads=4, maxItemSize=1m, monitoring.exporterImage=prom/memcached-exporter:v0.15.4, antiAffinityPreset=soft, etc.
- **S022: Validation Webhook** [medium] (depends on: S002)
  Implement a validating admission webhook that rejects invalid CRD configurations: replicas >= 0, maxMemoryMB > 0, threads > 0, valid maxItemSize format, valid antiAffinityPreset values (soft/hard), consistent resource limits (memory limit >= maxMemoryMB + overhead), valid verbosity range (0-2).
- **S023: Webhook Certificate Management** [medium] (depends on: S021, S022)
  Set up webhook TLS certificate provisioning — either via cert-manager integration or self-signed certificate generation. Configure the webhook server in the operator manager.

## Phase 7: Testing

Comprehensive testing strategy with unit tests, envtest integration tests, and KUTTL/Chainsaw end-to-end tests.

- **S024: Unit Tests for Reconciler Logic** [high] (depends on: S005, S006, S011, S014, S019)
  Write unit tests for the reconciler's resource-building logic: Deployment construction, Service construction, PDB construction, ServiceMonitor construction, NetworkPolicy construction. Test field mapping from CRD spec to Kubernetes resource specs. Mock the Kubernetes client for isolated testing.
- **S025: envtest Integration Tests** [high] (depends on: S007, S008)
  Write integration tests using controller-runtime's envtest framework. Test the full reconciliation loop against a real API server: CR creation triggers Deployment + Service creation, spec updates propagate correctly, deletion cleans up owned resources, status conditions are set correctly.
- **S026: KUTTL / Chainsaw E2E Tests** [medium] (depends on: S025)
  Write end-to-end tests using KUTTL or Chainsaw that run against a real (kind) cluster. Test scenarios: basic deployment, scaling, configuration changes, monitoring toggle, PDB creation, graceful rolling update, webhook rejection of invalid CRs.
- **S027: Webhook Tests** [medium] (depends on: S021, S022)
  Test defaulting and validation webhooks: verify defaults are applied for omitted fields, verify invalid configurations are rejected with clear error messages, verify edge cases in validation logic.

## Phase 8: Packaging, CI & Release

Package the operator for deployment, set up CI pipelines, and prepare release artifacts.

- **S028: Multi-Stage Dockerfile** [high] (depends on: S001)
  Create a multi-stage Dockerfile: build stage with Go 1.24+ compiles the operator binary, runtime stage uses distroless/static base image for minimal attack surface. Include labels for OCI image spec.
- **S029: Kustomize Deployment Manifests** [high] (depends on: S003, S020, S023)
  Organize config/ directory with Kustomize overlays: base (CRD, RBAC, Deployment), default (with webhook and cert-manager), samples (example Memcached CRs). Follow Kubebuilder conventions for config/manager, config/rbac, config/crd, config/webhook.
- **S030: CI Pipeline** [medium] (depends on: S024, S025, S028)
  Set up CI (GitHub Actions) with: lint (golangci-lint), unit tests, envtest integration tests, Docker image build, CRD schema validation. Run on PRs and main branch pushes.
- **S031: Documentation** [medium] (depends on: S029)
  Write operator documentation: installation guide, CRD reference with all fields documented, upgrade guide, troubleshooting, architecture overview. Include example CRs for common use cases (minimal, HA, monitoring-enabled, TLS-enabled).
