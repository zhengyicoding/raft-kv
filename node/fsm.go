package node

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/raft"
)

// CommandType identifies the KV operation in a Raft log entry.
type CommandType string

const (
	CmdPut    CommandType = "PUT"
	CmdDelete CommandType = "DELETE"
)

// Command is the payload written into the Raft log.
type Command struct {
	Type  CommandType `json:"type"`
	Key   string      `json:"key"`
	Value string      `json:"value,omitempty"`
}

// FSM is the Raft finite state machine backed by an in-memory KV store.
type FSM struct {
	mu            sync.RWMutex
	data          map[string]string
	LastAppliedAt int64 // unix nanoseconds, updated atomically on every Apply
}

func NewFSM() *FSM {
	return &FSM{data: make(map[string]string)}
}

// Apply is called by Raft on every committed log entry.
// This is the ONLY place where state is mutated.
func (f *FSM) Apply(l *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(l.Data, &cmd); err != nil {
		return fmt.Errorf("unmarshal command: %w", err)
	}

	atomic.StoreInt64(&f.LastAppliedAt, time.Now().UnixNano())

	f.mu.Lock()
	defer f.mu.Unlock()

	switch cmd.Type {
	case CmdPut:
		f.data[cmd.Key] = cmd.Value
		return nil
	case CmdDelete:
		delete(f.data, cmd.Key)
		return nil
	default:
		return fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

// Get reads a value directly from the FSM (for follower/stale reads).
func (f *FSM) Get(key string) (string, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	v, ok := f.data[key]
	return v, ok
}

// Snapshot and Restore are required by the raft.FSM interface.
// We use a simple JSON snapshot of the entire map.
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	clone := make(map[string]string, len(f.data))
	for k, v := range f.data {
		clone[k] = v
	}
	return &fsmSnapshot{data: clone}, nil
}

func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	m := make(map[string]string)
	if err := json.NewDecoder(rc).Decode(&m); err != nil {
		return err
	}
	f.mu.Lock()
	f.data = m
	f.mu.Unlock()
	return nil
}

type fsmSnapshot struct {
	data map[string]string
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := json.NewEncoder(sink).Encode(s.data)
	if err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}
