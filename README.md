# Demo API — Task Manager

A minimal task management REST API with an embedded [OpenAPI 3.1](https://spec.openapis.org/oas/v3.1.0) spec. Built as a demo for [Sterling](https://github.com/hdresearch/sterling) SDK generation.

## Quick start

```bash
go run main.go
# => Task Manager API listening on :8080

# Fetch the OpenAPI spec
curl http://localhost:8080/openapi.json

# Create a task
curl -X POST http://localhost:8080/api/tasks \
  -H "Authorization: Bearer my-key" \
  -H "Content-Type: application/json" \
  -d '{"title": "Buy milk", "priority": 1}'

# List tasks
curl http://localhost:8080/api/tasks -H "Authorization: Bearer my-key"
```

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/tasks` | List all tasks |
| `POST` | `/api/tasks` | Create a task |
| `GET` | `/api/tasks/{id}` | Get task by ID |
| `PATCH` | `/api/tasks/{id}` | Update a task |
| `DELETE` | `/api/tasks/{id}` | Delete a task |
| `GET` | `/openapi.json` | OpenAPI 3.1 spec |

All `/api/*` endpoints require `Authorization: Bearer <key>` (any non-empty value).

## Generated SDKs

SDKs are generated from `openapi.json` using Sterling and published to separate repos:

| Language | Repository |
|----------|-----------|
| TypeScript | [sterling-testing/taskmanager-ts](https://github.com/sterling-testing/taskmanager-ts) |
| Python | [sterling-testing/taskmanager-python](https://github.com/sterling-testing/taskmanager-python) |
| Rust | [sterling-testing/taskmanager-rust](https://github.com/sterling-testing/taskmanager-rust) |
| Go | [sterling-testing/taskmanager-go](https://github.com/sterling-testing/taskmanager-go) |

### Regenerate SDKs

```bash
sterling generate --spec openapi.json --config sterling.toml
```

## Schema overview

- **Task** — `id`, `title`, `description`, `status` (pending/in_progress/done), `priority`, `created_at`, `updated_at`, `due_date`
- **CreateTaskRequest** — `title` (required), `description`, `priority`, `due_date`
- **UpdateTaskRequest** — all fields optional
- **TaskListResponse** — `tasks[]`, `total`
- **ErrorResponse** — `error` (message), `success` (always false)
