# OSAC operator

Kubernetes controllers for OSAC resources, provisioning through AAP, feedback
to the fulfillment service, and the KubeVirt console proxy.

This component is part of the OSAC monorepo, not an isolated project. Its APIs,
generated artifacts, deployment configuration, and runtime behavior may affect
other components. Apply the repository-wide rules in
[`../AGENTS.md`](../AGENTS.md), consider downstream consumers before changing
behavior, and follow the instructions for every affected component.

## Required context

Before changing this component, identify the documents relevant to the change
below, then read and follow them. These documents are authoritative for their
respective areas.

- Component setup and architecture: [`README.md`](README.md)
- Cross-component contracts: [`../docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md) and [`../docs/CONVENTIONS.md`](../docs/CONVENTIONS.md)
- Controller-specific examples: neighboring files in `internal/controller/`
- API consumer changes: [`../fulfillment-service/AGENTS.md`](../fulfillment-service/AGENTS.md)

## Invariants

- Resource controllers generally own provisioning, finalizers, and lifecycle status; feedback controllers synchronize state with the fulfillment service.
- Every resource controller except `tenant_controller.go` must skip reconciliation when `osac.openshift.io/management-state` is `Unmanaged`.
- `StorageReconciler` is an intentional exception to the dual-controller pattern: it reconciles Tenant storage and does not own a separate CRD.
- Preserve tenant namespace isolation and established predicates when creating or watching resources.
- When changing shared controller behavior, inspect every controller using the same lifecycle.
- `pkg/provisioning`, `pkg/aap`, and `pkg/dispatcher` have external consumers, including the bare-metal fulfillment operator; interface changes are cross-component changes.
- When debugging operators, check for stale `vendor/` dependencies and cached images before rebuilding.
- Keep generated fulfillment clients compatible with the private API inputs in `buf.gen.yaml`; regenerate when those inputs change.
- Never put credentials in logs, samples, or manifests.

## Generated files

- After changing `api/v1alpha1/*_types.go`, run `make manifests generate`.
- Then run `make helm-crds` to synchronize `config/crd/` with `charts/operator-crds/`; use `make check-helm-crds` to verify the result.
- After changing private fulfillment protos consumed by this component, run `buf generate` from `osac-operator/` and commit the resulting `internal/api/` changes.
- Never hand-edit `config/crd/`, `zz_generated.deepcopy.go`, `internal/api/`, or `go.sum`; run `go mod tidy` for module changes.

## Integration Testing

### Test tiers and commands

| Tier | Location / command | Exercises for real | Faked or omitted |
|---|---|---|---|
| Unit | Co-located `*_test.go`; `make test` | Controller helpers, validation, provisioning state logic | External APIs and providers are mocked. |
| Envtest | Controller tests under `internal/controller/*_envtest_test.go`; run by `make test` | Kubernetes API server, etcd, loaded CRDs, and in-process reconciliation | The controller is not deployed to Kind; provisioning uses controllable or noop providers. |
| Component integration | `test/integration/`; deploy the current operator into a Kind cluster, then run `make integration-tests` | Installed operator, Kubernetes API, CRDs, controller-manager, console proxy, and networking behavior | AAP/provider provisioning and external infrastructure are not real in the current suite; some tests remove finalizers to bypass that boundary. |
| Component integration (CI) | `make -C osac-installer test PLATFORM=kind PROFILE=dev NS=osac SUITE=operator` | The thin Kind deployment used by the PR workflow | The same external-provider limitations as the local Kind suite. |
| Contract | `test/contract/`; included by `make test` | Helm chart RBAC templates against the operator permission contract | No deployed operator or external provider is exercised. |
| E2E | `../tests/e2e/` | Cross-component fulfillment journeys | Depends on the deployed test environment and its configured providers. |

### Touched-area requirements

| Touched area | Minimum required tier | Required command | Notes |
|---|---|---|---|
| Pure helpers, validation, or state calculations | Unit | `make test` | Add error and edge-case coverage. |
| Controller reconciliation, finalizers, status, or CRD interactions | Envtest | `make test` | The envtest suite must exercise the changed lifecycle through the public reconciler behavior. |
| Controller deployment, watches, RBAC, console proxy, networking, or Helm wiring | Component integration | Deploy the current image/manifests, then `make integration-tests`, or use the installer `SUITE=operator` command | Unit/envtest coverage alone does not prove deployed wiring. |
| AAP, dispatcher, provisioning-provider, KubeVirt, or fulfillment boundary | Contract or E2E | Relevant contract/E2E command | A controllable provider in envtest is not coverage of the real provider boundary. |
| Generated CRDs or manifests | Envtest plus applicable Kind suite | `make manifests generate helm-crds check-helm-crds`, then the required test command | Do not hand-edit generated output. |

### Coverage gaps

The current component integration suite does not exercise real AAP,
OpenStack, KubeVirt, or hardware provisioning. Changes to those boundaries
must be covered by a contract or real-provider suite owned by the relevant
OSAC-4843 follow-up task; they cannot be marked covered by envtest alone.

## Validation

From `osac-operator/`:

```bash
make fmt
make lint
make test
make helm-lint
make check-helm-crds
make integration-tests       # Requires a pre-existing Kind cluster
```

`make build` also runs the unit-test path. The console proxy integration tests
are under `test/integration/`; E2E suites are under [`../tests/e2e/`](../tests/e2e/).
