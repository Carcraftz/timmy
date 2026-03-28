# Contributing to Timmy

Thanks for contributing.

## Development Setup

1. Install Go `1.26.1`.
2. Install Docker.
3. Install Tailscale and sign in.
4. Build the project:

```bash
make build-cli build-backend
```

5. Start local Postgres:

```bash
make docker-up
```

6. Export backend configuration as needed. See `.env.example`.

## Project Structure

- `cli/`: end-user CLI
- `backend/`: API service and data access
- `infra/`: local infrastructure

The CLI and backend are intentionally split into separate Go modules so they can be built and released independently.

## Before Opening a Pull Request

Run:

```bash
make test build-cli build-backend
```

Make sure your change:

- keeps the CLI scriptable
- does not introduce credential storage
- preserves the Tailscale-first security model
- keeps backend code out of the CLI release path

## Contribution Guidelines

- Prefer small, focused pull requests.
- Add or update tests when they meaningfully reduce regression risk.
- Keep public interfaces stable where practical, especially CLI output and flags.
- Document new environment variables, commands, or operational behavior in `README.md`.

## Commit And Review Expectations

- Use clear commit messages that explain why the change exists.
- Include manual verification steps in the pull request description when behavior changes.
- Call out any changes to auth, search behavior, or SSH execution explicitly.
