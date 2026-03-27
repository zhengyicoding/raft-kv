#!/usr/bin/env python3
"""
Correctness checker for the Raft KV store.

Usage:
    # Basic check against local 3-node cluster
    python3 correctness_check.py

    # Custom nodes
    python3 correctness_check.py --nodes "localhost:17001,localhost:17002,localhost:17003"

    # After a partition experiment — just check, don't write
    python3 correctness_check.py --check-only --keys "hello,foo,during-partition"

    # Against EC2 cluster
    python3 correctness_check.py --nodes "10.0.1.1:17001,10.0.1.2:17001,10.0.1.3:17001"
"""

import argparse
import json
import time
import requests
import sys
from typing import Optional

# ─── Config ───────────────────────────────────────────────────────────────────

DEFAULT_NODES = [
    "localhost:17001",
    "localhost:17002",
    "localhost:17003",
]

# Keys written during the correctness check
TEST_KEYS = {
    "cc-key-1": "value-alpha",
    "cc-key-2": "value-beta",
    "cc-key-3": "value-gamma",
    "cc-key-4": "value-delta",
    "cc-key-5": "value-epsilon",
}

TIMEOUT = 5  # seconds per request

# ─── Helpers ──────────────────────────────────────────────────────────────────


def find_leader(nodes: list[str]) -> Optional[str]:
    """Return the HTTP address of the current leader."""
    for node in nodes:
        try:
            r = requests.get(f"http://{node}/health", timeout=TIMEOUT)
            if r.status_code == 200 and r.json().get("role") == "leader":
                return node
        except requests.RequestException:
            continue
    return None


def write_keys(leader: str, keys: dict[str, str]) -> list[str]:
    """Write all test keys to the leader. Returns list of failed keys."""
    failed = []
    for key, value in keys.items():
        try:
            r = requests.put(f"http://{leader}/key/{key}", data=value, timeout=TIMEOUT)
            if r.status_code not in (200, 204):
                failed.append(key)
        except requests.RequestException as e:
            print(f"  ⚠️  Write failed for {key}: {e}")
            failed.append(key)
    return failed


def read_key(node: str, key: str, level: str = "stale") -> Optional[str]:
    """Read a key from a node. Returns value or None if not found."""
    try:
        r = requests.get(f"http://{node}/key/{key}?level={level}", timeout=TIMEOUT)
        if r.status_code == 200:
            return r.json().get("value")
        elif r.status_code == 404:
            return "NOT_FOUND"
        else:
            return f"ERROR_{r.status_code}"
    except requests.RequestException as e:
        return f"UNREACHABLE ({e})"


def check_node_health(nodes: list[str]) -> dict[str, str]:
    """Return role of each node."""
    health = {}
    for node in nodes:
        try:
            r = requests.get(f"http://{node}/health", timeout=TIMEOUT)
            if r.status_code == 200:
                data = r.json()
                health[node] = data.get("role", "unknown")
            else:
                health[node] = "unreachable"
        except requests.RequestException:
            health[node] = "unreachable"
    return health


# ─── Core checker ─────────────────────────────────────────────────────────────


def run_correctness_check(
    nodes: list[str],
    keys_to_check: dict[str, str],
    write_first: bool = True,
    wait_seconds: float = 0.5,
) -> bool:
    """
    Main correctness check.
    Returns True if all nodes are consistent, False if divergence detected.
    """
    print("\n" + "=" * 60)
    print("  RAFT KV STORE — CORRECTNESS CHECKER")
    print("=" * 60)

    # ── Step 1: Cluster health ────────────────────────────────────
    print("\n[1] Cluster health:")
    health = check_node_health(nodes)
    for node, role in health.items():
        icon = "👑" if role == "leader" else ("✅" if role == "follower" else "❌")
        print(f"    {icon}  {node}  →  {role}")

    reachable = [n for n, r in health.items() if r != "unreachable"]
    if not reachable:
        print("\n❌ No nodes reachable. Is the cluster running?")
        return False

    # ── Step 2: Write test keys ───────────────────────────────────
    if write_first:
        leader = find_leader(nodes)
        if not leader:
            print("\n⚠️  No leader found — skipping writes (check-only mode)")
        else:
            print(
                f"\n[2] Writing {len(keys_to_check)} test keys to leader ({leader})..."
            )
            failed = write_keys(leader, keys_to_check)
            if failed:
                print(f"    ⚠️  Failed to write: {failed}")
            else:
                print(f"    ✅ All keys written successfully")

            # Wait for replication
            print(f"    ⏳ Waiting {wait_seconds}s for replication...")
            time.sleep(wait_seconds)
    else:
        print("\n[2] Skipping writes (--check-only mode)")

    # ── Step 3: Read from all nodes ───────────────────────────────
    print(f"\n[3] Reading {len(keys_to_check)} keys from {len(reachable)} nodes...")
    results = {}  # key → {node → value}
    for key in keys_to_check:
        results[key] = {}
        for node in reachable:
            results[key][node] = read_key(node, key)

    # ── Step 4: Check for divergence ─────────────────────────────
    print("\n[4] Checking for divergence...")
    divergences = []
    consistent_keys = []

    for key, node_values in results.items():
        unique_values = set(node_values.values())
        if len(unique_values) > 1:
            divergences.append({"key": key, "values": node_values})
        else:
            consistent_keys.append(key)

    # ── Step 5: Report ────────────────────────────────────────────
    print("\n[5] Results:")
    print(f"    Keys checked:    {len(keys_to_check)}")
    print(f"    Nodes checked:   {len(reachable)}")
    print(f"    Consistent keys: {len(consistent_keys)}")
    print(f"    Diverged keys:   {len(divergences)}")

    if divergences:
        print("\n❌ DIVERGENCE DETECTED — Raft safety property violated!")
        print("-" * 60)
        for d in divergences:
            print(f"\n  Key: '{d['key']}'")
            for node, value in d["values"].items():
                print(f"    {node}  →  {repr(value)}")
        print("-" * 60)
        return False
    else:
        print("\n✅ ALL NODES CONSISTENT — No divergence detected")
        print("   Raft safety property holds ✓")

        # Print the consistent values for reference
        print("\n   Key values across all nodes:")
        for key, node_values in results.items():
            value = list(node_values.values())[0]
            expected = keys_to_check.get(key, "?")
            match = "✓" if value == expected else "⚠️ unexpected"
            print(f"   {key}  =  {repr(value)}  {match}")

        return True


# ─── Entry point ──────────────────────────────────────────────────────────────


def main():
    parser = argparse.ArgumentParser(description="Raft KV Store Correctness Checker")
    parser.add_argument(
        "--nodes",
        default=",".join(DEFAULT_NODES),
        help="Comma-separated list of node HTTP addresses",
    )
    parser.add_argument(
        "--check-only",
        action="store_true",
        help="Skip writing — only read and compare existing keys",
    )
    parser.add_argument(
        "--keys",
        default=None,
        help="Comma-separated list of keys to check (for --check-only mode)",
    )
    parser.add_argument(
        "--wait",
        type=float,
        default=0.5,
        help="Seconds to wait after writing before reading (default: 0.5)",
    )
    args = parser.parse_args()

    nodes = [n.strip() for n in args.nodes.split(",")]

    # Build keys dict
    if args.check_only and args.keys:
        keys = {k.strip(): None for k in args.keys.split(",")}
    else:
        keys = TEST_KEYS

    ok = run_correctness_check(
        nodes=nodes,
        keys_to_check=keys,
        write_first=not args.check_only,
        wait_seconds=args.wait,
    )

    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
