# CS 6650 Milestone 1 Report

## Distributed Key-Value Store with Raft Consensus and Chaos Engineering

**Date:** March 27, 2026
**Team:** Qian Li, Zhengyi Xu, Wenyu Yang, Siwen Wu

**Repo:** https://github.com/zhengyicoding/raft-kv

---

## 1. Problem, Team, and Overview of Experiments

Distributed systems must stay consistent and available under partial failures — node crashes, network partitions, and leader failures are expected operational conditions, not edge cases. Production systems like etcd (Kubernetes), CockroachDB, and TiKV rely on Raft consensus to guarantee that all nodes agree on the same state before acknowledging a write. Building and validating these guarantees from scratch requires deep distributed systems knowledge, careful engineering, and systematic fault injection.

We are building a distributed key-value store powered by Raft, deployed as a 5-node cluster on AWS EC2, with a Chaos Engineering toolkit to systematically inject failures and validate fault tolerance. We also implement tunable read consistency (strong, bounded-staleness, stale) to explore the consistency-vs-throughput tradeoff.

**Team members:** All four members independently implemented the full stack this week — Raft core integration, HTTP API, Store API, and local testing — to ensure deep familiarity with every layer before converging on a shared codebase for deployment.

| Member     | This Week                                           | Next Week                              |
| ---------- | --------------------------------------------------- | -------------------------------------- |
| Qian Li    | Full stack implementation + local tests             | Experiment 1: Leader crash recovery    |
| Zhengyi Xu | Full stack + correctness checker + env var refactor | Experiment 2: Network partition        |
| Wenyu Yang | Full stack implementation + local tests             | Experiment 3: Read scalability         |
| Siwen Wu   | Full stack implementation + local tests             | AWS EC2 infrastructure + observability |

**Experiments overview:** Three experiments cover the three pillars of distributed systems — fault tolerance, consistency under partition, and scalability. (1) Leader Crash Recovery validates Raft's liveness guarantee under different election timeouts. (2) Network Partition Consistency validates the CAP theorem CP behavior across three partition topologies. (3) Read Scalability quantifies the consistency-vs-throughput tradeoff across our three read modes.

**Role of AI:** Claude was used to accelerate scaffolding and debug Go issues. All experiment design, infrastructure operation, and result interpretation is human-driven. We discuss AI tradeoffs further in the Methodology section.

**Observability:** Each node emits structured JSON logs (term, role, commit index, apply latency, throughput) collected to S3 after each experiment. A `/health` endpoint exposes real-time cluster state. Python/matplotlib generates analysis charts from logs.

---

## 2. Project Plan and Timeline

**Deadline: April 20 (4 weeks total)**

| Week | Dates        | Milestone                                                                          | Status         |
| ---- | ------------ | ---------------------------------------------------------------------------------- | -------------- |
| 1    | Mar 24–30    | Full stack locally, individual implementations, correctness checker                | ✅ Complete    |
| 2    | Mar 31–Apr 6 | Converge on codebase, deploy to AWS EC2, finalize load generator and chaos scripts | 🔄 In progress |
| 3    | Apr 7–13     | Run all experiments, collect logs to S3, begin analysis                            | ⬜ Upcoming    |
| 4    | Apr 14–20    | Complete analysis, charts, final report, present                                   | ⬜ Upcoming    |

**Task breakdown (Week 2 onward):**

| Task                                                          | Owner   | Due    |
| ------------------------------------------------------------- | ------- | ------ |
| Review individual implementations, agree on combined codebase | All     | Apr 1  |
| Terraform: provision 5 EC2 t3.micro instances                 | Siwen   | Apr 3  |
| Deploy binary to EC2, verify end-to-end cluster               | Siwen   | Apr 5  |
| Locust load generator for all experiments                     | Wenyu   | Apr 5  |
| Experiment 1: leader crash sweep (150/300/500ms)              | Qian    | Apr 10 |
| Experiment 2: partition topologies + correctness check        | Zhengyi | Apr 10 |
| Experiment 3: read scaling + consistency mode benchmark       | Wenyu   | Apr 10 |
| S3 log collection + matplotlib charts                         | Siwen   | Apr 12 |
| Final report + presentation                                   | All     | Apr 20 |

---

## 3. System Architecture

Each of the 5 EC2 nodes runs a single Go binary configured via environment variables (`NODE_ID`, `RAFT_ADDR`, `HTTP_ADDR`, `BOOTSTRAP`, `ELECTION_TIMEOUT`). Non-bootstrap nodes join dynamically via the leader's `/join` endpoint.

**Core layers:**

- **FSM (`node/fsm.go`)** — Raft state machine implementing PUT/DELETE/GET. `Apply()` is the only place state is mutated, ensuring all nodes apply operations in the same order. BoltDB-backed snapshotting for crash recovery.
- **Node (`node/node.go`)** — wires `hashicorp/raft` with BoltDB log/stable store, TCP transport, and file snapshot store.
- **Store (`store/store.go`)** — public KV API. Writes go through `raft.Apply()`. Reads use `Barrier()` for strong, `LastAppliedAt` timestamp for default bounded-staleness, and direct FSM reads for stale.
- **HTTP (`http/handler.go`)** — `GET /key/{key}?level=strong|default|stale`, `PUT`, `DELETE`, `GET /health`, `GET /join`. Writes and strong reads on followers are **transparently forwarded** to the leader.

---

## 4. Preliminary Results

### 4.1 Local Functional Tests (16/16 Passed)

| Category    | Test                                        | Result      |
| ----------- | ------------------------------------------- | ----------- |
| Functional  | Write to leader                             | ✅          |
| Functional  | Read — strong / default / stale             | ✅          |
| Functional  | Delete + 404 verification                   | ✅          |
| Functional  | Transparent write forwarding from follower  | ✅          |
| Functional  | Health endpoint                             | ✅          |
| Consistency | All 3 levels return same value              | ✅          |
| Consistency | Strong read on follower forwarded to leader | ✅          |
| Consistency | Nonexistent key 404 on all levels           | ✅          |
| Failover    | Leader killed → new election observed       | ✅          |
| Failover    | Writes continue after failover              | ✅          |
| Failover    | Pre-failover data survives                  | ✅          |
| Failover    | Node restart + rejoin as follower           | ✅          |
| Correctness | Zero divergence — healthy cluster           | ✅ 5/5 keys |
| Correctness | Zero divergence — after leader kill         | ✅ 5/5 keys |
| Persistence | BoltDB log replayed on restart              | ✅          |

### 4.2 Correctness Checker Results

The correctness checker writes known keys to the leader, waits for replication, then reads from all nodes and asserts zero divergence.

**Healthy 3-node cluster:**

```
node1 ✅ follower | node2 ✅ follower | node3 👑 leader
Keys checked: 5 | Diverged: 0
✅ ALL NODES CONSISTENT — Raft safety property holds
```

**After killing the leader (node3):**

```
node1 ✅ follower | node2 👑 leader | node3 ❌ unreachable
Keys checked: 5 | Diverged: 0
✅ ALL NODES CONSISTENT — Raft safety property holds
```

Zero divergence even during active failover — Raft's safety property holds under leader failure.

### 4.3 What Remains

All three formal experiments are pending EC2 deployment. Locust load testing, `iptables` partition injection, and quantitative throughput/latency charts will be produced in Week 3.

**Anticipated worst-case workload:** The symmetric 2-2-1 partition — no group has majority, all writes halt cluster-wide. This is the most extreme availability degradation in Raft and the most critical to verify correctly.

---

## 5. Related Work and Differentiation

**Raft (Ongaro & Ousterhout, 2014)** — primary design reference. Our implementation follows the paper's leader election and log replication via `hashicorp/raft`.

**CAP Theorem (Brewer, 2000)** — foundational framing for Experiment 2. Raft is CP: majority serves, minority halts, no split-brain.

**Dynamo (DeCandia et al., 2007)** — conceptual counterpoint. Our three read consistency modes mirror the Dynamo R+W>N quorum tradeoff made explicit.

**Related class projects:**

**Project: Sharded Distributed KV Store**
Link: https://piazza.com/class/mk3hftotl6e229/post/1124
The most architecturally similar project. Both use `hashicorp/raft` in Go for replication. Their project adds consistent-hashing-based sharding across multiple Raft groups for horizontal write scaling; ours uses a single 5-node Raft cluster and focuses on chaos engineering depth and formal correctness verification. Their unique contribution is the sharding architecture; ours is the fault injection rigor and post-healing safety verification.

**Project: Fault-Tolerant Distributed Rate Limiter**
Link: https://piazza.com/class/mk3hftotl6e229/post/1119
Also uses `hashicorp/raft` in Go, but applies Raft to replicate token bucket state rather than a KV store. Their unique contribution is a direct Raft vs. Redis comparison quantifying the consistency-vs-latency tradeoff against an industry-standard tool. Our projects share leader crash and network partition experiments but differ in chaos engineering depth — we test three partition topologies with formal post-healing verification across all nodes.

**Project: High-Availability L7 Load Balancer**
Link: https://piazza.com/class/mk3hftotl6e229/post/957
The least overlapping project. They build a Layer 7 load balancer using Redis pub/sub for distributed state coordination on ECS Fargate. The conceptual connection is their Experiment 3 (horizontal scaling vs. Redis state contention) which asks the same question as our Experiment 3 (follower read scaling vs. leader bottleneck) — does adding nodes help throughput, or does shared state become the bottleneck? The key difference is they use an eventually-consistent managed service; we implement the consistency protocol ourselves.

---

## Submission Checklist

| Deliverable                                                   | Status                                            |
| ------------------------------------------------------------- | ------------------------------------------------- |
| ✅ Repo link                                                  | https://github.com/zhengyicoding/raft-kv          |
| ✅ Project plan (MeisterTask or similar)                      | https://github.com/users/zhengyicoding/projects/1 |
| ✅ Initial results (correctness checker, 16 functional tests) | In Section 4                                      |
| ⬜ 2-minute elevator pitch video                              | To be recorded                                    |
| ✅ Report (≤5 pages)                                          | This document                                     |
