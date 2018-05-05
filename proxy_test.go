package main

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"testing"
)

var testServer *http.Server
var tlsProxy *Proxy

func init() {
	go startTLSProxy()
	go startProxy()
}

func startProxy() {
	if testServer == nil {
		testServer = &http.Server{Addr: "127.0.0.1:1081", Handler: &Proxy{}}
	}
	testServer.ListenAndServe()
}

var mutex sync.Mutex

func startTLSProxy() {
	mutex.Lock()
	if tlsProxy != nil {
		return
	}
	tlsProxy := &Proxy{IP: "127.0.0.1", Port: "1082", CertPath: "localhost.crt", KeyPath: "localhost.key"}
	go tlsProxy.Run()
	mutex.Unlock()
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

// transport is an http.RoundTripper that keeps track of the in-flight
// request and implements hooks to report HTTP tracing events.
type transport struct {
	*http.Transport
	current *http.Request
}

// RoundTrip wraps http.DefaultTransport.RoundTrip to keep track
// of the current request.
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.current = req
	return t.Transport.RoundTrip(req)
}

// GotConn prints whether the connection has been used previously
// for the current request.
func (t *transport) GotConn(info httptrace.GotConnInfo) {
	log.Printf("Connection reused for %v? %v\n", t.current.URL, info.Reused)
}

func (t *transport) ConnectStart(network, addr string) {
	log.Printf("Connect start with %s, %s\n", network, addr)
}

func getTLSProxyClient() (*http.Client, error) {
	purl, err := url.Parse("https://localhost:1082")
	if err != nil {
		return nil, err
	}
	proxyFunc := http.ProxyURL(purl)
	tp := &transport{&http.Transport{Proxy: proxyFunc, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, nil}
	client := http.Client{Transport: tp}
	return &client, nil
}

type hs struct {
	Headers map[string]string `json:"headers"`
}

// header definition in hs
func containsHeader(response *http.Response, header string, value string) bool {
	b := make([]byte, response.ContentLength)
	io.ReadFull(response.Body, b)
	log.Println(string(b))
	var h hs
	json.Unmarshal(b, &h)
	return strings.Contains(h.Headers[header], value)
}

// Test http Proxy
func TestProxy_ServeHTTP(t *testing.T) {
	client, _ := getProxyClient()
	resp, err := client.Get("http://httpbin.org/headers?show_env=1")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !containsHeader(resp, "X-Forwarded-For", "127.0.0.1") {
		t.Fatal("no X-Forwarded-For found, Proxy failed")
	}
}

// Test https Proxy
func TestProxy_ServeHTTP2(t *testing.T) {
	client, _ := getProxyClient()
	resp, err := client.Get("https://httpbin.org/headers?show_env=1")
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("response with %d\n", resp.StatusCode)
	}
}

// Test TLS support with http request
func TestProxy_ServeHTTP3(t *testing.T) {
	client, _ := getTLSProxyClient()
	resp, err := client.Get("http://httpbin.org/headers?show_env=1")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !containsHeader(resp, "X-Forwarded-For", "127.0.0.1") {
		t.Fatal("no X-Forwarded-For found, Proxy failed")
	}
}

// Test TLS support with https request
func TestProxy_ServeHTTP4(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://httpbin.org/headers?show_env=1", nil)
	client, _ := getTLSProxyClient()
	trace := &httptrace.ClientTrace{
		GotConn:      client.Transport.(*transport).GotConn,
		ConnectStart: client.Transport.(*transport).ConnectStart,
		ConnectDone: func(network, addr string, err error) {
			log.Printf("Connect done with network %s, addr %s\n", network, addr)
			if err != nil {
				log.Printf("Connect with error %s\n", err.Error())
			}
		},
		TLSHandshakeStart: func() {
			log.Println("Tls handshake start")
		},
		TLSHandshakeDone: func(state tls.ConnectionState, e error) {
			log.Printf("Tls connect with version: %d, serverName: %s\n", state.Version, state.ServerName)
			if e != nil {
				log.Printf("Tls handshake with error %s\n", e.Error())
			}
		},
	}
	//resp, err := client.Get("https://httpbin.org/headers?show_env=1")
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("response with %d\n", resp.StatusCode)
	}
}
