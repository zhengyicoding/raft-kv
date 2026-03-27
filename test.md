# testing plan

# Full Testing Plan — Raft KV Store

## Phase 1: Local Functional Tests (Done ✅)

Basic correctness — does the system work at all?

| Test                           | Command                                    | Expected                          |
| ------------------------------ | ------------------------------------------ | --------------------------------- |
| Write to leader                | `curl -X PUT :17001/key/hello -d "world"`  | 204 No Content                    |
| Strong read from leader        | `curl ":17001/key/hello?level=strong"`     | `{"key":"hello","value":"world"}` |
| Stale read from follower       | `curl ":17002/key/hello?level=stale"`      | `{"key":"hello","value":"world"}` |
| Default read from follower     | `curl ":17003/key/hello?level=default"`    | `{"key":"hello","value":"world"}` |
| Write forwarding from follower | `curl -X PUT :17002/key/foo -d "bar"`      | 204 (silently forwarded)          |
| Delete a key                   | `curl -X DELETE :17001/key/hello`          | 204 No Content                    |
| Key not found                  | `curl ":17001/key/hello?level=strong"`     | 404                               |
| Health check                   | `curl :17001/health`                       | `{"role":"leader",...}`           |
| Leader failover                | Kill node1, check :17002 and :17003 health | New leader elected                |
| Data survives failover         | Read key written before kill               | Value still present               |

---

## Phase 2: Consistency Mode Tests

Validate the three read consistency levels behave correctly.

### 2a. All three levels return same value (normal conditions)

```bash
curl -X PUT :17001/key/consistency -d "test"
curl ":17001/key/consistency?level=strong"   # leader, linearizable
curl ":17002/key/consistency?level=default"  # follower, bounded staleness
curl ":17003/key/consistency?level=stale"    # follower, no check
# All three should return "test"
```

### 2b. Strong read on follower gets forwarded

```bash
curl ":17002/key/consistency?level=strong"
# Should return value (forwarded to leader), not an error
```

### 2c. Default read rejects stale follower

```bash
# Pause replication artificially (disconnect node2 briefly)
# Then immediately write and read with default level
curl -X PUT :17001/key/staletest -d "fresh"
curl ":17002/key/staletest?level=default"
# If node2 is lagging > 2s, should forward to leader
# If node2 is caught up, should serve locally
```

### 2d. Nonexistent key on all levels

```bash
curl ":17001/key/doesnotexist?level=strong"   # 404
curl ":17002/key/doesnotexist?level=stale"    # 404
curl ":17003/key/doesnotexist?level=default"  # 404
```

---

## Phase 3: Experiment 1 — Leader Crash Recovery

**Goal:** Measure election time and client-visible downtime across different election timeout configurations.

### Setup

- 5-node cluster on EC2
- Background write load: 200 writes/sec via load generator
- Kill leader at t=30s

### 3a. Election timeout sweep

Run the full experiment three times with different timeouts:

```bash
# Run 1
ELECTION_TIMEOUT=150 HEARTBEAT_TIMEOUT=75 ./raft-kv ...

# Run 2
ELECTION_TIMEOUT=300 HEARTBEAT_TIMEOUT=150 ./raft-kv ...

# Run 3
ELECTION_TIMEOUT=500 HEARTBEAT_TIMEOUT=250 ./raft-kv ...
```

**Measure for each run:**

- Time from leader kill to new leader elected (election time)
- Number of write failures during transition
- Time for throughput to return to baseline (recovery curve)
- Which node won the election

### 3b. Back-to-back leader kills

```bash
# Kill leader, wait for new election, immediately kill new leader
# Repeat 3 times
# Measure: does cluster stabilize within 2-3 election rounds?
```

**Expected results:**

- Election completes within 1-2x configured timeout
- Zero committed writes lost
- Back-to-back kills stabilize within 3 rounds

---

## Phase 4: Experiment 2 — Network Partition

**Goal:** Validate Raft safety guarantees under three partition topologies.

### 4a. Minority partition (2 nodes isolated)

```bash
# Isolate node4 and node5 using iptables
sudo iptables -A INPUT -s <node4-ip> -j DROP
sudo iptables -A INPUT -s <node5-ip> -j DROP

# Majority (nodes 1-3) should continue serving
curl -X PUT :17001/key/during-partition -d "majority"  # should succeed

# Minority should reject writes
curl -X PUT :17004/key/minority-write -d "test"  # should fail/timeout

# Heal partition
sudo iptables -F

# Verify nodes 4-5 catch up
sleep 5
curl ":17004/key/during-partition?level=stale"  # should return "majority"
```

### 4b. Leader isolation (leader in minority)

```bash
# Isolate the current leader from the other 4 nodes
# Majority should elect a new leader
# Old leader should step down (can't reach quorum)
# Verify no split-brain: old leader rejects writes
```

### 4c. Symmetric split (2-2-1, no majority)

```bash
# Partition into groups of 2, 2, and 1
# No group has majority (need 3 of 5)
# ALL writes should fail cluster-wide
# After healing: cluster recovers, all nodes consistent
```

### 4d. Post-healing correctness check

After healing each partition, run the correctness checker across all 5 nodes:

```bash
python3 correctness_check.py \
  --nodes "node1:17001,node2:17002,node3:17003,node4:17004,node5:17005"
# Verifies all nodes return identical values for all keys
# Zero divergence = Raft safety property holds
```

---

## Phase 5: Experiment 3 — Read Scalability

**Goal:** Measure how follower reads scale horizontally and quantify consistency-throughput tradeoff.

### 5a. Phase A — Read scaling with node count

```bash
# 90% read / 10% write workload at 2000 req/sec
# Compare leader-only reads vs follower reads

# Round 1: 3 nodes, leader-only reads
locust -f locustfile.py --host=http://<leader-ip>:17001 \
  --users 200 --spawn-rate 20 --read-level=strong

# Round 2: 3 nodes, follower reads (distribute across all nodes)
locust -f locustfile.py --host=http://<any-node>:17001 \
  --users 200 --spawn-rate 20 --read-level=stale

# Round 3: 4 nodes, follower reads
# Round 4: 5 nodes, follower reads

# Measure: read throughput scaling factor per node added
```

### 5b. Phase B — Consistency mode comparison

```bash
# 5 nodes, 5000 req/sec read load
# Run three separate tests:

# Test 1: strong reads (all go to leader)
locust --read-level=strong --users 500

# Test 2: default reads (bounded staleness)
locust --read-level=default --users 500

# Test 3: stale reads (any follower)
locust --read-level=stale --users 500

# Measure per mode: p50/p95/p99 latency, throughput, staleness
```

**Expected results:**

- Stale reads scale near-linearly with node count
- Stale reads achieve 3-5x throughput of strong reads
- Staleness typically under 50ms

---

## Phase 6: Correctness Checker

A Python script that verifies no divergence across all nodes after chaos experiments.

```python
# correctness_check.py — run after every partition experiment
import requests

nodes = ["localhost:17001", "localhost:17002", "localhost:17003"]
keys_to_check = ["hello", "foo", "during-partition", ...]

divergences = []
for key in keys_to_check:
    values = {}
    for node in nodes:
        r = requests.get(f"http://{node}/key/{key}?level=stale")
        values[node] = r.json().get("value") if r.status_code == 200 else "NOT_FOUND"

    if len(set(values.values())) > 1:
        divergences.append({"key": key, "values": values})

if divergences:
    print(f"❌ DIVERGENCE DETECTED: {divergences}")
else:
    print("✅ All nodes consistent — no divergence")
```

---

## Summary Checklist

Phase 1: Local functional tests ✅ Done
Phase 2: Consistency mode tests ✅ Done
Phase 3: Experiment 1 (leader crash) ⬜ After EC2 deploy
Phase 4: Experiment 2 (partitions) ⬜ After EC2 deploy
Phase 5: Experiment 3 (read scaling) ⬜ After EC2 deploy
Phase 6: Correctness checker ✅ Done
