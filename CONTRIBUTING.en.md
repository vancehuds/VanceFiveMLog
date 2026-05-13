# Contributing to VanceFiveMLog

Thank you for helping improve VanceFiveMLog.

## Development Setup

1. Copy `.env.example` to `.env` and adjust local values.
2. Run `docker compose up --build` for a local PostgreSQL-backed app.
3. Run `go test ./...` before opening a pull request.

## Pull Requests

- Keep changes focused and describe the user-visible behavior.
- Include verification commands you ran.
- Update docs and `.env.example` when changing configuration.
- Include screenshots for UI changes.

## Code Style

- Follow Go idioms and formatting (`go fmt`, `go vet`)
- Keep functions small and focused
- Add comments for non-obvious logic
- Write meaningful commit messages

## Testing

- Run `go test ./...` before submitting
- Add tests for new functionality
- Ensure all existing tests pass

## Documentation

- Update relevant documentation when changing features
- Add examples for new API endpoints
- Keep README and integration guides current

## License

By contributing, you agree that your contribution is licensed under `AGPL-3.0-or-later`.
