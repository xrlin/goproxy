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
	"testing"
)

var simpleProxy *Proxy
var tlsProxy *Proxy
var authTLSProxy *Proxy

const simpleProxyURL = "http://localhost:1081"
const tlsProxyURL = "https://localhost:1082"
const authTLSProxyURL = "https://user:password@localhost:1083"
const authFailedProxyURL = "https://user:failed@localhost:1083"

func init() {
	go startTLSProxy()
	go startProxy()
	go startAuthTLSProxy()
}

func startProxy() {
	if simpleProxy == nil {
		simpleProxy = &Proxy{IP: "127.0.0.1", Port: "1081"}
	}
	go simpleProxy.Run()
}

func startTLSProxy() {
	if tlsProxy != nil {
		return
	}
	tlsProxy = &Proxy{IP: "127.0.0.1", Port: "1082", CertPath: "localhost.crt", KeyPath: "localhost.key"}
	go tlsProxy.Run()
}

func startAuthTLSProxy() {
	if authTLSProxy != nil {
		return
	}
	authTLSProxy = &Proxy{IP: "127.0.0.1", Port: "1083", CertPath: "localhost.crt", KeyPath: "localhost.key", authFlag: "user:password"}
	go authTLSProxy.Run()
}

func getProxyClient() (*http.Client, error) {
	purl, err := url.Parse(simpleProxyURL)
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
	purl, err := url.Parse(tlsProxyURL)
	if err != nil {
		return nil, err
	}
	proxyFunc := http.ProxyURL(purl)
	tp := &transport{&http.Transport{Proxy: proxyFunc, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, nil}
	client := http.Client{Transport: tp}
	return &client, nil
}

func getAuthProxyClient() (*http.Client, error) {
	purl, err := url.Parse(authTLSProxyURL)
	if err != nil {
		return nil, err
	}
	proxyFunc := http.ProxyURL(purl)
	client := http.Client{Transport: &http.Transport{Proxy: proxyFunc, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	return &client, nil
}

// Return the client with proxy config that will be forbidden
func getAuthFailedProxyClient() (*http.Client, error) {
	purl, err := url.Parse(authFailedProxyURL)
	if err != nil {
		return nil, err
	}
	proxyFunc := http.ProxyURL(purl)
	client := http.Client{Transport: &http.Transport{Proxy: proxyFunc, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("response with %d\n", resp.StatusCode)
	}
}

// Test authorized TLS support with http request
func TestProxy_ServeHTTP5(t *testing.T) {
	client, _ := getAuthProxyClient()
	resp, err := client.Get("http://httpbin.org/headers?show_env=1")
	if err != nil {
		t.Fatal(err.Error())
	}
	log.Println(resp.StatusCode)
	defer resp.Body.Close()
	if !containsHeader(resp, "X-Forwarded-For", "127.0.0.1") {
		t.Fatal("no X-Forwarded-For found, Proxy failed")
	}
}

// Test authorized TLS support with https request
func TestProxy_ServeHTTP6(t *testing.T) {
	client, _ := getAuthProxyClient()

	resp, err := client.Get("https://httpbin.org/headers?show_env=1")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("response with %d\n", resp.StatusCode)
	}
}

// Test auth failed
func TestProxy_ServeHTTP7(t *testing.T) {
	client, _ := getAuthFailedProxyClient()
	resp, err := client.Get("http://httpbin.org/headers?show_env=1")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("response with %d, but 403 is expected\n", resp.StatusCode)
	}
}
