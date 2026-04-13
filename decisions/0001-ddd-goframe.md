---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# DDD + Package-Oriented Design with goframe

## Context and Problem Statement

wmux needs a code organization strategy that enforces clear domain boundaries, prevents import cycles, and scales as the project grows across multiple phases.

## Decision Drivers

* Prevent coupling between domain packages
* Enforce file naming conventions automatically
* Keep external dependencies out of domain logic

## Considered Options

* Flat package structure
* Traditional layered architecture (handler/service/repo)
* DDD with goframe enforced conventions

## Decision Outcome

Chosen option: "DDD with goframe", because it enforces domain isolation via import rules (domain packages cannot import each other), standardizes file naming (entity.go, service.go, module.go), and uses linting to catch violations. Platform packages in `internal/platform/` provide shared infrastructure.

### Consequences

* Good, because import cycles are structurally impossible
* Good, because new contributors know exactly where to put code
* Bad, because goframe conventions require discipline (every package needs entity.go, service.go, module.go)
