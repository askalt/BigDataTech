package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	// MaxWalSize is max entries count in one wal.
	// Thus, we simulate file restrictions.
	MaxWalSize = 30
	// SnapInterval is the time after which the snapshot is created.
	SnapInterval = time.Second * 30
	// PutTimeout is timeout after which put considered as fail.
	PutTimeout = time.Millisecond * 500
)

type wal struct {
	// index is an order number of wal.
	index   int
	entries []string
}

type snapshot struct {
	// index is as order number of snapshot.
	index int
	state string
}

// For example, we have such sausage:
// wal0 -> wal1 -> [snap2] -> wal3 -> wal4 -> [snap5] -> ...
// And when we take [snapN] we know which wal should be applied
// to take actual state.

type txn struct {
	// done is used to verify that txn was applied.
	done chan struct{}
	val  string
}

// getterSetter describes stored value.
// Use interface to test some scenarios.
type getterSetter interface {
	get() string
	set(string)
}

// defaultValue is the basic implementation of getterSetter.
type defaultValue string

func (v *defaultValue) get() string {
	return string(*v)
}

func (v *defaultValue) set(s string) {
	*v = defaultValue(s)
}

type manager struct {
	wals  []wal
	snaps []snapshot
	// actual is the current state.
	actual   getterSetter
	lock     sync.RWMutex
	txnQueue chan txn
}

func makeTicker() func() int {
	tick := 0
	return func() int {
		tick++
		return tick
	}
}

// run is a manager routine.
func (m *manager) run(ctx context.Context) error {
	tick := makeTicker()

	// Initialize first WAL.
	m.wals = append(m.wals, wal{
		index:   0,
		entries: []string{},
	})
	for {
		select {
		case txn := <-m.txnQueue:
			wLen := len(m.wals)
			// Check if size of the last WAL is too big.
			if len(m.wals[wLen-1].entries) == MaxWalSize {
				// Create new WAL.
				m.wals = append(m.wals, wal{
					index:   tick(),
					entries: []string{},
				})
			}
			// Append new entry to the last WAL.
			m.wals[wLen-1].entries = append(m.wals[wLen-1].entries, txn.val)
			// Update state.
			m.lock.Lock()
			m.actual.set(txn.val)
			m.lock.Unlock()
			// Notify about changes.
			close(txn.done)
		case <-time.After(SnapInterval):
			log.Println("Snapshot begin.")
			m.lock.RLock()
			// Append new snapshot entry.
			m.snaps = append(m.snaps, snapshot{
				index: tick(),
				state: m.actual.get(),
			})
			// Create new WAL.
			m.wals = append(m.wals, wal{
				index:   tick(),
				entries: []string{},
			})
			m.lock.RUnlock()
			log.Println("Snapshot is made")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// get returns current state.
func (m *manager) get() string {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.actual.get()
}

// put puts item to the txn queue.
// If txn queue is overloaded, returns an error.
func (m *manager) put(s string) error {
	done := make(chan struct{})
	t := txn{
		done: done,
		val:  s,
	}
	select {
	case m.txnQueue <- t:
		break
	case <-time.After(PutTimeout):
		return fmt.Errorf("put timeout")
	}
	<-done
	return nil
}
