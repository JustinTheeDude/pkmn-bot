package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// dropStore owns DropJob state. Previously handlers and the runner goroutine
// mutated the same *DropJob directly (e.g. job.Attempts = i) while API reads
// raced against it. All access now goes through these methods.
type dropStore struct {
	mu     sync.Mutex
	jobs   map[string]*DropJob
	cancel map[string]context.CancelFunc
}

func newDropStore() *dropStore {
	return &dropStore{jobs: map[string]*DropJob{}, cancel: map[string]context.CancelFunc{}}
}

func (s *dropStore) List() []*DropJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]*DropJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		list = append(list, j.clone())
	}
	// Stable order: soonest fire time first, newest created break ties.
	sort.Slice(list, func(i, j int) bool {
		if list[i].FireAt.Equal(list[j].FireAt) {
			return list[i].CreatedAt.After(list[j].CreatedAt)
		}
		return list[i].FireAt.Before(list[j].FireAt)
	})
	return list
}

func (s *dropStore) Get(id string) (*DropJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	return j.clone(), true
}

// mustGet is Get for callers that already confirmed existence; returns nil if
// the job vanished in between (callers treat nil as "gone").
func (s *dropStore) mustGet(id string) *DropJob {
	if j, ok := s.Get(id); ok {
		return j
	}
	return nil
}

// add registers a job and its cancel func.
func (s *dropStore) add(job *DropJob, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	s.cancel[job.ID] = cancel
}

// setState updates state + note atomically.
func (s *dropStore) setState(id, state, note string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.State = state
		j.Note = note
	}
}

func (s *dropStore) setAttempts(id string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Attempts = n
	}
}

func (s *dropStore) setCheckoutRun(id, runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.CheckoutRunID = runID
	}
}

// cancel stops a job's timer/loop. Reports whether a live cancel existed.
func (s *dropStore) cancelJob(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel, ok := s.cancel[id]; ok {
		cancel()
		delete(s.cancel, id)
		return true
	}
	return false
}

func (s *dropStore) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancel, id)
}

func (j *DropJob) clone() *DropJob {
	c := *j
	return &c
}

// dropJobID generates a unique, readable job id.
func dropJobID(now time.Time) string {
	return fmt.Sprintf("drop-%d", now.UnixNano())
}
