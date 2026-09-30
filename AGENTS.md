## Task size

- One task = one commit that builds and passes `go tool mage check` on its own.
- A task changes **at most 10 files**, tests and docs included. Check with
  `git diff --stat` before calling a task done.
- A feature that needs more is split into tasks **before starting**, listed in
  order with the files each one touches, e.g.:
  1. core package + its tests;
  2. CLI wiring + its tests;
  3. docs (README, docs/, CHANGELOG, PRIVACY in both languages).
- Docs go in their own task when they would push a task over the limit.
- If a task grows past the limit while in progress, stop, split what is left
  into a new task, and say so; never raise the limit silently.

## Code style

- Functions: 4-20 lines. Split if longer.
- Files: under 500 lines. Split by responsibility.
- One thing per function, one responsibility per module (SRP).
- Names: specific and unique. Avoid `data`, `handler`, `Manager`.
  Prefer names that return <5 grep hits in the codebase.
- Types: explicit. No `any`, no `Dict`, no untyped functions.
- No code duplication. Extract shared logic into a function/module.
- Early returns over nested ifs. Max 2 levels of indentation.
- Exception messages must include the offending value and expected shape.
- Avoid MAGIC NUMBERS or HARDCODED Strings always use ENUMS or any data structure that makes sense in the context

## Comments

- Write WHY, not WHAT. Skip `// increment counter` above `i++`.
- Docstrings on public functions if project has this pattern: intent + one usage example.
- Reference issue numbers / commit SHAs when a line exists because
  of a specific bug or upstream constraint.

## Tests

- Tests run with a single command: `go tool mage test`.
- Every new function gets a test. Bug fixes get a regression test.
- Mock external I/O (API, DB, filesystem) with named fake classes,
  not inline stubs.
- Tests must be F.I.R.S.T: fast, independent, repeatable,
  self-validating, timely.

## Dependencies

- Inject dependencies through constructor/parameter, not global/import.
- Wrap third-party libs behind a thin interface owned by this project.

## Structure

- Follow the framework's convention (Rails, Django, Next.js, etc.).
- Prefer small focused modules over god files.
- Predictable paths: controller/model/view, src/lib/test, etc.

## Formatting

- Use the language default formatter (`cargo fmt`, `gofmt`, `prettier`,
  `black`, `rubocop -A`). Don't discuss style beyond that.

## Logging

- Structured JSON when logging for debugging / observability.
- Plain text only for user-facing CLI output.

## Documentation

- READMEs contain only: description, install, supported platforms, examples, and a "More information" table linking to the docs.
- Implementation details (config fields, internals, benchmarks, setup) go in `docs/`, not in the README.
- Each `docs/` file has a single responsibility: `CONFIGURATION.md` for config and setup, `ARCHITECTURE.md` for code structure, `BENCHMARKS.md` for measurements, `DEVELOPMENT.md` for build and CI, `USECASES.md` for query patterns.
- Every README section that moved to `docs/` must be referenced in the "More information" table with a one-line description of what the reader will find there.
- Bilingual rule: README.md + docs with no `.pt-BR` suffix are in English; README.pt-BR.md + `docs/*.pt-BR.md` are in Portuguese. Keep both in sync.
