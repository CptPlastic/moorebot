package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestLookupScoutIPv4Literal(t *testing.T) {
	got := lookupScoutIPv4("192.168.4.33")
	if len(got) != 1 || got[0] != "192.168.4.33" {
		t.Fatalf("unexpected addresses: %v", got)
	}
}

func TestROSMasterHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/RPC2" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	if !rosMasterHealthy(host, port) {
		t.Fatal("healthy XML-RPC server reported unavailable")
	}

	srv.Close()
	if rosMasterHealthy(host, port) {
		t.Fatal("closed XML-RPC server reported healthy")
	}
}
