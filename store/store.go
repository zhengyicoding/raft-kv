package store

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"raft-kv/node"

	"github.com/hashicorp/raft"
)

const applyTimeout = 5 * time.Second

// Store is the public API for the distributed KV store on a single node.
type Store struct {
	n *node.Node
}

func New(n *node.Node) *Store {
	return &Store{n: n}
}

// Put applies a PUT command through Raft (must be called on the leader).
func (s *Store) Put(key, value string) error {
	return s.applyCmd(node.Command{Type: node.CmdPut, Key: key, Value: value})
}

// Delete applies a DELETE command through Raft (must be called on the leader).
func (s *Store) Delete(key string) error {
	return s.applyCmd(node.Command{Type: node.CmdDelete, Key: key})
}

// Get reads a key from the local FSM (stale/follower read).
// For a linearizable read, forward to the leader instead.
func (s *Store) Get(key string) (string, bool) {
	return s.n.FSM.Get(key)
}

// IsLeader reports whether this node is currently the Raft leader.
func (s *Store) IsLeader() bool {
	return s.n.Raft.State() == raft.Leader
}

// LeaderAddr returns the current leader's Raft address.
func (s *Store) LeaderAddr() string {
	addr, _ := s.n.Raft.LeaderWithID()
	return string(addr)
}

// AddVoter adds a new node to the cluster (call on leader only).
func (s *Store) AddVoter(id, addr string) error {
	f := s.n.Raft.AddVoter(
		raft.ServerID(id),
		raft.ServerAddress(addr),
		0, applyTimeout,
	)
	return f.Error()
}

// Barrier waits until the FSM has applied all committed log entries.
// Used for strong (linearizable) reads on the leader.
func (s *Store) Barrier() error {
	f := s.n.Raft.Barrier(applyTimeout)
	return f.Error()
}

// IsCommitIndexFresh returns true if the FSM's last applied index
// was updated within the given duration — used for bounded-staleness reads.
func (s *Store) IsCommitIndexFresh(maxAge time.Duration) bool {
	last := atomic.LoadInt64(&s.n.FSM.LastAppliedAt)
	return time.Since(time.Unix(0, last)) <= maxAge
}

func (s *Store) applyCmd(cmd node.Command) error {
	if !s.IsLeader() {
		return fmt.Errorf("not leader: forward to %s", s.LeaderAddr())
	}
	b, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	f := s.n.Raft.Apply(b, applyTimeout)
	return f.Error()
}
