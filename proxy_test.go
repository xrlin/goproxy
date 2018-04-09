package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

var testServer *http.Server

func startProxy() {
	if testServer == nil {
		testServer = &http.Server{Addr: "127.0.0.1:1081", Handler: &proxy{}}
	}
	testServer.ListenAndServe()
}

func getProxyClient() (*http.Client, error) {
	purl, err := url.Parse("http://127.0.0.1:1081")
	if err != nil {
		return nil, err
	}
	proxyFunc := http.ProxyURL(purl)
	transport := &http.Transport{Proxy: proxyFunc}
	client := http.Client{Transport: transport}
	return &client, nil
}

type hs struct {
	Headers map[string]string `json:"headers"`
}

// header definition in hs
func containsHeader(response *http.Response, header string, value string) bool {
	b := make([]byte, response.ContentLength)
	io.ReadFull(response.Body, b)
	var h hs
	json.Unmarshal(b, &h)
	return strings.Contains(h.Headers[header], value)
}

// Test http proxy
func TestProxy_ServeHTTP(t *testing.T) {
	go startProxy()
	//defer stopProxy()
	client, _ := getProxyClient()
	resp, err := client.Get("http://httpbin.org/headers?show_env=1")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !containsHeader(resp, "X-Forwarded-For", "127.0.0.1") {
		t.Fatal("no X-Forwarded-For found, proxy failed")
	}
}

// Test https proxy
func TestProxy_ServeHTTP2(t *testing.T) {
	go startProxy()
	client, _ := getProxyClient()
	resp, err := client.Get("https://httpbin.org/headers?show_env=1")
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("response with %d\n", resp.StatusCode)
	}
}
