package main

import (
	"fmt"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"sync"
	"sync/atomic"
)

const txnCap = 30

type txn struct {
	Payload string
	Source  string
	Id      uint64
}

type manager struct {
	snap     string
	wal      []txn
	txId     atomic.Uint64
	vclock   map[string]uint64
	lock     sync.RWMutex
	txnQueue chan txn

	clientId   int
	repClients map[int]chan txn
}

// newManager creates manager.
func newManager() *manager {
	m := &manager{
		snap:       "{}",
		wal:        []txn{},
		txId:       atomic.Uint64{},
		vclock:     map[string]uint64{},
		lock:       sync.RWMutex{},
		txnQueue:   make(chan txn, txnCap),
		repClients: map[int]chan txn{},
	}
	return m
}

// apply applies txn.
func (m *manager) apply(t txn) error {
	// If we have already seen this txn do nothing.
	if m.vclock[t.Source] >= t.Id {
		return nil
	}
	fmt.Println("New tx: ", t)
	patch, err := jsonpatch.DecodePatch([]byte(t.Payload))
	if err != nil {
		return err
	}
	applied, err := patch.Apply([]byte(m.snap))
	if err != nil {
		return err
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	// Patch snapshot.
	m.snap = string(applied)
	// Add entry to wal.
	m.wal = append(m.wal, t)
	// Update vclock.
	m.vclock[t.Source] = t.Id

	// Notify clients in non-block mode.
	go func() {
		for _, ch := range m.repClients {
			ch <- t
		}
	}()
	return nil
}

// run is manager routine.
func (m *manager) run() {
	for t := range m.txnQueue {
		if err := m.apply(t); err != nil {
			fmt.Println(err)
		}
	}
}

// getSnap returns current snapshot.
func (m *manager) getSnap() string {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.snap
}

// getVCLock returns current vclock.
func (m *manager) getVCLock() map[string]uint64 {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.vclock
}

// putTx puts txn to the queue.
func (m *manager) putTx(t txn) {
	m.txnQueue <- t
}

// addClient registers new replica client.
// It returns actual wal, client id, channel, the channel to which updates will be sent.
func (m *manager) addClient() ([]txn, int, chan txn) {
	m.lock.Lock()
	defer m.lock.Unlock()
	id := m.clientId
	m.clientId++
	m.repClients[id] = make(chan txn)
	fmt.Println("Got client: ", id)
	return m.wal, id, m.repClients[id]
}

// removeClient removes replica client.
func (m *manager) removeClient(id int) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.repClients, id)
}
