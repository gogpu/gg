# Contributing to gogpu/gg

Thank you for your interest in contributing to **gg** — the enterprise-grade 2D graphics library for Go!

## Requirements

- **Go 1.25+** (required for iter.Seq, generics, and other modern features)
- **golangci-lint** for code quality checks
- **git** with conventional commits knowledge

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/gg`
3. Create a branch: `git checkout -b feat/your-feature`
4. Make your changes
5. Run pre-release check: `bash scripts/pre-release-check.sh`
6. Commit: `git commit -m "feat(component): add your feature"`
7. Push: `git push origin feat/your-feature`
8. Open a Pull Request

## Development Setup

```bash
# Clone the repository
git clone https://github.com/gogpu/gg
cd gg

# Install dependencies
go mod download

# Run tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run linter (5-minute timeout for large checks)
golangci-lint run --timeout=5m

# Format code
go fmt ./...

# Run pre-release validation (recommended before PR)
bash scripts/pre-release-check.sh

# Run examples
go run ./examples/basic/
go run ./examples/shapes/
go run ./examples/scene/
```

## Architecture Principles

gg follows **Rust-inspired 2D graphics patterns**:

| Principle | Description |
|-----------|-------------|
| **Pure Go** | No CGO, easy cross-compilation, single binary |
| **GPU-First** | Designed for GPU acceleration from day one |
| **Zero-Allocation** | Hot paths use pooling and pre-allocation |
| **Type Safety** | Sealed interfaces, explicit types, no interface{} |
| **Production-Ready** | Enterprise-grade error handling, 70%+ coverage |

Reference libraries: [vello](https://github.com/linebender/vello), [tiny-skia](https://github.com/RazrFalcon/tiny-skia), [kurbo](https://github.com/linebender/kurbo)

## Code Style

- **Formatting:** `gofmt` (run `go fmt ./...` before committing)
- **Linting:** `golangci-lint` with project configuration
- **Coverage:** Minimum 70% for new code
- **Documentation:** All public APIs must be documented
- **Error Handling:** Use sentinel errors with `errors.Is()`/`errors.As()`

### Naming Conventions

```go
// Types: PascalCase
type ShapedGlyph struct {}

// Exported: PascalCase
func NewContext(width, height int) *Context {}

// Unexported: camelCase
func processGlyphs(glyphs []Glyph) {}

// Constants: PascalCase for exported, camelCase for internal
const MaxCacheSize = 1024
const defaultTimeout = 5 * time.Second

// Acronyms: uppercase (ID, URL, HTTP, GPU, CPU)
type GlyphID uint32
```

## Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description

[optional body]

[optional footer]
```

### Types

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `test` | Adding or fixing tests |
| `refactor` | Code change without feature/fix |
| `perf` | Performance improvement |
| `chore` | Maintenance, dependencies |
| `ci` | CI/CD changes |

### Scopes

`context`, `path`, `scene`, `text`, `cache`, `backend`, `wgpu`, `raster`, `blend`, `filter`, `examples`, `docs`, `ci`

### Examples

```bash
feat(text): add Unicode text wrapping with UAX #14 support
fix(scene): resolve race condition in layer cache
perf(cache): reduce lock contention with sharded cache
docs: update README with v0.13.0 features
test(path): add iterator edge case tests
```

## Pull Request Guidelines

### Before Opening a PR

1. **Run pre-release check:** `bash scripts/pre-release-check.sh`
2. **Ensure all tests pass:** `go test -race ./...`
3. **Check linter:** `golangci-lint run --timeout=5m`
4. **Format code:** `go fmt ./...`
5. **Update documentation** if adding/changing public APIs

### PR Requirements

- **Focused:** One feature or fix per PR
- **Tested:** Include tests for new functionality
- **Documented:** Update relevant docs (README, CHANGELOG for releases)
- **Clean history:** Squash commits if needed
- **CI passing:** All GitHub Actions checks must pass

### PR Template

```markdown
## Summary
Brief description of changes

## Changes
- Change 1
- Change 2

## Testing
How was this tested?

## Checklist
- [ ] Tests pass (`go test -race ./...`)
- [ ] Linter passes (`golangci-lint run`)
- [ ] Code formatted (`go fmt ./...`)
- [ ] Documentation updated (if applicable)
```

## Reporting Issues

When opening an issue, please include:

- **Go version:** `go version`
- **OS and architecture:** e.g., Windows 11 x64, macOS 14 ARM64
- **gg version:** e.g., v0.31.0
- **Minimal reproduction:** Code snippet or repository link
- **Expected vs actual behavior**
- **Error messages and stack traces**
- **Output images** (if visual issue)

## Priority Areas

We especially welcome contributions in:

1. **API Feedback** — Try the library and report pain points
2. **Test Coverage** — Expand test cases, especially edge cases
3. **Examples** — Real-world usage demonstrations
4. **Documentation** — Improve clarity and completeness
5. **Performance** — Benchmark and optimize hot paths
6. **Cross-Platform** — Testing on different OS/architectures

## Questions?

- **GitHub Discussions:** For questions and ideas
- **GitHub Issues:** For bugs and feature requests

---

Thank you for contributing to gogpu/gg!
