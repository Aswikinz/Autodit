# Continuous Auditing Platform — Agent Brief

This bundle is the instruction set for an AI agent (or a new engineer) building the
continuous auditing platform described in the solution blueprint.

## Start here

**`AGENTS.md`** — read it completely before writing any code. It contains the
inviolable rules, the working process, and the definition of done. It is also
valid as `CLAUDE.md`; symlink rather than duplicate.

## Contents

| File | Covers |
|---|---|
| `AGENTS.md` | Entry point: inviolable rules, workflow, definition of done |
| `docs/00-BUILD-ORDER.md` | Five milestones, each a vertical slice, with exit tests |
| `docs/01-DOMAIN-MODEL.md` | Canonical model, exception identity, evidence chain, tie-out, vocabulary |
| `docs/02-REPOSITORY.md` | Monorepo layout, branching, commits, PRs, CODEOWNERS, releases |
| `docs/03-CODING-STANDARDS.md` | Go, TypeScript, SQL/dbt, JDM authoring, naming |
| `docs/04-TESTING.md` | Test pyramid, golden datasets, coverage thresholds and ratchet policy |
| `docs/05-CICD.md` | GitHub Actions workflows, release pipeline, branch protection, Renovate |
| `docs/06-DOCUMENTATION.md` | MkDocs, ADRs, runbooks, the test catalogue |
| `docs/07-SECURITY.md` | Logging prohibition, secrets, RBAC, supply chain, dependency policy |

## How to use it

Drop these files into the repository root of a new project. Point the agent at
`AGENTS.md`. Give it one milestone from `00-BUILD-ORDER.md` at a time — the
milestones are sequenced so that later work depends on earlier correctness
properties being in place, and building ahead creates rework.

## Companion artefacts

- `Continuous-Auditing-Platform-Blueprint.docx` — the architecture and critical evaluation
- `continuous-audit-architecture.drawio` — solution, component and deployment diagrams
