# Contributing to Giosk

Thanks for your interest in improving Giosk! Contributions of all kinds are welcome —
bug reports, docs, features, and reviews.

## Getting started

1. Read [`docs/development.md`](docs/development.md) for local setup.
2. Fork the repo and create a topic branch (`feat/…`, `fix/…`).
3. Make your change with tests where it makes sense.
4. Ensure checks pass:
   - Backend: `cd backend && go build ./... && go vet ./... && go test ./...`
   - Frontend: `cd frontend && npm run build && npm run lint`
   - Chart: `helm lint charts/giosk`
5. Open a pull request describing **what** and **why**.

## Guidelines

- Keep changes focused; one logical change per PR.
- Match the surrounding code's style and comment density.
- Never commit secrets, credentials, kubeconfigs, or internal network details.
  (`.gitignore` covers the common ones — double-check `git diff --cached`.)
- Update docs and `CHANGELOG.md` (Unreleased) when behavior or config changes.

## Reporting bugs / requesting features

Use the GitHub issue templates. Include your environment (Kubernetes distro/version,
GPU stack) and steps to reproduce.

## License

By contributing, you agree that your contributions are licensed under the
[Apache-2.0](LICENSE) license.
