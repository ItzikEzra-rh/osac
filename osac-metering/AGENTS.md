# OSAC metering

Collects fulfillment lifecycle events, publishes CloudEvents to Kafka, and
provides provider adapters for downstream billing integrations.

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
- Event schema: `schema/`
- Producer: `metering-service/`
- Adapter runner and contracts: `adapters/`
- Deployment values: `charts/osac-metering/values.yaml`

## Invariants

- `schema/`, `metering-service/`, and `adapters/` are separate Go modules; test the module you change.
- Changes under `schema/` affect both `metering-service/` and `adapters/`; run the root `make test` after schema changes.
- Metering events use the shared CloudEvents schema and preserve resource transition ordering.
- Kafka offsets are committed only after successful processing and flush.
- Preserve deduplication, ordering, retry, and DLQ behavior in the shared adapter `Runner`; concrete adapters must not reimplement it.
- A DLQ send failure must not silently acknowledge the source event.
- New billing integrations implement `ProviderAdapter` and use the shared runner lifecycle.
- Keep Kafka credentials and API keys out of logs, fixtures, examples, and manifests.

## Integration Testing

### Test tiers and commands

| Tier | Location / command | Exercises for real | Faked or omitted |
|---|---|---|---|
| Unit | Co-located Ginkgo tests in `schema/`, `metering-service/`, and `adapters/`; `make test` | Schema, mapping, runner, retry, ordering, and adapter behavior in-process | Kafka, fulfillment Watch, and most external services are mocked. |
| Database integration | `metering-service/internal/projection/postgres_test.go`; included by `make test` | A real PostgreSQL testcontainer, schema, persistence, versioning, and queries | Kafka and fulfillment event delivery are not exercised. `SKIP_DB_TESTS` disables this tier. |
| Component integration | No dedicated real-Kafka component suite currently exists | — | Kafka, CloudEvents delivery, fulfillment Watch, offset commits, retries, and DLQ behavior are currently tested with mocks. |
| E2E | Cross-component OSAC metering/E2E deployment | The deployed metering pipeline and its configured Kafka/provider dependencies | Depends on the installer environment and enabled metering path. |

### Touched-area requirements

| Touched area | Minimum required tier | Required command | Notes |
|---|---|---|---|
| Event schema or transition mapping | Unit across affected modules | `make test` | Schema changes affect `schema/`, `metering-service/`, and `adapters/`. |
| Projection/database code | Database integration | `make test` | Do not set `SKIP_DB_TESTS` when validating database behavior. |
| Kafka producer/consumer, CloudEvents transport, offsets, retries, or DLQ | Component integration | Required suite is currently unavailable; track OSAC-4846 | Mock Kafka tests alone do not prove the pipeline boundary. |
| Fulfillment Watch or gRPC event ingestion | Contract or component integration | Required suite is currently unavailable; track the relevant OSAC-4843 task | Mock streams validate local handling, not the wire contract. |
| Provider adapters | Unit plus component/E2E coverage for the provider boundary | `make test` and the qualifying provider suite | The shared runner must remain the owner of ordering, retry, deduplication, and DLQ behavior. |

### Coverage gaps

There is no component-level suite that runs the full fulfillment Watch → Kafka
→ CloudEvents pipeline. Changes to that path must not claim integration
coverage from mock-based tests; add or extend the real-Kafka coverage under
OSAC-4846.

## Generated files

- After changing private fulfillment protos consumed by `metering-service`, run `make generate` from `metering-service/` and commit the resulting `internal/api/` changes; never edit generated client code manually.
- Use `go mod tidy` for dependency updates in the affected module.

## Validation

From `osac-metering/`:

```bash
make test                  # schema, metering-service, and adapters
make lint
make helm-lint
```

For isolated changes, run `make test` and `make lint` in the changed module
and any directly affected module. Build adapter binaries with
`make build-echo-adapter` or `make build-m360-adapter` from `adapters/`.
Kafka-backed integration and E2E
validation requires the installer deployment and its Kafka prerequisites.
