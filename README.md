# Endpoint Monitor

A lightweight endpoint monitoring and threat-detection platform. Agents collect system, process, and network data from hosts; a Go backend stores and serves it; a Rust analyzer runs statistical and behavioral detectors on top of it; a single-page dashboard visualizes host risk and alerts in real time.

Built as a learning/portfolio project to explore the full pipeline behind a monitoring product: data collection → detection → risk scoring → API → UI, rather than just one layer of it.

---

## Architecture

```
 ┌──────────┐   TCP (length-prefixed JSON)   ┌──────────┐
 │  Agent   │ ─────────────────────────────▶ │ Backend  │
 │  (Go)    │                                 │  (Go)    │
 └──────────┘                                 └────┬─────┘
   collects:                                        │  stores in
   - cpu / ram                                       ▼
   - processes (+ppid, exe,                    ┌──────────┐
     create_time, num_fds)                      │ Postgres │
   - connections (+resolved                     └────┬─────┘
     process name)                                    │  read by
   - inline novelty rules                              ▼
     (new process / connection)              ┌──────────────┐
                                              │   Analyzer   │
                                              │    (Rust)    │
                                              └──────┬───────┘
                                                      │ writes alerts,
                                                      │ risk scores back
                                                      ▼
                                              ┌──────────────┐
 ┌──────────┐   REST + API key auth          │  Postgres    │
 │Dashboard │ ◀────────────────────────────  │ (alerts,     │
 │  (HTML)  │        Backend API              │ host_risk)   │
 └──────────┘                                └──────────────┘
```

Three independent processes, one shared Postgres database:

- **Agent** (Go) — runs on a monitored host. Polls system metrics, running processes, and network connections, and streams them to the backend over a small length-prefixed TCP protocol. Also runs lightweight in-line rules (new process seen, new connection seen) for immediate console feedback.
- **Backend** (Go) — accepts agent connections, persists everything to Postgres, runs schema migrations automatically on startup, and exposes a REST API (with API-key authentication and role-based access) for the dashboard and any other consumer.
- **Analyzer** (Rust) — polls Postgres on a fixed interval, runs statistical and behavioral detectors per host, classifies combinations of alerts into human-readable categories, and computes a per-host risk score.
- **Dashboard** — a single static HTML file (no build step, no framework) that polls the backend's API and renders fleet status, alerts, and per-host detail.

---

## Detection logic

Detectors fall into two families:

**Statistical detectors** (Rust, run per host and per process, on CPU/RAM/connection-count/process CPU/process memory time series):
- **Z-score** — flags values far from the recent mean.
- **Moving average deviation** — flags values that diverge from a rolling average by more than a set percentage.
- **EWMA** (exponentially weighted moving average) — trend-sensitive; reacts faster to sustained drift than a flat average.
- **MAD** (median absolute deviation) — robust to outliers and noisy data, where a plain z-score would be skewed by a single spike.

**Behavioral / relationship detectors** (Rust, compare current state to each process's own history):
- **Path mismatch** — a process name has previously run from a different binary path than the one it's running from now.
- **New parent** — a process name has a parent PID it's never had before.
- **Young + high CPU** — a process is both very recently started and already consuming significant CPU.

**Alert pipeline**, once a detector fires:
1. Every alert carries a **category** (`cpu`, `mem`, `process`, `connections`, etc.) and a **severity**, computed from how far the observed value is past the detector's own threshold (not hardcoded per detector).
2. **Deduplication** — a repeated firing of the same `(hostname, detector, category)` within a rolling window updates an existing alert's occurrence count instead of creating a new row, so persistent issues don't flood the table.
3. **Classification** — alerts from the last few minutes are checked in combination: certain co-occurring categories (e.g. a process anomaly *and* an unusual connection) get labeled with a higher-level verdict such as `possible_intrusion` or `resource_exhaustion`, rather than left as disconnected raw alerts.
4. **Risk scoring** — each host gets a single score, computed from recent alerts weighted by severity, with diminishing returns for repeated occurrences (so one critical alert outweighs a hundred low-severity ones, but repetition still counts for something).

---

## API

All endpoints require an `X-API-Key` header. Keys are generated via the backend CLI, stored only as salted hashes (never in plaintext), and carry a role (`viewer` or `admin`) that gates access per endpoint.

| Endpoint | Role | Description |
|---|---|---|
| `GET /hosts` | viewer | Distinct hostnames reporting in |
| `GET /alerts` | viewer | Alerts, filterable by `hostname`, `category`, `severity`, `since`; paginated via `limit`/`offset` |
| `GET /risk` | viewer | Current risk score per host, or for a single `hostname` |
| `GET /metrics` | viewer | Recent CPU/RAM samples for a host |
| `GET /processes` | admin | Recent process snapshots for a host, including path, parent PID, open file descriptors |
| `GET /connections` | admin | Recent network connections for a host, joined with the resolved process name |

Process and connection data is gated behind the `admin` role since it's more sensitive than aggregated alerts and metrics.

Generating a key:

```bash
# first key ever created is automatically granted admin (bootstrap)
go run . -generate-key=your-name -role=viewer

# every key after that requires an existing admin key
go run . -generate-key=teammate -role=viewer -admin-key=<existing-admin-key>
```

Keys are shown once at creation time and are not recoverable — only regenerable.

---

## Dashboard

A single `dashboard.html` file, served directly by the Go backend, with no separate frontend build or server. It polls the API every 15 seconds and renders:

- **Fleet overview** — one card per host, with a radial gauge encoding that host's current risk score and a color-coded status label (nominal / elevated / high / critical).
- **Alerts table** — recent alerts across the fleet, filterable by severity, and by host once a host card is selected.
- **Host detail panel** — opens on clicking a host card: latest CPU/RAM reading, top processes by CPU usage, and currently established connections for that host.

The dashboard authenticates using an API key embedded directly in the page. This is a deliberate, documented trade-off for a single-operator local tool — it is **not** suitable as-is for a shared or internet-facing deployment, where authentication would need to move server-side (e.g. a login/session flow) instead of a static key in client-side JavaScript.

---

## Tech stack

- **Agent / Backend** — Go, [gopsutil](https://github.com/shirou/gopsutil) for system data collection, `lib/pq` for Postgres, standard-library `net/http` for the API (no framework)
- **Analyzer** — Rust, `tokio` + `tokio-postgres` for async Postgres access, `anyhow` for error handling
- **Database** — PostgreSQL
- **Dashboard** — vanilla HTML/CSS/JavaScript, no build tooling

---

## Getting started

Requires Go, Rust, and a running PostgreSQL instance.

```bash
# 1. Start the backend — runs migrations automatically, no manual SQL needed
cd backend
go run . -generate-key=admin -role=viewer   # first key auto-becomes admin
go run .

# 2. Start the analyzer
cd ../analyzer
cargo run

# 3. Start the agent (needs elevated privileges to read other processes'
#    binary paths and file descriptor counts)
cd ../agent
sudo go run .

# 4. Open the dashboard
# add the API key from step 1 into dashboard.html, then:
# http://localhost:8081/dashboard
```

Configuration currently lives in constants near the top of `main.go` in the backend and agent (database connection string, ports). Externalizing this into environment variables/config files is on the roadmap below.

---

## Roadmap

This project follows a phased roadmap from "working pipeline" toward a product an organization could adopt. Completed so far:

- ✅ Multi-source data collection (metrics, processes, connections)
- ✅ Statistical + behavioral detection
- ✅ Alert classification and per-host risk scoring
- ✅ Authenticated REST API with role-based access
- ✅ Fleet dashboard

Still open:

- ⬜ Notifications (email/Slack/webhook) and suggested remediation actions
- ⬜ True multi-tenancy and per-user/per-team access scoping
- ⬜ Automated tests, CI, externalized configuration, self-monitoring
- ⬜ Multivariate anomaly detection, cross-host correlation, forecasting

---

## License

MIT — see `LICENSE` for details.
