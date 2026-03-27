package node

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
)

// Config holds the parameters for a single Raft node.
type Config struct {
	NodeID    string // unique ID, e.g. "node1"
	BindAddr  string // TCP address for Raft transport, e.g. "127.0.0.1:7001"
	DataDir   string // directory for Raft logs and snapshots
	Bootstrap bool   // true only for the very first node bootstrapping the cluster
}

// Node wraps a Raft instance and its FSM.
type Node struct {
	Raft *raft.Raft
	FSM  *FSM
}

// HasExistingState returns true if this node already has Raft state on disk.
// Used to avoid re-joining a cluster on restart.
func (n *Node) HasExistingState() bool {
	last := n.Raft.LastIndex()
	return last > 0
}

// NewNode creates and starts a Raft node.
func NewNode(cfg Config) (*Node, error) {
	fsm := NewFSM()

	// --- Raft configuration ---
	rc := raft.DefaultConfig()
	rc.LocalID = raft.ServerID(cfg.NodeID)
	rc.HeartbeatTimeout = 500 * time.Millisecond
	rc.ElectionTimeout = 1000 * time.Millisecond
	rc.CommitTimeout = 50 * time.Millisecond
	rc.LeaderLeaseTimeout = 400 * time.Millisecond

	// --- Transport (TCP) ---
	addr, err := newTCPTransport(cfg.BindAddr)
	if err != nil {
		return nil, fmt.Errorf("transport: %w", err)
	}

	// --- Persistent log and stable store (BoltDB) ---
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir data dir: %w", err)
	}
	boltPath := filepath.Join(cfg.DataDir, "raft.db")
	boltStore, err := raftboltdb.NewBoltStore(boltPath)
	if err != nil {
		return nil, fmt.Errorf("boltdb: %w", err)
	}

	// --- Snapshot store ---
	snapStore, err := raft.NewFileSnapshotStore(cfg.DataDir, 3, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("snapshot store: %w", err)
	}

	// --- Create the Raft instance ---
	r, err := raft.NewRaft(rc, fsm, boltStore, boltStore, snapStore, addr)
	if err != nil {
		return nil, fmt.Errorf("new raft: %w", err)
	}

	// Bootstrap only if requested (first-time cluster formation)
	if cfg.Bootstrap {
		servers := raft.Configuration{
			Servers: []raft.Server{
				{ID: raft.ServerID(cfg.NodeID), Address: addr.LocalAddr()},
			},
		}
		r.BootstrapCluster(servers)
	}

	return &Node{Raft: r, FSM: fsm}, nil
}

// newTCPTransport creates a Raft TCP transport on the given address.
func newTCPTransport(addr string) (*raft.NetworkTransport, error) {
	return raft.NewTCPTransport(addr, nil, 3, 10*time.Second, os.Stderr)
}
