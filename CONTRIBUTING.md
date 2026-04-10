# Contributing to uptimyctl

Thank you for your interest in contributing! This guide helps you get started.

## Getting Started

1. **Fork** the repository
2. **Clone** your fork locally
3. **Create a branch** for your feature or fix
4. **Make changes**, ensuring tests pass
5. **Submit a pull request**

## Development Setup

```bash
# Clone and enter the project
git clone https://github.com/<your-fork>/uptimyctl.git
cd uptimyctl

# Install dependencies
go mod download

# Run tests
make test

# Build
make build
```

## Code Standards

- Run `go vet ./...` and `golangci-lint run` before submitting
- Write tests for new functionality
- Follow existing code patterns and naming conventions
- Use meaningful commit messages

## Pull Request Guidelines

- Keep PRs focused on a single change
- Include tests for new features or bug fixes
- Update documentation if applicable
- Reference any related issues

## Reporting Issues

- Use the GitHub issue tracker
- Include steps to reproduce
- Include Go version, OS, and relevant configuration

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md).
All participants are expected to uphold this code.

## License

By contributing, you agree that your contributions will be licensed
under the Apache License 2.0.
