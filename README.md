# tep-operator

Kubernetes operator that lets a cluster follow an industrial plant at a high level. The user declares a **cost function** (the objective function J) and an **operating policy** (targets, constraints, cost budget) as manifests; the operator evaluates the plant against the active policy and writes the verdict to the `Plant` status. It never writes to the plant.

The operator is **plant-agnostic**: there is nothing TEP-specific in the Go code. Plant knowledge lives only in the manifests — for TEP, in `tep-supervisor/local/k8s/tep/` (Downs & Vogel cost function, mode 1 policy).

## Context

Part of the **TEP CPS Lab** (Tennessee Eastman Process as a Cyber-Physical System, not a "digital twin" — there is no physical reference plant).

```
tep-plant ──OPC-UA──▶ tep-historian ──HTTP──▶ tep-operator ──▶ Plant.status ──▶ kubectl / tep-ihm
```

Kubernetes never sees raw signals. `tep-historian` turns signals into window statistics, the operator turns statistics into a verdict, and Kubernetes stores the verdict and notifies whoever watches it.

## Quick start

```bash
# Prerequisites: Go 1.25+, Docker, Kind, kubectl

make generate manifests   # deepcopy, CRDs, RBAC
make test                 # unit + envtest
make docker-build
```

Deploying to Kind with the TEP manifests: `tep-supervisor/local/setup.sh`.

## CRDs

Group `supervision.greenlabs.io/v1alpha1`:

```
Plant ──policyRef──▶ OperatingPolicy ──costFunctionRef──▶ CostFunction
```

- **CostFunction** — J as a sum of terms, each `coefficient × Π mean(signal)`.
- **OperatingPolicy** — cost budget, targets (`±tolerancePercent`), constraints (`[min, max]`), averaging window, and a persistence count (consecutive failing evaluations before the verdict flips).
- **Plant** — historian URL and active policy; its status holds J, per-term contributions, per-check results and the conditions `DataAvailable`, `CostWithinBudget`, `TargetsMet`, `ConstraintsSatisfied`, `PolicyCompliant`.

```
$ kubectl get plants
NAME   POLICY      COST     UNIT   PHASE       AGE
tep    tep-mode1   166.39   $/h    Compliant   5m
```

Generic samples: [config/samples/](config/samples/). Full details: [docs/03-crds.md](docs/03-crds.md).

## Documentation

| Doc                                                    | Content                                                  |
| ------------------------------------------------------ | -------------------------------------------------------- |
| [01 — Overview](docs/01-visao-geral.md)                | What this repo is, where it fits, current state          |
| [02 — Project anatomy](docs/02-anatomia-do-projeto.md) | File map: what to edit vs. what Kubebuilder generates    |
| [03 — CRDs](docs/03-crds.md)                           | CostFunction, OperatingPolicy, Plant: spec, status, phases |
| [04 — Reconciliation](docs/04-reconciliacao.md)        | One evaluation step by step, triggers, idempotency       |

## Lab repositories

| Repo                                                                                    | What it does                                   |
| --------------------------------------------------------------------------------------- | ---------------------------------------------- |
| [spec-tennessee-eastman](https://github.com/Green-Cinnamon-Labs/spec-tennessee-eastman) | Issues, specs, decisions (epic #77)            |
| [tep-plant](https://github.com/Green-Cinnamon-Labs/tep-plant)                           | TEP plant (Rust), signals over OPC-UA          |
| tep-historian                                                                           | OPC-UA collector, window statistics over HTTP  |
| **tep-operator**                                                                        | **This repo** — K8s operator (Go)              |
| [tep-supervisor](https://github.com/Green-Cinnamon-Labs/tep-supervisor)                 | Cluster infra (Kind, compose) + TEP manifests  |
| [tep-ihm](https://github.com/Green-Cinnamon-Labs/tep-ihm)                               | Dashboard; reads the verdict from `Plant.status` |

## Structure

```
api/v1alpha1/          <- CRD types (CostFunction, OperatingPolicy, Plant)
internal/evaluate/     <- Pure evaluation: J, targets, constraints, persistence
internal/historian/    <- HTTP client for tep-historian
internal/controller/   <- Plant reconciler
cmd/main.go            <- Manager entry point
config/                <- Kustomize: CRDs, RBAC, deployment, samples
docs/                  <- Project documentation
```
