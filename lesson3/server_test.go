package main

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var testUrl = filepath.Join(os.TempDir(), "socket.sock")
var getUrl = "http://unix/get"
var replaceUrl = "http://unix/replace"

func assertResponse(t *testing.T, resp *http.Response, expected []byte) {
	defer resp.Body.Close()
	require.Equal(t, resp.StatusCode, http.StatusOK)
	actual, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestBasic(t *testing.T) {
	client := http.Client{
		Transport: &http.Transport{
			DialContext: func(context.Context, string, string) (net.Conn, error) {
				return net.Dial("unix", testUrl)
			},
		},
	}

	resp, err := client.Get(getUrl)
	require.NoError(t, err)
	assertResponse(t, resp, []byte{})

	resp, err = client.Post(replaceUrl, "application/octet-stream",
		bytes.NewReader([]byte{1, 2, 3}))
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)

	resp, err = client.Get(getUrl)
	require.NoError(t, err)
	assertResponse(t, resp, []byte{1, 2, 3})
}

func TestConcurrent(t *testing.T) {
	client := http.Client{
		Transport: &http.Transport{
			DialContext: func(context.Context, string, string) (net.Conn, error) {
				return net.Dial("unix", testUrl)
			},
		},
	}

	wg := sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		body := make([]byte, 100)
		body[i] = 1
		go func() {
			defer wg.Done()
			resp, err := client.Post(replaceUrl, "application/octet-stream",
				bytes.NewReader(body))
			require.NoError(t, err)
			require.Equal(t, resp.StatusCode, http.StatusOK)
		}()
	}
	wg.Wait()

	resp, err := client.Get(getUrl)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	oneCount := 0
	for i := 0; i < 100; i++ {
		if body[i] == 1 {
			oneCount++
		}
	}
	require.Equal(t, 1, oneCount)
}

func TestMain(m *testing.M) {
	listener, err := net.Listen("unix", testUrl)
	if err != nil {
		panic(err)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	done := make(chan struct{})
	defer func() {
		cancelFunc()
		<-done
		os.Remove(testUrl)
	}()

	if err = startServer(ctx, listener, done); err != nil {
		panic(err)
	}

	m.Run()
}
