# Minimum read-only product interface and explicit startup

Status: frozen implementation contract, not yet deployed. D-042 / D-043.
Read dependency PR #80 merged at `574124aa997464eb3305fcc6ce2d5ddcb9ff28f7`,
reviewed HEAD `f8c0607c73cbe640aeec06be0d39c11cd9296bd1`, CI `38507300691`
all three groups successful. This slice brings presentation forward at the
human's request to see the UI; it does not skip admission/usage/access/share.

## Modules and process boundary

- producthttp owns embedded HTML/CSS/JS and the same-origin HTTP boundary.
  Only accepted auth/read application ports and allowlisted DTOs. No CDN,
  external telemetry, SDK, SQL or browser-stored token. Names use textContent,
  never resource-derived innerHTML. CSP denies inline code/framing and external
  connections; no-store, nosniff, no-referrer and frame denial on every response.
- platform/productserver is the sole new composition root: protected external
  config -> real PostgreSQL auth/task/catalog/storage stores -> accepted read-only
  IndexCore Client/PhysicalService -> product policy -> producthttp.
  No acquisition runtime, provider session resolver, Hint, downloader or worker.
- cmd/panta adds explicit `serve-product-readonly` and `provision-product-user`
  actions. No arguments containing credentials. Default mode remains unchanged.
  Both reject enabled acquisition worker settings. No automatic migration.

## Frozen configuration and admission

PANTA_PRODUCT_CONFIG_FILE identifies an absolute owner-protected Linux file in
an owner-only directory, loaded through the existing protected-file guard.
JSON: database_url_file (protected absolute secret), listen_address (literal
loopback IP and nonzero port), origin (exact matching HTTP origin),
indexcore_base_url (absolute credential-free HTTP(S), no query/fragment),
bindings [{binding_id,label}] (1-64 unique UUIDs, bounded safe display labels).
No cookie/Hint/provider/native-directory value belongs here. Read the database
URL only from its separate protected file. Errors/logs never include paths,
secret values, URL or backend responses. Validate before opening the listener.

Startup checks compatible exact schema head and every configured Binding.
Missing/invalid configuration, schema or Binding aborts; no in-memory fake,
implicit schema upgrade or broad namespace fallback. An external read outage
is shown as unavailable by the accepted API, not replaced with fabricated data.

PANTA_PRODUCT_USER_FILE for explicit provision contains login/password/USER or
ADMIN. Validate protected strict JSON, password bounds and role; use the
accepted auth.Service.Provision. Duplicate account does not reset a password.
No public signup, default account or plaintext password output. Provision and
serve close their owned pools. Provision never exposes a listener.

HTTP is loopback-only in this slice; remote viewing uses a loopback-bound SSH
forward, not 0.0.0.0, a firewall change or insecure LAN cookie. ReadHeaderTimeout
5s, ReadTimeout 15s, WriteTimeout 20s, IdleTimeout 60s; headers <=16 KiB;
application request context 15s. Bounded global login admission: 8 attempts per
minute and <=2 simultaneous bcrypt requests, 429/AUTH_BUSY and Retry-After on
refusal. No unbounded per-user/IP memory map. Future public deployment needs its
own transport/security acceptance. Shutdown joins HTTP handlers before pool
close; five-second graceful deadline then close/cancel remaining requests.

## Interface and routes

Embedded pages have a login view and a responsive signed-in workspace with
Resources / My Tasks tabs. Product identity and readonly status are visible.
GET `/api/v1/workspace` is authenticated and returns only mode=READ_ONLY,
schema_version and configured Binding UUID/label pairs. It contains no root,
provider scope, credential or query endpoint. API remains existing login/me/
logout + bounded task list/detail + resources Browse/Resolve.

Resources: choose Binding, bounded Q4 pagination, directory navigation and
exact-path Q5 lookup. Show current page items, PRESENT state and projected Copy
or physical-only state. Do not invent global totals or imply provider completion.
Tasks: owner-only durable state/paired Job, bounded pagination/detail. Empty,
expired login, ambiguity, unavailable service and recovery-required states are
explicit. No fabricated records/progress, optimistic success or auto-mutation.
Logout revokes the session and clears displayed private data. Request failures
are static user-facing categories, not raw server text.

Downloads, file access, sharing and quota are visibly unavailable in this slice.
No clickable controls may pretend they are implemented. UI preview is not full
MVP acceptance. Controlled preview fixtures must be visibly labelled as test
data, use an isolated database and external test HTTP service, never a fake
production fallback or evidence of the real 115 chain.

## Verification and rollback

HTTP tests for static assets/CSP/404, identity/workspace redaction, login bounds
and shutdown/cancellation. Protected config/user-file tests including unsafe
permissions, duplicates/unknown fields, invalid role/origin/listener/Binding and
secret-free errors. Real isolated PG provisioning/login/reads and actual CLI
startup/SIGTERM, port conflicts, v12/missing schema admission. Browser verify
login/failure/logout, resource/task states and responsive layout with screenshots.
Full existing tests/race/vet/build/import guards and exact-head CI before merge.

Deploy only a new owned readonly process, protected config and independent MVP
preview database. Do not migrate/restart the old Gate 3.18 DB/IndexCore/services,
reassign old tasks or touch unknown provider reservations. Rollback stops the
new PID and local SSH forward; retain the new private data and old binary.
Stage 2B admission/ledger and access/share remain subsequent bounded work.
