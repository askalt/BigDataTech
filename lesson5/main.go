package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"os"
	"time"
)

var (
	//go:embed index.html
	index []byte
)

var port = flag.Int("port", 0, "port to use")
var peersFile = flag.String("peers", "", "peers file")
var source = flag.String("source", "Skalt", "source name")

func main() {
	flag.Parse()
	peersRaw, err := os.ReadFile(*peersFile)
	if err != nil {
		panic(err)
	}
	var peers []string
	if err := yaml.Unmarshal(peersRaw, &peers); err != nil {
		panic(err)
	}

	m := newManager()
	go m.run()

	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		w.Write(index)
	})

	http.HandleFunc("/vclock", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		w.Write([]byte(fmt.Sprintln(m.getVCLock())))
	})

	http.HandleFunc("/replace", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		txId := m.txId.Add(1)
		t := txn{
			Payload: string(body),
			Source:  *source,
			Id:      txId,
		}
		m.putTx(t)
	})

	http.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		w.Write([]byte(m.getSnap()))
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("NEW WS QUERY")
		defer r.Body.Close()
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
			OriginPatterns:     []string{"*"},
		})
		if err != nil {
			fmt.Println(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		wal, id, ch := m.addClient()
		defer func() {
			m.removeClient(id)
			// Release notify goroutines.
			for range ch {
			}
		}()
		for _, tx := range wal {
			if err := wsjson.Write(r.Context(), c, tx); err != nil {
				return
			}
		}
		for tx := range ch {
			if err := wsjson.Write(r.Context(), c, tx); err != nil {
				return
			}
		}
	})

	for _, peer := range peers {
		peer := peer
		go func() {
			for {
				ctx := context.Background()
				c, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://%s/ws", peer), nil)
				if err != nil {
					fmt.Println(err)
					<-time.After(time.Second * 5)
					continue
				}
				for {
					var t txn
					if err := wsjson.Read(ctx, c, &t); err != nil {
						fmt.Println(err)
						break
					}
					m.putTx(t)
				}
			}
		}()
	}

	if err := http.ListenAndServe(fmt.Sprintf("localhost:%d", *port), nil); err != nil {
		panic(err)
	}
}
