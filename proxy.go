package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

type proxy struct {
}

// Main method to listen and handle the incoming connection, includes http, https, ws
func (p *proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodConnect {
		p.handleTunnel(w, req)
		return
	}
	p.handleHttp(w, req)
}

// Handle normal http connection
func (p *proxy) handleHttp(w http.ResponseWriter, req *http.Request) {
	transport := http.DefaultTransport
	newReq := new(http.Request)
	*newReq = *req
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		if prior, ok := newReq.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		newReq.Header.Set("X-Forwarded-For", clientIP)
	}
	resp, err := transport.RoundTrip(newReq)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	for key, value := range resp.Header {
		for _, v := range value {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	resp.Body.Close()
}

// Use tunnel to proxy https、ws and other proto..
func (p *proxy) handleTunnel(w http.ResponseWriter, req *http.Request) {
	conn, err := net.Dial("tcp", req.Host)
	eof := sync.WaitGroup{}
	if err != nil {
		w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	// called before Hijack() called, otherwise will block the connection
	w.WriteHeader(http.StatusOK)
	source, _, err := hj.Hijack()
	defer source.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	eof.Add(2)
	go func() {
		io.Copy(conn, source)
		eof.Done()
	}()
	go func() {
		io.Copy(source, conn)
		eof.Done()
	}()
	eof.Wait()
}

func main() {
	log.Fatal(http.ListenAndServe(":1081", &proxy{}))
}
