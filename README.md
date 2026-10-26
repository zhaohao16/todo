# Go Todo API

A small Todo REST API built with nothing but the Go standard library:
`net/http`, `encoding/json`, `log/slog`, `sync`. No web framework, no database,
no third-party dependencies.

## Quick start

```bash
go run ./cmd/todo-api            # listens on :8080
go run ./cmd/todo-api -addr :9000
TODO_ADDR=:9000 go run ./cmd/todo-api
```

Listen address resolution: `-addr` flag > `TODO_ADDR` environment > `:8080`.

```bash
curl -s -X POST localhost:8080/api/v1/todos \
  -H 'Content-Type: application/json' \
  -d '{"title":"买牛奶","description":"低脂"}'

curl -s localhost:8080/api/v1/todos
curl -s localhost:8080/api/v1/todos/1
curl -s -X PATCH localhost:8080/api/v1/todos/1 \
  -H 'Content-Type: application/json' -d '{"completed":true}'
curl -s -i -X DELETE localhost:8080/api/v1/todos/1
```

## API

| Method | Path                  | Success                |
| ------ | --------------------- | ---------------------- |
| POST   | `/api/v1/todos`       | 201 + `Location` header |
| GET    | `/api/v1/todos`       | 200, JSON array        |
| GET    | `/api/v1/todos/{id}`  | 200, JSON object       |
| PATCH  | `/api/v1/todos/{id}`  | 200, JSON object       |
| DELETE | `/api/v1/todos/{id}`  | 204, empty body        |
| GET    | `/healthz`            | 200 `{"status":"ok"}`  |

A todo is:

```json
{
  "id": 1,
  "title": "买牛奶",
  "description": "低脂",
  "completed": false,
  "createdAt": "2026-10-26T12:00:00Z",
  "updatedAt": "2026-10-26T12:00:00Z"
}
```

`PATCH` is a partial update: only the fields present are changed. Sending `{}`
is rejected, because a request that changes nothing is a client bug.

### Errors

All application errors share one envelope:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "invalid request body",
    "details": [{"field": "title", "message": "must not be empty"}]
  }
}
```

| Code                     | Status | When                                      |
| ------------------------ | ------ | ----------------------------------------- |
| `VALIDATION_ERROR`       | 400    | bad id, bad JSON, unknown field, bad value |
| `NOT_FOUND`              | 404    | no todo with that id                       |
| `PAYLOAD_TOO_LARGE`      | 413    | body over 1 MiB                            |
| `UNSUPPORTED_MEDIA_TYPE` | 415    | `Content-Type` is not `application/json`   |
| `INTERNAL`               | 500    | anything unexpected; details are logged    |

One deliberate exception: a request with the wrong *method* is answered by
`net/http`'s `ServeMux` with a plain-text 405 and an `Allow` header, not with the
JSON envelope.

## Validation rules

- `title` — required, trimmed, 1–200 characters counted in **runes**, not bytes.
- `description` — optional, at most 2000 runes. May be empty.
- `completed` — optional boolean, default `false`.
- `id` — positive decimal integer; `0`, `-1`, `+1`, `1.5` and `abc` are 400s.
- Request bodies must be a single JSON object with no unknown fields.

## Layout

```
cmd/todo-api/        process wiring: flags, logger, server, graceful shutdown
internal/httpapi/    HTTP only: routing, decoding, error mapping, middleware
internal/todo/       domain: entity, validation, Service, Store port
internal/store/      Memory, an in-memory implementation of todo.Store
```

Dependencies point inward: `cmd` → `httpapi`/`store` → `todo`. The domain package
imports neither HTTP nor storage, so a Postgres store only has to implement
`todo.Store`.

## Tests

```bash
go test ./...
go test -race ./...
go test ./... -cover
```

There is no database and no external service, so every test runs offline.

## Known limitations (v1)

- State lives in memory: restarting loses everything.
- No authentication, no per-user data.
- No pagination or filtering on the list endpoint.
- No optimistic concurrency control between read and write.
