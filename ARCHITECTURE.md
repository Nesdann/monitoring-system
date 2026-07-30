# Architecture & Technical Notes

This document goes deeper than the README: how each service actually works internally, the reasoning behind key design decisions, the wire protocol, the schema, and the exact detection math. It's written for someone reading the code, or for future-me revisiting this project later.

---

## 1. Wire protocol — Agent ↔ Backend

Agents and the backend talk over a raw TCP socket using a custom length-prefixed JSON protocol (`protocol` package, shared by both).

Every message is:

```
[4 bytes: length, big-endian uint32][N bytes: JSON payload]
```

`WriteEvent` marshals an `Event` struct to JSON, writes its byte length as a 4-byte big-endian header, then writes the payload. `ReadEvent` does the reverse: read exactly 4 bytes, interpret as the payload length, then read exactly that many bytes and unmarshal.

This is a deliberately minimal alternative to framing messages with delimiters (like newlines) or using a heavier protocol (gRPC, HTTP). It avoids ambiguity about where one JSON object ends and the next begins, at the cost of having to hand-roll framing instead of getting it from a library.

```go
type Event struct {
    Type      string         `json:"type"`
    Timestamp int64          `json:"timestamp"`
    Hostname  string         `json:"hostname"`
    Data      map[string]any `json:"data"`
}
```

`Data` is an untyped map rather than a fixed struct per event type, because a single `Event` shape carries four very different payloads (`metrics`, `process`, `connection_snapshot`, `heartbeat`). The tradeoff: the backend has to do runtime type assertions (`event.Data["cpu"].(float64)`, etc.) instead of getting compile-time safety on event contents. Given the small number of event types, this was judged an acceptable tradeoff over four separate message types with a discriminated union (which Go doesn't support natively anyway).

---

## 2. Agent

The agent runs four goroutines concurrently, each producing `Event`s onto a shared buffered channel, with a single `sender` goroutine draining that channel and writing to the TCP connection:

- **`collectMetrics`** — polls `cpu.Percent` and `mem.VirtualMemory` every 5s.
- **`collectProcess`** — polls `process.Processes()` every 15s. For each process, collects `name`, `pid`, `user`, `cpu`, `mem`, and (added later) `ppid`, `exe`, `create_time`, `num_fds`.
- **`collectConnections`** — polls `net.Connections("inet")` every 20s, and additionally resolves each connection's `pid` to a process name via `process.NewProcess(pid).Name()`, so connections carry a human-readable `process_name` instead of just a raw `pid`.
- **`heartbeat`** — emits a bare heartbeat event every 10s, mostly useful for confirming liveness in logs / for a future "is this agent still alive" check (Phase 18 on the roadmap).

**Why several of the enrichment fields need root:** `exe` (binary path) and `num_fds` (open file descriptor count) are read from `/proc/<pid>/exe` and `/proc/<pid>/fd/` respectively. Linux only allows a process to read those paths for itself or for processes it owns; reading them for other users' processes (including root-owned system daemons, if the agent isn't root) requires elevated privileges. This is why the agent needs to run with `sudo` for those fields to populate — without it, they silently fall back to `"unknown"` / `-1` rather than crashing, so the agent keeps running even under reduced privilege, just with less data.

**In-process rule engine:** separately from the statistical work the Rust analyzer does later, the agent-side of the pipeline (actually evaluated backend-side, per received event, in `evaluateRules`) runs cheap, stateful novelty checks in real time:
- `ruleCPUSostenida` — flags 3 consecutive CPU readings all above 80%.
- `ruleProcesoRootDesconocido` — flags a root-owned process whose name isn't in a known-good allowlist and isn't a kernel thread.
- `ruleProcesoNuevo` — flags a process name never seen before (after a warm-up period, to avoid alerting on every process during the first snapshot).
- `ruleConexionPuertoRaro` — flags an established (or, after a later fix, established-or-UDP) connection to a port outside a small allowlist.
- `ruleConexionNueva` — flags a `(dst_ip, dst_port)` pair never seen before.

This state (`KnownProcesses`, `KnownConnections`, `RecentCPUs`) lives in memory, keyed per hostname, in a `StateStore` guarded by a mutex. It resets on backend restart — there's no persistence for "have I seen this before," which is a known limitation: a backend restart means every process/connection looks "new" again for one cycle.

---

## 3. Backend

The backend has three responsibilities, all sharing one Postgres connection pool:

### 3.1 Ingestion
Accepts TCP connections from agents, reads `Event`s via the shared protocol, and for each event: runs the in-memory rule engine (above), then persists interesting data to Postgres. Not everything is stored — `isInterestingProcess`/`isInterestingConnection` filter out routine noise (near-idle kernel threads, ordinary port-443/80 traffic) before it ever reaches the database, to keep table growth manageable.

### 3.2 Migrations
`runMigrations` runs a fixed list of `CREATE TABLE IF NOT EXISTS` / `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` statements every time the backend starts. Both forms are idempotent — safe to run on every boot, whether the schema already matches or not — which means schema setup never requires a human to run SQL by hand, on a fresh machine or an existing one.

### 3.3 REST API
A second goroutine (`startAPIServer`) runs `net/http` on a separate port from the agent-facing TCP listener, exposing read endpoints over the same Postgres connection: `/hosts`, `/alerts`, `/risk`, `/metrics`, `/processes`, `/connections`. All support filtering (hostname, category, severity, time window) and pagination (`limit`/`offset`, capped server-side to prevent unbounded queries).

**Auth model:** API keys are generated via a backend CLI flag (`-generate-key`), never exposed over HTTP. A raw key is a random 32-byte value, shown once at generation time; only its SHA-256 hash is ever stored, alongside an `owner` and a `role` (`viewer` or `admin`). Every request's `X-API-Key` header is hashed and looked up — if the stored role doesn't meet the endpoint's minimum requirement, the request is rejected with `403`; an unrecognized key gets `401`.

The first key ever created is automatically granted `admin`, regardless of the requested role — this is the bootstrap exception, since there's no existing admin to authorize it. Every subsequent key requires a valid, active admin key to be generated. This closes the gap where anyone with shell access could otherwise mint themselves unlimited keys — though it's worth being explicit that this only protects the *HTTP API*; anyone with direct database credentials could still insert rows into `api_keys` manually. The API key system's trust boundary is the network interface, not the machine itself.

---

## 4. Database schema (summary)

| Table | Purpose | Notable columns |
|---|---|---|
| `metrics` | Host-level CPU/RAM samples | `hostname`, `cpu`, `ram`, `timestamp` |
| `processes` | Per-process snapshots | `pid`, `name`, `ppid`, `exe`, `create_time`, `num_fds`, `cpu`, `mem` |
| `connections` | Per-connection snapshots | `pid`, `process_name`, `src/dst ip:port`, `protocol`, `status` |
| `alerts` | Detector output | `category`, `severity`, `message`, `occurrence_count`, `last_seen` |
| `host_risk` | One row per host | `score`, `updated_at` |
| `api_keys` | Auth | `key_hash`, `owner`, `role`, `active` |

`alerts.occurrence_count` / `last_seen` exist specifically to support deduplication (§5.3) — without them, every detector firing would be a brand-new row even if the underlying condition hadn't changed since the last check.

---

## 5. Analyzer (Rust)

Runs on a fixed 30-second loop: fetch all known hostnames, then run a sequence of analysis passes per host.

### 5.1 Statistical detectors

All four share the same shape — implement a `Detector` trait with one method, `analyze(hostname, samples) -> Option<Alert>` — which is what lets the same detector struct be reused across CPU, per-process CPU/mem, and connection-count series without duplicating logic per data source.

- **Z-score**: `(current - mean) / std_dev`. Sensitive to any deviation from a normal distribution, but skewed hard by a single outlier in the sample window (since that outlier pulls both the mean and the std dev).
- **Moving average deviation**: `|current - avg| / avg`, expressed as a percentage. Simple, but treats a single spike and a genuine sustained shift the same way.
- **EWMA** (exponentially weighted moving average): recent samples are weighted more heavily than older ones (`alpha` controls the decay rate). This is the trend-sensitive detector — it reacts to gradual drift faster than a flat moving average would, since old "normal" values get discounted over time rather than counted equally forever.
- **MAD** (median absolute deviation): `0.6745 * (current - median) / MAD`. Uses median instead of mean, and median absolute deviation instead of standard deviation — both of which are far less sensitive to a single extreme value than the mean/std-dev pair. This is the "robust to noisy/outlier-heavy data" detector called for in the roadmap.

**Category assignment:** the `Alert` struct returned by a detector doesn't know what kind of data it just analyzed — the same `MadDetector` is called with CPU samples, process-memory samples, and connection-count samples in different places. Rather than parameterizing every detector with a category argument, each detector returns an `Alert` with an empty `category`, and the **caller** (which does know the context) sets `alert.category` afterward, right before saving. This keeps the detector implementations fully generic and reusable, at the cost of requiring every call site to remember to stamp the category — a tradeoff made deliberately in favor of not duplicating four detector implementations per data source.

### 5.2 Behavioral detectors (`analyze_process_relationships`)

Unlike the statistical detectors, these don't work on numeric time series — they compare the latest snapshot of a process against *that process's own history* in Postgres:

- **Path mismatch**: has this process `name` ever run from a different `exe` path than the one it's running from right now? Flags binary substitution / masquerading.
- **New parent**: has this process `name` ever had a different `ppid` than its current one? Only fires if there's prior history for the name at all (a genuinely brand-new process name is a different, separate alert — "new process" — not a parent anomaly).
- **Young + high CPU**: `create_time` age under 60 seconds and current `cpu` over 50%. A blunt but effective heuristic — a process that's barely started and is already CPU-heavy is a stronger signal combined than either fact is alone.

A known limitation, worth stating plainly: on first run, every `(name, exe)` and `(name, ppid)` pairing is unconditionally accepted as the baseline, since there's nothing to compare it to yet. This means containerized or multi-instance processes (e.g. `apache2` running under different container-shim parents) won't false-positive on day one, but it also means the detector has no concept of "this baseline itself might be wrong" — it only ever learns forward from what it's already seen.

### 5.3 Deduplication (`save_alert`)

Every alert write goes through the same function, which checks — before inserting — whether a matching `(hostname, detector, category)` alert already exists with a `last_seen` inside a 5-minute window. If so, it `UPDATE`s that row's `occurrence_count` and `message` instead of inserting a new one. This turns "the same condition firing every 30-second cycle for ten minutes" into one row that accurately shows it happened repeatedly, rather than twenty near-identical rows.

### 5.4 Classification (`analyze_combination`)

Looks at which `(category, detector)` pairs have fired for a host within the same 5-minute window and maps specific combinations to a human-readable verdict:

| Signals present | Classification |
|---|---|
| behavioral anomaly (path/parent/young-high-cpu) **+** connection alert | `possible_intrusion` (critical) |
| behavioral anomaly alone | `suspicious_process_change` |
| host-level CPU alert **+** a specific process CPU/mem alert | `resource_exhaustion` |
| any other 2+ distinct categories together | `uncategorized_combination` |

This itself writes a new alert (`detector = "classification"`), which is what lets `/alerts?category=possible_intrusion` surface a verdict directly, instead of requiring a human to manually notice that two unrelated-looking alerts actually describe one situation.

### 5.5 Risk scoring (`compute_risk_score`)

For each host, sums over all alerts with `last_seen` in the last 15 minutes:

```
score += severity_weight(severity) * sqrt(occurrence_count)
```

with `critical = 10`, `high = 5`, `warning = 2`. The square root on occurrence count is deliberate: a low-severity detector firing 100 times shouldn't be able to outscore a single critical alert, but repetition should still matter more than a one-off. This is a heuristic, not a statistically derived formula — tuned by inspection, not fit to labeled data (there isn't any labeled incident data to fit to at this stage of the project).

---

## 6. Design decisions worth calling out

- **Why Go for agent/backend and Rust for the analyzer, instead of one language?** The agent/backend side is mostly I/O — network protocol handling, DB writes, an HTTP API — where Go's simplicity and goroutine model fit well. The analyzer is closer to a batch numeric-processing job running on a timer, decoupled from the ingestion path entirely (it polls Postgres, not the agents directly). Splitting them means the statistical/detection logic can evolve independently of the ingestion protocol, at the cost of maintaining two toolchains and duplicating some concepts (e.g. the `Alert` shape exists conceptually in both, though only Rust's version is authoritative since only the analyzer writes to `alerts`).
- **Why filter what gets stored (`isInterestingProcess`/`isInterestingConnection`) instead of storing everything and filtering at query time?** Storing every process/connection snapshot from every host, every 15–20 seconds, grows unbounded very quickly with mostly-redundant data (the same idle kernel threads, over and over). Filtering at write time trades some analytical flexibility (you can't retroactively analyze a process that was filtered out) for keeping the database's growth proportional to what's actually interesting.
- **Known gap, not yet addressed:** in-memory rule-engine state (`StateStore`) resets on backend restart, and the statistical detectors' "baseline" is implicitly whatever's in the last 30 rows of a table — neither has a formal concept of a *trusted, reviewed baseline* the way a mature system would. Right now, "normal" is just "whatever's been observed recently," which means a sustained attack that ramps up slowly could, in principle, get absorbed into the baseline instead of triggering a trend detector. This is a real limitation of the current design, not a bug — addressing it would mean deliberately separating "baseline" from "recent history," which isn't built yet.

---

## 7. Known limitations

- No automated tests exist yet (Phase 18 on the roadmap).
- Configuration (DB connection string, ports) is hardcoded as constants rather than externalized to environment variables or a config file.
- The API key model supports roles but not full multi-tenancy — there's no concept of an "organization" that scopes which hosts a given key can see; every valid key can see every host.
- No alerting/notification delivery (email, Slack, webhook) exists — alerts are queryable via the API and visible on the dashboard, but nothing pushes them anywhere.
