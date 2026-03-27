# Distributed Key-Value Store with Raft Consensus and Chaos Engineering

## 1. System Description

This project builds a distributed key-value store powered by the Raft consensus protocol (using the `hashicorp/raft` library), deployed as a 5-node cluster on AWS EC2. We build a Chaos Engineering toolkit to systematically inject failures and evaluate fault tolerance, and explore horizontal read scaling through follower reads with tunable consistency levels.

### Core Components

#### Raft KV Store Nodes (5 EC2 instances)

Each node runs a Go service with:

- `hashicorp/raft` handling consensus (leader election, log replication)
- A custom `raft.FSM` state machine implementing the key-value store (GET, PUT, DELETE)
- An embedded HTTP API for client operations, cluster health, and read routing

The HTTP handler on each node manages routing directly:

- **Writes** are forwarded to the current Raft leader
- **Reads** are served locally based on a consistency query parameter (`?level=strong|default|stale`), supporting three modes:
  - **strong** — forwarded to the leader (linearizable)
  - **default** — served by followers caught up to a recent commit index (bounded staleness)
  - **stale** — served by any follower immediately (eventual consistency, lowest latency)

This eliminates the need for a separate API gateway while still enabling the key scalability dimension: measuring how follower reads scale horizontally with node count.

#### Chaos Controller (Shell scripts + SSH)

A collection of scripts that inject controlled failures into the cluster:

- **Leader kill** — stop/restart the leader process via SSH
- **Network partition** — isolate node subsets using `iptables` rules
- **Message delay/drop** — inject latency or packet loss via `tc` (traffic control)
- **Automated experiment runner** — a shell script that orchestrates failure scenarios sequentially (with timed steps and `sleep` between phases) and collects results

#### Monitoring and Data Collection

Each node writes structured JSON logs recording key metrics: current term, role, commit index, apply latency, and request throughput. Logs are collected to S3 after each experiment. A Python script processes the logs and generates analysis charts using matplotlib. No live dashboard or Grafana is needed — all visualizations are static charts for the final report.

### How It Needs to Scale

- **Fault tolerance** — 5-node cluster must tolerate up to 2 failures while maintaining consistency and availability, validated through chaos experiments
- **Read throughput scaling** — Follower reads (stale/bounded-staleness) enable horizontal read scaling; adding nodes should increase read throughput linearly
- **Graceful degradation** — As nodes fail or latency increases, the system should degrade predictably rather than catastrophically
- **Recovery speed** — Fast leader election and log catch-up minimize periods of reduced capacity

## 2. Experiments

### Experiment 1: Leader Crash Recovery and Election Performance

**Goal:** Measure cluster recovery speed after leader failures.

**Setup:** 5-node cluster under 200 writes/sec. Kill the leader at t=30s. Test with election timeouts of 150ms, 300ms, and 500ms. Also test back-to-back leader kills.

**Key Metrics:** Election time, client-visible downtime, writes lost during transition, throughput recovery curve.

**Expected:** Election completes within 1–2x the configured timeout. Zero committed writes are lost. Back-to-back kills stabilize within 2–3 election rounds.

### Experiment 2: Network Partition Behavior and Consistency Verification

**Goal:** Validate consistency under partitions — majority continues, minority halts, no split-brain.

**Setup:** Three scenarios under 200 writes/sec:

- **Minority partition (2 isolated)** — 3-node majority continues serving
- **Leader isolation** — leader placed in minority; majority elects new leader
- **Symmetric split (2-2-1)** — no majority exists; cluster halts writes

Heal each partition and run a correctness checker comparing all 5 nodes' committed state.

**Key Metrics:** Majority throughput during partition, minority write rejection rate (must be 100%), reconciliation time after healing, state consistency across all nodes post-recovery.

**Expected:** Majority partition serves throughout. Minority refuses all writes. After healing, isolated nodes catch up within seconds. Zero inconsistencies across all nodes.

### Experiment 3: Read Scalability and Consistency-Throughput Tradeoff

**Goal:** Measure how follower reads enable horizontal read scaling and quantify the consistency-vs-throughput tradeoff.

**Setup:**

- **Phase A (read scaling)** — 90% read / 10% write workload at 2,000 req/sec. Compare leader-only reads vs. follower reads with 3, 4, and 5 active nodes.
- **Phase B (consistency modes)** — Under 5,000 req/sec read load with 5 nodes, compare strong, default, and stale modes for throughput, latency, and staleness.

**Key Metrics:**

- Phase A: Read throughput scaling factor per node added
- Phase B: Throughput, latency (p50/p95/p99), and measured staleness per consistency mode

**Expected:** Follower reads scale near-linearly with node count. Stale reads achieve 3–5x the throughput of leader-only reads, with staleness typically under 50ms.

## 3. Team Responsibilities

| Member   | Responsibility                                                                                                                                    |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Person A | Raft core integration — node setup with `hashicorp/raft`, leader election config, FSM state machine (KV store logic)                              |
| Person B | HTTP API layer — embedded routing (write forwarding, read consistency modes), client load generator for experiments                               |
| Person C | AWS infrastructure — EC2 provisioning, deployment scripts, structured JSON logging, S3 log collection, monitoring                                 |
| Person D | Chaos controller — failure injection scripts (`iptables`, `tc`, process kill), experiment automation, data analysis and matplotlib visualizations |

## 4. Timeline

| Week   | Milestone                                                                           |
| ------ | ----------------------------------------------------------------------------------- |
| Week 1 | Raft + KV store working locally, AWS infra provisioned, chaos scripts drafted       |
| Week 2 | HTTP API with consistency modes complete, deploy to AWS, end-to-end cluster working |
| Week 3 | Run all chaos experiments, collect data to S3, begin analysis                       |
| Week 4 | Generate charts, write final report, polish and present                             |

## 5. Tech Stack

- **Language:** Go
- **Consensus:** `hashicorp/raft`
- **Infrastructure:** AWS EC2 (5× t3.micro), S3 for log storage
- **Chaos tooling:** Shell scripts, SSH, `iptables`, `tc`
- **Analysis:** Python, matplotlib
- **Client load generation:** Go (custom load generator) or `hey`/`wrk`

# Project Update

## 1. Group Members

Qian Li, Zhengyi Xu, Wenyu Yang, Siwen Wu. We are not looking for additional teammates.

## 2. Experiments and Rationale

**Experiment 1:** Leader Crash Recovery. Kill the leader of a 5-node cluster under 200 writes/sec and measure election time, downtime, and throughput recovery across different election timeouts (150ms, 300ms, 500ms). This validates Raft’s liveness guarantee and the f = (n−1)/2 fault tolerance bound.

**Experiment 2:** Network Partition Consistency. Inject three partition scenarios (minority isolation, leader isolation, symmetric 2-2-1 split), then verify all nodes converge to identical state after healing. This makes the CAP theorem concrete: we expect CP behavior where the majority serves, the minority rejects all writes, and no split-brain occurs.

**Experiment 3:** Read Scalability Tradeoffs. Compare leader-only reads vs. follower reads across 3, 4, and 5 nodes, then benchmark strong, bounded-staleness, and stale consistency modes under high load. This quantifies the fundamental consistency-vs-throughput tradeoff in distributed stores.

Together these three experiments cover fault tolerance, consistency under partition, and scalability, which are the three pillars of distributed systems design.

## 3. Value We Add Beyond AI

AI can generate boilerplate code and explain Raft conceptually, but cannot do what makes this project work. Debugging distributed systems requires correlating logs across multiple nodes and reasoning about timing and message ordering in real time. Our chaos experiments demand judgment calls: deciding whether an unexpected result is a bug, a misconfiguration, or a genuine protocol insight. The AWS deployment involves interactive, environment-specific infrastructure work (networking, security groups, process management) that requires adapting on the fly. Finally, connecting empirical results to distributed systems theory in the analysis requires genuine understanding of the protocols, not pattern matching. We provide the systems thinking, debugging, and analytical depth; AI accelerates the scaffolding.

## 4. Timeline

Deadline: April 20th (4 weeks).

Week 1 (Mar 24 to 30): Integrate hashicorp/raft with KV store FSM, get cluster running locally, provision AWS, draft chaos scripts.

Week 2 (Mar 31 to Apr 6): Build HTTP API with read consistency modes, deploy to AWS end-to-end, finalize chaos scripts and load generator.

Week 3 (Apr 7 to 13): Run all experiments, collect logs to S3, begin analysis and charts.

Week 4 (Apr 14 to 20): Complete analysis, write report, polish, and submit.
