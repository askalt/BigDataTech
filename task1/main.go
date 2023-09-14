package main

import (
	"errors"
	"flag"
	"github.com/google/renameio"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

// replace replaces state to new bytes.
func replace(b []byte) error {
	// Atomically write to the disk to can raise the dead. 💀.
	if err := renameio.WriteFile(dbFilename, b, 0600); err != nil {
		return err
	}
	// Also keep data in RAM,
	// because we write extra-fast-super-1e9RPS database.
	// ⚡️ ⚡️ ⚡️.
	bytesMem = b
	return nil
}

// get returns current state.
func get() ([]byte, error) {
	return bytesMem, nil
}

// bootstrap initialize state from backup file.
func bootstrap() error {
	bytes, err := os.ReadFile(dbFilename)
	if errors.Is(err, os.ErrNotExist) {
		bytesMem = []byte{}
		return nil
	}
	if err != nil {
		return err
	}
	bytesMem = bytes
	return nil
}

var (
	port = flag.String("port", "0", "port to listen")
	mu   = sync.RWMutex{}

	// bytesMem describes current state.
	bytesMem []byte

	// dbFilename is backup filename.
	// Use `var` for testing purposes.
	// 💾.
	dbFilename = "db.state"
)

func handleReplace(w http.ResponseWriter, req *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := replace(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func handleGet(w http.ResponseWriter, req *http.Request) {
	mu.RLock()
	defer mu.RUnlock()
	res, err := get()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err = w.Write(res); err != nil {
		log.Println(err)
	}
}

func main() {
	flag.Parse()

	// Bootstrap.
	// 🐢 🐢 🐢.
	if err := bootstrap(); err != nil {
		log.Fatalf(err.Error())
	}

	// Set up handlers.
	// Serve any methods.
	http.HandleFunc("/replace", handleReplace)
	http.HandleFunc("/get", handleGet)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatalf(err.Error())
	}
}
