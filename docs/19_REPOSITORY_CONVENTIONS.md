# Repository Conventions

This document closes the repository conventions for PR 1. It describes the
structure and boundaries only; product behavior belongs in later PRs.

## Monorepo boundaries

- `apps/` contains user-facing applications. The first application will be
  `apps/web`, using Next.js App Router and TypeScript.
- `services/` contains independently runnable Go units. `services/api` and
  `services/worker` each own their `go.mod`, `cmd/`, and `internal/` tree.
- `contracts/` contains integration contracts only: OpenAPI documents and
  versioned event schemas. It does not contain Go or TypeScript business code.
- `migrations/` contains PostgreSQL migrations with six-digit ordering and
  paired `.up.sql`/`.down.sql` files when rollback is supported.
- `deploy/` contains local and deployment infrastructure. Local Compose files
  live under `deploy/compose`.
- `scripts/` contains repository automation that is useful outside a service.

## Go

- Prefer the standard library and small focused dependencies.
- Keep entrypoints in `cmd/<name>` and implementation packages in `internal/`.
- Use `context.Context` at I/O and long-running boundaries.
- Keep dependency direction `transport -> application -> domain`; adapters
  implement ports and do not leak into domain packages.
- Run `gofmt` and `go vet`; tests stay next to the package they exercise.
- Do not share business entities or repositories between API and worker.

## Next.js

- Use App Router and TypeScript.
- Keep backend/domain rules in Go; the web app owns presentation and client
  interaction.
- Prefer server components; use client components for interactive UI only.
- Keep API types/adapters under `lib/` and `types/` as the web app grows.

## Contracts

- HTTP contracts live in `contracts/openapi/`.
- Message contracts live in `contracts/events/`.
- Event names and schema filenames are versioned, for example
  `video-uploaded-v1.json`.
- A contract change must update its documentation and compatibility notes in
  the same change.

## Migrations

- Files are ordered as `000001_description.up.sql` and, where needed,
  `000001_description.down.sql`.
- PostgreSQL is the system of record; media bytes stay in object storage.
- A migration is additive and reviewed with its owning domain change.
- The migration runner will be selected when the first schema is implemented;
  PR 1 only establishes the directory and naming convention.

## Scope boundary for PR 1

This foundation intentionally does not include video endpoints, upload logic,
FFmpeg processing, or complete authentication. Empty application directories
are kept with `.gitkeep` until their implementation PRs land.

