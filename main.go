package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	httpserver "raft-kv/http"
	"raft-kv/node"
	"raft-kv/store"
)

func main() {
	// --- Read config from environment variables ---
	nodeID := mustEnv("NODE_ID")                  // e.g. "node1"
	raftAddr := mustEnv("RAFT_ADDR")              // e.g. "127.0.0.1:7001"
	httpAddr := mustEnv("HTTP_ADDR")              // e.g. ":17001"
	dataDir := mustEnv("DATA_DIR")                // e.g. "/tmp/raft-kv/node1"
	bootstrap := os.Getenv("BOOTSTRAP") == "true" // only node1
	joinAddr := os.Getenv("JOIN_ADDR")            // e.g. "127.0.0.1:7001" for non-bootstrap nodes

	// --- Start this node ---
	n, err := node.NewNode(node.Config{
		NodeID:    nodeID,
		BindAddr:  raftAddr,
		DataDir:   dataDir,
		Bootstrap: bootstrap,
	})
	if err != nil {
		log.Fatalf("failed to start node: %v", err)
	}

	s := store.New(n)

	// --- Join the cluster if not bootstrapping ---
	if !bootstrap && joinAddr != "" {
		// Wait for this node's Raft to be ready, then ask the leader to add us
		time.Sleep(500 * time.Millisecond)
		if err := joinCluster(joinAddr, nodeID, raftAddr); err != nil {
			log.Fatalf("failed to join cluster: %v", err)
		}
	}

	// --- Start HTTP server ---
	srv := httpserver.NewServer(httpAddr, s)
	fmt.Printf("Node %s started — Raft: %s  HTTP: %s\n", nodeID, raftAddr, httpAddr)

	if err := srv.Start(); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// joinCluster calls the leader's /join endpoint to add this node to the cluster.
func joinCluster(leaderHTTPAddr, nodeID, raftAddr string) error {
	url := fmt.Sprintf("http://%s/join?id=%s&addr=%s", leaderHTTPAddr, nodeID, raftAddr)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("join request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("join request returned %d", resp.StatusCode)
	}
	return nil
}

// mustEnv reads a required environment variable or exits.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}
