# Bare-metal fulfillment operator

Kubernetes controllers for `BareMetalPool` and `BareMetalInstance` resources,
including inventory allocation, power management, and profile workflows.

This component is part of the OSAC monorepo, not an isolated project. Its APIs,
generated artifacts, deployment configuration, and runtime behavior may affect
other components. Apply the repository-wide rules in
[`../AGENTS.md`](../AGENTS.md), consider downstream consumers before changing
behavior, and follow the instructions for every affected component.

## Required context

Before changing this component, identify the documents relevant to the change
below, then read and follow them. These documents are authoritative for their
respective areas.

- Component setup: [`README.md`](README.md)
- Controller lifecycle examples: `internal/controller/`
- Cross-component deployment contracts: [`../docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md)

## Invariants

- Preserve the pool-to-instance ownership, finalizer, provisioning, and status lifecycle.
- Keep inventory allocation and power-management abstractions separate.
- Preserve tenant isolation metadata on tenant-scoped resources and avoid credentials in logs or samples.
- Check the sibling controller when changing shared reconciliation behavior.

## Generated files

- After changing `api/v1alpha1/*_types.go`, run `make manifests generate`.
- Then run `make helm-crds` to synchronize the CRD and operator Helm charts; use `make check-helm-crds` to verify synchronization.
- Never hand-edit `config/crd/` or `zz_generated.deepcopy.go`.
- After dependency changes, run `go mod tidy` and commit the resulting `go.mod` and `go.sum` changes.

## Integration Testing

### Test tiers and commands

| Tier | Location / command | Exercises for real | Faked or omitted |
|---|---|---|---|
| Unit | Co-located `*_test.go`; `make test` | Allocation, lifecycle, client, and provider logic in isolation | Kubernetes and external provider APIs are mocked or intercepted. |
| Envtest | Controller tests under `internal/controller/`; currently named `*_integration_test.go` and run by `make test` | Kubernetes API server, etcd, OSAC CRDs, and static Metal3 CRDs | Metal3 controller, Ironic/BMC, hardware, and some provider clients are faked; current filenames are addressed by OSAC-4837. |
| Component integration | `test/integration/`; deploy the current operator into a Kind cluster, then run `make integration-tests` | Deployed operator behavior, CRDs, Kubernetes API, pool/instance flows, and status transitions | The suite creates static `BareMetalHost` state and simulates provider transitions; it does not run a real Metal3 operator, Ironic, BMC, or hardware. |
| CI component integration | `make -C osac-installer test PLATFORM=kind PROFILE=dev NS=osac SUITE=bmf` | The thin Kind deployment used by the PR workflow | Same static Metal3/provider boundary as the local suite. |
| E2E | Cross-component OSAC E2E suites | Fulfillment-to-operator user journeys where the environment provides them | Real hardware and provider availability remain environment-dependent. |

### Touched-area requirements

| Touched area | Minimum required tier | Required command | Notes |
|---|---|---|---|
| Pure inventory, selection, validation, or client logic | Unit | `make test` | Cover success, no-match, and provider-error paths. |
| Reconciliation, finalizers, allocation, or status transitions | Envtest | `make test` | Use the public reconciler behavior and the appropriate CRD fixtures. |
| Controller deployment, CRDs, pool flows, or Kubernetes wiring | Component integration | Deploy the current image/manifests, then `make integration-tests`, or use the installer `SUITE=bmf` command | Envtest alone does not prove the deployed controller path. |
| Metal3, BCM, Ironic, BMC, power, or hardware semantics | Contract or real-provider integration | Follow the owning OSAC-4843 task | Static CRDs and HTTP test doubles do not satisfy a real-boundary requirement. |
| Generated CRDs or Helm CRDs | Envtest plus Kind | `make manifests generate helm-crds check-helm-crds`, then the required test command | Keep generated artifacts synchronized. |

### Coverage gaps

The current Kind suite deliberately stops at static Metal3 resources and
simulated provider status. Work that changes the real Metal3/Ironic/BCM/BMC
boundary must add the qualifying coverage under OSAC-4844 or its contract-test
follow-up; extending the existing static-fixture suite alone is insufficient.

## Validation

From `bare-metal-fulfillment-operator/`:

```bash
make fmt && make vet
make lint
make test
make helm-lint
make check-helm-crds
make integration-tests       # Requires a pre-existing Kind cluster
```

The integration tests are under `test/integration/`. Installer orchestration,
when needed, is owned by [`../osac-installer/AGENTS.md`](../osac-installer/AGENTS.md).
