# Monitoring System Roadmap

## Vision

Build a distributed observability and anomaly detection platform capable of collecting telemetry from multiple machines, processing events in real time, generating alerts, detecting anomalies, and providing operational visibility.

The long-term goal is not to build a dashboard, but a complete observability system composed of agents, event pipelines, storage, alerting, and analytics.

---

# Current Status

## Phase 1 — Transport & Protocol

Completed.

Implemented:

* TCP communication
* Custom framing protocol
* JSON serialization
* Message encoding/decoding
* Agent → Backend communication
* CPU and RAM metrics collection

Current architecture:

```text
Agent
  ↓
Protocol
  ↓
TCP
  ↓
Backend
```

Current flow:

```text
Collect Metrics
    ↓
Create Message
    ↓
Serialize
    ↓
Send Through TCP
    ↓
Deserialize
    ↓
Process
```

---

# Phase 2 — Event Model

Goal:

Design a scalable event architecture.

Current limitation:

```go
Message {
    CPU
    RAM
}
```

Future event types:

* HeartbeatEvent
* MetricEvent
* ProcessEvent
* ConnectionEvent
* AlertEvent

Target:

```text
Agent
  ↓
Events
  ↓
Backend
```

Learning objectives:

* Data modeling
* Contracts
* System design
* Event-driven architectures

---

# Phase 3 — Process Inventory

Goal:

Collect running process information.

Examples:

* PID
* Process name
* User
* CPU usage
* Memory usage

Questions to solve:

* How often should processes be reported?
* Full snapshots vs incremental updates?
* How should processes be represented as events?

Learning objectives:

* OS internals
* Process management
* Event modeling

---

# Phase 4 — Network Connections

Goal:

Observe network activity.

Collect:

* Source IP
* Destination IP
* Ports
* Protocol
* Associated process

Examples:

```text
firefox
  ↓
142.250.x.x
  ↓
443
```

Learning objectives:

* Networking
* Security visibility
* Traffic analysis

---

# Phase 5 — Storage Layer

Current situation:

```text
Receive
 ↓
Print
 ↓
Lose data
```

Target:

```text
Receive
 ↓
Store
 ↓
Query
```

Technologies:

* PostgreSQL
* TimescaleDB

Learning objectives:

* Database design
* Time-series storage
* Query optimization

---

# Phase 6 — Rule Engine

Goal:

Generate alerts from predefined rules.

Examples:

* CPU > 90% for 5 minutes
* Unknown process detected
* Excessive connection count
* Process execution outside baseline

Architecture:

```text
Event
 ↓
Rule Engine
 ↓
Alert
```

Learning objectives:

* Event processing
* State machines
* Temporal conditions

---

# Phase 7 — Anomaly Detection

Goal:

Detect abnormal behavior automatically.

Potential techniques:

* Moving averages
* Z-score
* EWMA
* Baselines
* Entropy analysis
* Frequency analysis

Examples:

```text
Normal:
50 connections/minute

Observed:
4000 connections/minute
```

Learning objectives:

* Statistics
* Streaming analytics
* Security detection

---

# Phase 8 — Distributed Architecture

Current:

```text
1 Agent
1 Backend
```

Target:

```text
Many Agents
      ↓
Gateway
      ↓
Queue
      ↓
Processors
      ↓
Storage
```

Problems to solve:

* Backpressure
* Throughput
* Ordering
* Reliability
* Horizontal scaling

Learning objectives:

* Distributed systems
* Event streaming
* Scalability

---

# Final Architecture

```text
                +----------------+
                |   Dashboard    |
                +--------+-------+
                         |
                         v
                +----------------+
                | Alert Engine   |
                +--------+-------+
                         |
                         v
                +----------------+
                | Event Pipeline |
                +--------+-------+
                         |
                         v
                +----------------+
                | Storage Layer  |
                +--------+-------+
                         ^
                         |
                +--------+-------+
                |    Backend     |
                +--------+-------+
                         ^
                         |
                +--------+-------+
                |   Protocol     |
                +--------+-------+
                         ^
                         |
          +--------------+--------------+
          |              |              |
          v              v              v
      Agent A       Agent B       Agent C
```

---

# Core Principle

The dashboard is not the product.

The product is:

```text
Agents
 ↓
Events
 ↓
Processing
 ↓
Alerts
 ↓
Anomaly Detection
```

Everything else is visualization.
