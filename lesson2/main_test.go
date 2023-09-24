package main

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func getTestingServer() *httptest.Server {
	testRouter := http.NewServeMux()
	testRouter.HandleFunc("/replace", handleReplace)
	testRouter.HandleFunc("/get", handleGet)
	return httptest.NewServer(testRouter)
}

func assertResponse(t *testing.T, resp *http.Response, expected []byte) {
	defer resp.Body.Close()
	require.Equal(t, resp.StatusCode, http.StatusOK)
	actual, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestBasic(t *testing.T) {
	server := getTestingServer()
	defer server.Close()

	resp, err := http.Get(server.URL + "/get")
	require.NoError(t, err)
	assertResponse(t, resp, []byte{})

	resp, err = http.Post(server.URL+"/replace", "application/octet-stream",
		bytes.NewReader([]byte{1, 2, 3}))
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)

	resp, err = http.Get(server.URL + "/get")
	require.NoError(t, err)
	assertResponse(t, resp, []byte{1, 2, 3})
}

func TestConcurrent(t *testing.T) {
	server := getTestingServer()
	defer server.Close()

	wg := sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		body := make([]byte, 100)
		body[i] = 1
		go func() {
			defer wg.Done()
			resp, err := http.Post(server.URL+"/replace", "application/octet-stream",
				bytes.NewReader(body))
			require.NoError(t, err)
			require.Equal(t, resp.StatusCode, http.StatusOK)
		}()
	}
	wg.Wait()

	resp, err := http.Get(server.URL + "/get")
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

func TestDead(t *testing.T) {
	server := getTestingServer()
	resp, err := http.Post(server.URL+"/replace", "application/octet-stream",
		bytes.NewReader([]byte{228}))
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)
	server.Close()

	_, err = http.Get(server.URL + "/get")
	require.Error(t, err)

	newServer := getTestingServer()
	defer newServer.Close()
	resp, err = http.Get(newServer.URL + "/get")
	require.NoError(t, err)
	assertResponse(t, resp, []byte{228})
}

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "test_server")
	if err != nil {
		panic(err)
	}
	oldDbFilename := dbFilename
	defer func() {
		dbFilename = oldDbFilename
	}()
	dbFilename = filepath.Join(tmpDir, "test_db.state")
	os.Exit(m.Run())
}
