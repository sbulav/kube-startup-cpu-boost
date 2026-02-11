# AGENTS.md: Guidelines for Agentic Coding in kube-startup-cpu-boost

This file provides essential guidance for AI agents (e.g., opencode) working on the kube-startup-cpu-boost repository. It covers build, lint, and test commands, plus code style conventions to ensure consistency with the project's Go-based Kubernetes operator codebase. Follow these strictly to maintain quality and avoid breaking CI/CD pipelines.

The project is a Kubernetes operator using controller-runtime to boost pod CPU requests/limits during startup (e.g., for JVM apps) and revert post-readiness. It uses Go 1.23, Kubebuilder scaffolding, Ginkgo/Gomega for tests, and GitHub Actions for CI. Licensed under Apache 2.0; not Google-supported.

## Build Commands

Run from project root. All builds use Go modules; ensure `go mod tidy` if adding deps.

### General Build
- Full build (compiles binaries, generates manifests, runs fmt/vet):  
  `make build`  
  This runs: `make generate`, `make manifests`, `make fmt`, `make vet`. Outputs binaries in `bin/`.

- Generate code (DeepCopy, CRDs, RBAC):  
  `make generate` (uses controller-gen)  
  `make manifests` (generates YAML configs in `config/`).

- Clean build artifacts:  
  `make clean` (removes `bin/` and temp files).

### Docker Builds
- Single-arch image:  
  `make docker-build IMG=ghcr.io/your-org/kube-startup-cpu-boost:v0.1.0`  
  Builds and tags image using the Dockerfile.

- Multi-arch (amd64, arm64):  
  `make docker-buildx IMG=ghcr.io/your-org/kube-startup-cpu-boost:v0.1.0`  
  Requires Docker Buildx setup; pushes to registry if configured.

- Local run without Docker:  
  `make run` (starts controller manager; needs kubeconfig).

### Release Builds
- Generate release manifests:  
  `make release MANIFESTS=manifests.yaml` (outputs all-in-one YAML).  
- For production releases, use GoReleaser: Edit `.goreleaser.yml` and run `goreleaser release --clean`.

## Lint Commands

Enforce code quality via Makefile and manual tools. CI runs these automatically.

### Go Linting
- Format code:  
  `make fmt` (runs `go fmt ./...` – enforces tabs, no semicolons).  
  Always run before commits.

- Vet code:  
  `make vet` (runs `go vet ./...` – checks suspicious constructs).  

- Static analysis:  
  Install: `go install honnef.co/go/tools/cmd/staticcheck@latest`  
  Run: `staticcheck ./...` (detects bugs, unused code; version v2024.1.1 in CI).

### Other Linting
- License headers:  
  `python3 scripts/license_header_check.py .` (ensures Apache 2.0 boilerplate in .go files).

- Commit messages (Conventional Commits):  
  Install: `npm install -g @commitlint/cli @commitlint/config-conventional`  
  Test: `echo "feat: add new booster" | commitlint` (types: feat, fix, docs, style, refactor, test, chore).

- Markdown linting:  
  Install: `npm install -g markdownlint-cli`  
  Run: `markdownlint "**/*.md" --config .markdownlint.yml` (checks README, docs).

- Link checking:  
  Use `.mlc_config.json` with a tool like `markdown-link-check` or CI equivalent.

Always run `make fmt && make vet && staticcheck ./...` after edits.

## Test Commands

Uses Go's `testing` package with Ginkgo/Gomega for BDD-style tests. Integration tests use `setup-envtest` for mocked K8s API server.

### Prerequisites
- Download envtest assets: `make envtest` (fetches K8s v1.30.x binaries).  
- Install Ginkgo: `go install github.com/onsi/ginkgo/v2/ginkgo@latest`.

### Full Tests
- All unit + integration:  
  `make test` (runs `go test ./... -coverprofile cover.out`).  
  Includes Ginkgo suites in `*_test.go` and `*_suite_test.go`.

- View coverage:  
  `go tool cover -html=cover.out` (opens browser with report).

### Running Single Tests
Focus on efficiency; Ginkgo supports regex filtering.

- Single test function (e.g., in internal/boost/manager_test.go):  
  `go test ./internal/boost -v -run TestManager_SomeBehavior`  
  Replace `TestManager_SomeBehavior` with exact test name (use `go test -v` to list).

- Single Ginkgo suite:  
  `go test ./internal/boost/resource -run ^TestResourcePolicySuite$` (matches suite init).  
  Or with Ginkgo: `ginkgo -v ./internal/boost/resource`.

- Focused run (regex on describe/it blocks):  
  `ginkgo -focus "should boost CPU" ./internal/...` (matches test descriptions).  
  Examples:  
  - Unit test: `go test -v ./api/v1alpha1 -run TestValidate`  
  - Integration: `go test -v ./internal/controller -run TestReconcile`  
  - Parallel: Add `-p 4` for multi-core.

- Specific package:  
  `go test ./cmd/kube-startup-cpu-boost -v` (controller entrypoint tests).

Run tests after changes; aim for >80% coverage. Fix failures before proceeding.

## Code Style Guidelines

Adhere to Go idioms (effective-go) and Kubebuilder conventions. No custom linters beyond gofmt/staticcheck; search codebase for patterns.

### Imports
- Sorted by `go fmt`: stdlib first (e.g., `import "context"`), then third-party (e.g., `sigs.k8s.io/controller-runtime`), then local (e.g., `"github.com/your-org/kube-startup-cpu-boost/api/v1alpha1"`).  
- Group with blank lines:  
  ```
  import (
      "context"
      "fmt"

      "sigs.k8s.io/controller-runtime/pkg/client"

      "github.com/your-org/kube-startup-cpu-boost/internal/boost"
  )
  ```  
- Avoid unused imports (staticcheck enforces). Use aliases sparingly (e.g., `k8s "k8s.io/apimachinery/pkg/apis/meta/v1"`).  
- For K8s: Prefer typed clients from controller-runtime over raw client-go.

### Formatting
- Use `go fmt ./...`: 8-space tabs? No – gofmt uses tabs for indentation, spaces for alignment. No trailing whitespace, Unix line endings (LF).  
- Line length: Aim <100 chars; no hard rule, but readable.  
- Braces: On same line for functions/ifs (e.g., `if err != nil { return err }`).  
- No semicolons; gofmt handles.

### Types
- Native Go: Structs for resources (e.g., `type StartupCPUBoostSpec struct {}` with JSON tags).  
- Interfaces for abstraction (e.g., `type Booster interface { Boost(ctx context.Context, pod *corev1.Pod) error }`).  
- Use K8s types: `corev1.Pod`, `appsv1.Deployment` from `k8s.io/api`.  
- Generics (Go 1.18+): Minimal use; e.g., for metric collectors. No external typing tools (not TS/Python).  
- Validation: Embed `metav1.TypeMeta` and `ObjectMeta`; use `+kubebuilder:validation` tags.

### Naming Conventions
- Exported (public): UpperCamelCase (e.g., `BoostManager`, `Reconcile`, `StartupCPUBoost`).  
- Unexported (private): lowerCamelCase (e.g., `boostManager`, `reconcilePod`).  
- Packages: lowercase, short (e.g., `boost`, `webhook`, `controller`). No underscores.  
- Variables/fields: camelCase; constants UpperCamelCase if exported (e.g., `const DefaultBoostDuration = 5 * time.Minute`).  
- Acronyms: Treat as words (e.g., `httpClient`, not `hTTPClient`). Observed in codebase: Consistent camelCase, no snake_case.  
- Test names: Descriptive (e.g., `TestBoostManager_ReconcilesPodSuccessfully`).

### Error Handling
- Standard: Propagate with `if err != nil { return fmt.Errorf("wrap: %w", err) }` or plain `return err`.  
- Wrapping: Use `%w` for errors.Is/As (e.g., `k8serrors.IsNotFound(err)`).  
- Aggregation: `utilerrors.NewAggregate([]error{err1, err2})` from apimachinery for multi-errors.  
- Logging: Use Zap (structured): `logger.Error(err, "failed to boost", "pod", pod.Name)`. Levels: Info for progress, Error for failures.  
- Webhooks: Return `admissionv1beta1.AdmissionResponse` with `Result` for validation errors (e.g., `&metav1.Status{Message: "invalid CPU boost"}`).  
- No panics except in init(); recover in goroutines if needed. Custom errors: Define types implementing `error` (e.g., `type ValidationError struct { msg string }`).

### Comments and Documentation
- Godoc: On exported types/functions (e.g., `// Boost temporarily increases CPU for a pod.`).  
- Package docs: In `doc.go` (e.g., `// Package boost manages CPU boosting logic.`).  
- Inline: Sparse; explain why, not what. No TODOs without assignee.  
- CHANGELOG.md: Update for user-facing changes (use release-please).

### Other Conventions
- Logging: Zap with JSON output in prod (config in manager); levels via `--v=4`.  
- Metrics: Prometheus via client_golang; expose at :8080/metrics.  
- Security: No hard-coded secrets; use env vars or secrets. Validate inputs (webhooks).  
- Dependencies: Add via `go get`; run `go mod tidy`. No vendoring.  
- Files: One public type per file; tests alongside (e.g., manager_test.go).  

## Agent Workflow Notes
- Before edits: Run `make test` to baseline.  
- After edits: Always `make fmt`, `make vet`, `staticcheck ./...`, `make test`.  
- Verification: If tests fail, debug with `-v` or Ginkgo `--trace`.  
- Commits: Only when requested; use Conventional Commits. No force-pushes.  
- No proactive file creation (e.g., no new READMEs). Mimic existing patterns (e.g., controller-runtime reconcilers).  
- For K8s specifics: Enable feature gates (InPlacePodVerticalScaling) in manifests. Use KIND for local testing: `make kind-deploy`.  

This doc is ~150 lines; update as project evolves (e.g., add golangci-lint if adopted).