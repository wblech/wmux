# wmux — goframe Conventions

This project uses [goframe](https://github.com/wlech/goframe) to enforce DDD + Package-Oriented Design.

## File Catalog

Files in `internal/<domain>/` must be one of:
- `entity.go` — REQUIRED: types, structs, sentinel errors (`var Err* = errors.New(...)`)
- `service.go` — REQUIRED: `type Repository interface`, `type Service struct`, business logic
- `*repository.go` — Repository implementation (e.g., `postgresrepository.go`)
- `module.go` — REQUIRED: `var Module = fx.Options(fx.Provide(...))` only
- `options.go` / `*_options.go` — Functional options: `type Option func(*T)`, `With*` constructors
- `events.go` — Domain event types
- `values.go` — Extra value objects
- `*_mock.go` — Generated mocks (go.uber.org/mock)
- `*_test.go` — Tests (same package, not `_test` suffix package)

## Import Rules

- `internal/<domain>/` can import: stdlib, `internal/platform/*`
- `internal/<domain>/` CANNOT import: other domains, external libs (except `go.uber.org/fx` in `module.go`)
- `internal/platform/*` can import: stdlib, external libs
- `internal/platform/*` CANNOT import: `internal/<domain>/`
- `cmd/*` can import: anything

## Naming

- Forbidden package names: utils, common, helpers, shared, base, models, lib, types, misc, core
- No `Get` prefix on getters: use `Name()` not `GetName()`
- Repository files: `<adapter>repository.go` (e.g., `postgresrepository.go`)

## Policy by Layer

- No `panic()` in `internal/` — only in `cmd/`
- No `os.Exit()` in `internal/` — only in `cmd/*/main.go`
- No `log.Fatal()` in `internal/` — only in `cmd/`
- No `log.*` in `internal/platform/`

## Options Pattern

- Type must be closure-based: `type Option func(*Service)` not struct/interface
- Constructor functions must start with `With`: `WithTimeout(d time.Duration) Option`
- Options must live in `options.go` or `*_options.go`
- `New*` constructors should use variadic: `func NewService(opts ...Option)`
- **Required parameters are positional arguments, not public options.** When a
  value is always needed, the constructor/method receives it as a fixed argument
  and sets it directly on the internal config. Only genuinely optional
  configuration is exposed as public `With*` functions.
- **Mutually exclusive modes get dedicated constructors.** Each takes
  mode-specific values as required parameters, builds the config struct
  directly, applies optional `With*` overrides, and delegates to a shared
  private implementation.
  Example (stdlib): `context.WithTimeout` / `context.WithCancel` / `context.WithValue`

## Testing

- Every `.go` file needs a corresponding `_test.go`
- Use `go.uber.org/mock` for mocks, name files `*_mock.go`

## Running Checks

```bash
make lint      # golangci-lint + goframe check
make test      # go test -race -shuffle=on ./...
make generate  # go generate ./...
```
