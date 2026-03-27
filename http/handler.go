package http

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"raft-kv/store"
)

// Server exposes the KV store over HTTP.
type Server struct {
	addr  string
	store *store.Store
}

func NewServer(addr string, s *store.Store) *Server {
	return &Server{addr: addr, store: s}
}
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/key/", s.handleKey)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/join", s.handleJoin)
	log.Printf("HTTP server listening on %s", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

// handleJoin adds a new node to the cluster.
// Called by joining nodes: GET /join?id=node2&addr=127.0.0.1:7002
func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	addr := r.URL.Query().Get("addr")
	if id == "" || addr == "" {
		http.Error(w, "missing id or addr", http.StatusBadRequest)
		return
	}
	if err := s.store.AddVoter(id, addr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleKey routes GET/PUT/DELETE /key/{key}
func (s *Server) handleKey(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/key/")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGet(w, r, key)
	case http.MethodPut:
		s.handlePut(w, r, key)
	case http.MethodDelete:
		s.handleDelete(w, r, key)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /key/{key}?level=strong|default|stale
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, key string) {
	level := r.URL.Query().Get("level")
	if level == "" {
		level = "default"
	}

	switch level {
	case "strong":
		// Forward to leader for linearizable read
		if !s.store.IsLeader() {
			s.forwardToLeader(w, r)
			return
		}
		// On the leader: apply a barrier to ensure FSM is up to date
		if err := s.store.Barrier(); err != nil {
			http.Error(w, fmt.Sprintf("barrier failed: %v", err), http.StatusInternalServerError)
			return
		}
		s.respondGet(w, key)

	case "default":
		// Serve locally only if commit index is recent (bounded staleness)
		if !s.store.IsCommitIndexFresh(2*time.Second) && !s.store.IsLeader() {
			s.forwardToLeader(w, r)
			return
		}
		s.respondGet(w, key)

	case "stale":
		// Always serve locally, no freshness check
		s.respondGet(w, key)

	default:
		http.Error(w, "invalid level: use strong, default, or stale", http.StatusBadRequest)
	}
}

// PUT /key/{key}  body: plain text value
func (s *Server) handlePut(w http.ResponseWriter, r *http.Request, key string) {
	if !s.store.IsLeader() {
		s.forwardToLeader(w, r)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	if err := s.store.Put(key, string(body)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /key/{key}
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, key string) {
	if !s.store.IsLeader() {
		s.forwardToLeader(w, r)
		return
	}
	if err := s.store.Delete(key); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /health — returns node role and leader address
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	role := "follower"
	if s.store.IsLeader() {
		role = "leader"
	}
	json.NewEncoder(w).Encode(map[string]string{
		"role":   role,
		"leader": s.store.LeaderAddr(),
	})
}

// respondGet writes the key lookup result as JSON.
func (s *Server) respondGet(w http.ResponseWriter, key string) {
	v, ok := s.store.Get(key)
	if !ok {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"key": key, "value": v})
}

// forwardToLeader proxies the request to the current Raft leader.
func (s *Server) forwardToLeader(w http.ResponseWriter, r *http.Request) {
	leaderRaftAddr := s.store.LeaderAddr()
	if leaderRaftAddr == "" {
		http.Error(w, "no leader available", http.StatusServiceUnavailable)
		return
	}
	// Derive HTTP address from Raft address (same host, HTTP port = Raft port + 10000)
	leaderHTTPAddr := raftAddrToHTTP(leaderRaftAddr)
	url := fmt.Sprintf("http://%s%s", leaderHTTPAddr, r.URL.RequestURI())

	req, err := http.NewRequest(r.Method, url, r.Body)
	if err != nil {
		http.Error(w, "failed to build forward request", http.StatusInternalServerError)
		return
	}
	req.Header = r.Header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("forward failed: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// raftAddrToHTTP converts "127.0.0.1:7001" → "127.0.0.1:17001"
func raftAddrToHTTP(raftAddr string) string {
	parts := strings.Split(raftAddr, ":")
	if len(parts) != 2 {
		return raftAddr
	}
	port := 0
	fmt.Sscanf(parts[1], "%d", &port)
	return fmt.Sprintf("%s:%d", parts[0], port+10000)
}
