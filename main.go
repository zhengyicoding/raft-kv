package main

import (
	"fmt"
	"log"
	"time"

	httpserver "raft-kv/http"
	"raft-kv/node"
	"raft-kv/store"
)

func main() {
	// --- Bootstrap node1 (the initial leader) ---
	n1, err := node.NewNode(node.Config{
		NodeID:    "node1",
		BindAddr:  "127.0.0.1:7001",
		DataDir:   "/tmp/raft-kv/node1",
		Bootstrap: true,
	})
	if err != nil {
		log.Fatalf("node1: %v", err)
	}

	// --- Start node2 and node3 (joiners) ---
	n2, err := node.NewNode(node.Config{
		NodeID:   "node2",
		BindAddr: "127.0.0.1:7002",
		DataDir:  "/tmp/raft-kv/node2",
	})
	if err != nil {
		log.Fatalf("node2: %v", err)
	}

	n3, err := node.NewNode(node.Config{
		NodeID:   "node3",
		BindAddr: "127.0.0.1:7003",
		DataDir:  "/tmp/raft-kv/node3",
	})
	if err != nil {
		log.Fatalf("node3: %v", err)
	}

	s1 := store.New(n1)
	s2 := store.New(n2)
	s3 := store.New(n3)

	// Wait for node1 to elect itself leader (retry up to 10s)
	fmt.Println("Waiting for node1 to become leader...")
	deadline := time.Now().Add(10 * time.Second)
	for !s1.IsLeader() {
		if time.Now().After(deadline) {
			log.Fatal("timed out waiting for node1 to become leader")
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println("node1 is leader, joining node2 and node3...")

	// Join node2 and node3 into the cluster via node1 (the leader)
	if err := s1.AddVoter("node2", "127.0.0.1:7002"); err != nil {
		log.Fatalf("add node2: %v", err)
	}
	if err := s1.AddVoter("node3", "127.0.0.1:7003"); err != nil {
		log.Fatalf("add node3: %v", err)
	}

	// Give the cluster time to stabilize
	time.Sleep(300 * time.Millisecond)

	// --- Start HTTP servers (Raft port + 10000) ---
	// node1 → :17001, node2 → :17002, node3 → :17003
	for addr, s := range map[string]*store.Store{
		":17001": s1,
		":17002": s2,
		":17003": s3,
	} {
		srv := httpserver.NewServer(addr, s)
		go func(srv *httpserver.Server) {
			if err := srv.Start(); err != nil {
				log.Printf("HTTP server error: %v", err)
			}
		}(srv)
	}
	time.Sleep(200 * time.Millisecond)

	// --- Smoke test ---
	fmt.Println("Writing key 'hello' = 'world' via leader (node1)...")
	if err := s1.Put("hello", "world"); err != nil {
		log.Fatalf("put: %v", err)
	}

	// Wait for replication
	time.Sleep(100 * time.Millisecond)

	// Read from all three nodes (stale reads from followers)
	for name, s := range map[string]*store.Store{"node1": s1, "node2": s2, "node3": s3} {
		v, ok := s.Get("hello")
		fmt.Printf("  %s → hello=%q (found=%v)\n", name, v, ok)
	}

	fmt.Println("Done. Cluster is working locally.")
	fmt.Println("HTTP servers running. Press Ctrl+C to stop.")

	// Block forever — keep the cluster alive
	select {}
}
