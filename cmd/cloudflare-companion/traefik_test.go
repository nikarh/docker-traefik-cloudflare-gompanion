package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchTraefikRoutersInvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer ts.Close()

	routers, status, body, err := FetchTraefikRouters(context.Background(), ts.URL, false, "")
	require.Error(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "not-json", body)
	require.Nil(t, routers)
}

func TestFetchTraefikRoutersErrorPage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	}))
	defer ts.Close()

	routers, status, body, err := FetchTraefikRouters(context.Background(), ts.URL, false, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusBadGateway, status)
	require.Equal(t, "<html>bad gateway</html>", body)
	require.Nil(t, routers)
}
