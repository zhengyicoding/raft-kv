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

In Week 1, every member independently built the entire Raft KV system end-to-end (FSM, HTTP API, Raft setup, local testing). With AI-assisted learning (Claude Code), this was feasible and gave every member a complete understanding of the full distributed systems stack.

Starting Week 2, we divide experiment and infrastructure work as follows:

| Member | Primary Responsibility                                                          |
| ------ | ------------------------------------------------------------------------------- |
| Qian     | EC2 provisioning (Terraform) + structured JSON logging + data analysis (charts) |
| Zhengyi      | Experiment 1: Leader Crash Recovery (chaos script + run experiment)             |
| Wenyu      | Experiment 2: Network Partition Behavior (chaos script + run experiment)        |
| Siwen      | Experiment 3: Read Scalability + shared load generator                          |


**Dependencies:** Exp 1 and Exp 2 owners can write chaos scripts immediately. D builds the load generator, which all three experiments reuse. A's EC2 provisioning must complete before any experiment runs on AWS. After chaos scripts are written, A adds structured JSON logging into the scripts and the Go application, then builds the S3 collection and chart generation pipeline. Final report and video are shared across all members.

**Experiments overview:** Three experiments cover the three pillars of distributed systems — fault tolerance, consistency under partition, and scalability. (1) Leader Crash Recovery validates Raft's liveness guarantee under different election timeouts. (2) Network Partition Consistency validates the CAP theorem CP behavior across three partition topologies. (3) Read Scalability quantifies the consistency-vs-throughput tradeoff across our three read modes.

**Role of AI:** Claude was used to accelerate scaffolding and debug Go issues. All experiment design, infrastructure operation, and result interpretation is human-driven. We discuss AI tradeoffs further in the Methodology section.

**Observability:** Each node emits structured JSON logs (term, role, commit index, apply latency, throughput) collected to S3 after each experiment. A `/health` endpoint exposes real-time cluster state. Python/matplotlib generates analysis charts from logs.

---

## 2. Project Plan and Timeline

### Timeline

| Week                  | Milestone                                                                              | Status       |
| --------------------- | -------------------------------------------------------------------------------------- | ------------ |
| Week 1 (Mar 23–29)    | Raft + KV store + HTTP API working locally, local 5-node cluster tested                | **Complete** |
| Week 2 (Mar 30–Apr 5) | AWS EC2 provisioning, deploy to AWS, chaos scripts drafted, end-to-end cluster working | Planned      |
| Week 3 (Apr 6–12)     | Run all chaos experiments, collect data to S3, begin analysis                          | Planned      |
| Week 4 (Apr 13–19)    | Generate charts, write final report, polish and present                                | Planned      |

### Task Breakdown

**Phase 1 — Done (all members independently):**

| Task                                                      | Status             |
| --------------------------------------------------------- | ------------------ |
| FSM implementation (`Apply`, `Snapshot`, `Restore`)       | Done (all members) |
| HTTP API with GET/PUT/DELETE and 3 read consistency modes | Done (all members) |
| Raft node setup (`main.go`) with configurable timeouts    | Done (all members) |
| Local 5-node cluster testing and validation               | Done (all members) |

**Phase 2 — Week 2 (parallel work):**

| Task                                                                   | Owner | Status      |
| ---------------------------------------------------------------------- | ----- | ----------- |
| EC2 provisioning with Terraform                                        | Qian     | In progress |
| Exp 1 chaos script (leader kill, back-to-back kill)                    | Zhengyi     | Planned     |
| Exp 2 chaos script (`iptables` partition + heal)                       | Wenyu     | Planned     |
| Load generator (configurable rate, read/write ratio, consistency mode) | Siwen     | Planned     |

**Phase 3 — After chaos scripts done:**

| Task                                                  | Owner | Status  |
| ----------------------------------------------------- | ----- | ------- |
| Add structured JSON logging to Go app + chaos scripts | Qian     | Planned |
| S3 log collection pipeline                            | Qian     | Planned |

**Phase 4 — Week 3 (run experiments on EC2):**

| Task             | Owner | Status  |
| ---------------- | ----- | ------- |
| Run Experiment 1 | Zhengyi     | Planned |
| Run Experiment 2 | Wenyu     | Planned |
| Run Experiment 3 | Siwen     | Planned |

**Phase 5 — Week 4 (all members):**

| Task                                      | Owner       | Status  |
| ----------------------------------------- | ----------- | ------- |
| Generate charts from S3 logs (matplotlib) | Qian           | Planned |
| Write final report                        | All members | Planned |
| Record video                              | All members | Planned |


## 3. System Architecture

Each of the 5 EC2 nodes runs a single Go binary configured via environment variables (`NODE_ID`, `RAFT_ADDR`, `HTTP_ADDR`, `BOOTSTRAP`, `ELECTION_TIMEOUT`). Non-bootstrap nodes join dynamically via the leader's `/join` endpoint.

### Core layers:

- **FSM (`node/fsm.go`)** — Raft state machine implementing PUT/DELETE/GET. `Apply()` is the only place state is mutated, ensuring all nodes apply operations in the same order. BoltDB-backed snapshotting for crash recovery.
- **Node (`node/node.go`)** — wires `hashicorp/raft` with BoltDB log/stable store, TCP transport, and file snapshot store.
- **Store (`store/store.go`)** — public KV API. Writes go through `raft.Apply()`. Reads use `Barrier()` for strong, `LastAppliedAt` timestamp for default bounded-staleness, and direct FSM reads for stale.
- **HTTP (`http/handler.go`)** — `GET /key/{key}?level=strong|default|stale`, `PUT`, `DELETE`, `GET /health`, `GET /join`. Writes and strong reads on followers are **transparently forwarded** to the leader.

### Read Consistency Modes

| Mode      | Behavior                                                                                              | Tradeoff                                             |
| --------- | ----------------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| `strong`  | Must be served by the leader                                                                          | Linearizable, but all reads hit one node             |
| `default` | Served by any node within a configurable staleness threshold (`LastIndex - AppliedIndex ≤ threshold`) | Balanced — bounded staleness with horizontal scaling |
| `stale`   | Served by any node immediately                                                                        | Lowest latency, but may return stale data            |

### Chaos Engineering Approach

We apply the principles of chaos engineering to systematically validate fault tolerance:

- **Leader kill:** SSH into the leader node and kill the process. Measure time to new election and client-visible downtime.
- **Network partition:** Use `iptables` rules to isolate node subsets. Verify majority continues, minority halts, and state converges after healing.
- **Latency injection:** Use `tc` (traffic control) to add artificial delay. Measure impact on commit latency and throughput.

### Experiments in Detail

**Experiment 1: Leader Crash Recovery**

- 5-node cluster under 200 writes/sec
- Kill leader at t=30s
- Test with election timeouts: 150ms, 300ms, 500ms
- Also test back-to-back leader kills
- Metrics: election time, client downtime, writes lost, throughput recovery curve

**Experiment 2: Network Partition Behavior**

- Three scenarios under 200 writes/sec:
  - Minority partition (2 nodes isolated) — majority continues
  - Leader isolation — majority elects new leader
  - Symmetric split (2-2-1) — no majority, cluster halts
- After healing: correctness checker compares all 5 nodes' state
- Metrics: throughput during partition, write rejection rate, reconciliation time

**Experiment 3: Read Scalability**

- Phase A: 90% read / 10% write at 2,000 req/sec. Compare leader-only vs. follower reads with 3, 4, 5 nodes.
- Phase B: 5,000 req/sec reads with 5 nodes. Compare strong, default, stale modes.
- Metrics: throughput scaling factor, latency (p50/p95/p99), measured staleness


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
| ✅ Initial results (correctness checker, 16 functional tests) | In Section 4 and details in local_test folder     |
| ⬜ 2-minute elevator pitch video                              | https://youtu.be/aeLkNwBYk5M                      |
| ✅ Report (≤5 pages)                                          | This document                                     |
