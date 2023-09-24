package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func makeHandleReplace(m *manager) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := m.put(string(body)); err != nil {
			// Timeout.
			w.WriteHeader(http.StatusGatewayTimeout)
		}
	}
}

func makeHandleGet(m *manager) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		res := m.get()
		if _, err := w.Write([]byte(res)); err != nil {
			log.Println(err)
		}
	}
}

func setSigHandler(cancelFunc context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range sigCh {
			cancelFunc()
		}
	}()
}

func startServer(ctx context.Context, listener net.Listener, done chan struct{}) error {
	m := manager{
		wals:     []wal{},
		snaps:    []snapshot{},
		actual:   new(defaultValue),
		lock:     sync.RWMutex{},
		txnQueue: make(chan txn),
	}
	go m.run(ctx)

	serverMux := http.NewServeMux()
	serverMux.HandleFunc("/replace", makeHandleReplace(&m))
	serverMux.HandleFunc("/get", makeHandleGet(&m))

	server := http.Server{
		Handler: serverMux,
	}

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Println(err.Error())
		}
	}()

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
		done <- struct{}{}
	}()

	return nil
}

func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())
	setSigHandler(cancelFunc)
	listener, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}
	done := make(chan struct{})
	if err := startServer(ctx, listener, done); err != nil {
		panic(err)
	}
	<-done
}
