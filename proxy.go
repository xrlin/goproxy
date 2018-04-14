package main

import (
	"github.com/pkg/errors"
	"io"
	"log"
	"net"
	"net/http"
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
	p.handleHTTP(w, req)
}

func (p *proxy) getIP(req *http.Request) (ip string, err error) {
	ip, _, err = net.SplitHostPort(req.RemoteAddr)
	return
}

func (p proxy) addHeader(sourceHeader http.Header, header http.Header) {
	for key, value := range header {
		for _, v := range value {
			sourceHeader.Add(key, v)
		}
	}
}

// Handle normal http connection
func (p *proxy) handleHTTP(w http.ResponseWriter, req *http.Request) {
	transport := http.DefaultTransport
	newReq := new(http.Request)
	*newReq = *req
	if clientIP, err := p.getIP(req); err == nil {
		p.addHeader(newReq.Header, http.Header{"X-Forwarded-For": []string{clientIP}})
	}
	resp, err := transport.RoundTrip(newReq)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	p.addHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	resp.Body.Close()
}

func (p proxy) pipeConnection(dst *net.Conn, src *net.Conn, wg *sync.WaitGroup) {
	io.Copy(*dst, *src)
	wg.Done()
}

func (p proxy) convertToHijackConn(w http.ResponseWriter) (conn net.Conn, err error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		err = errors.New("Hijacking not supported")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// called before Hijack() called, otherwise will block the connection
	w.WriteHeader(http.StatusOK)
	conn, _, err = hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
	return
}

// Use tunnel to proxy https、ws and other proto..
func (p *proxy) handleTunnel(w http.ResponseWriter, req *http.Request) {
	conn, err := net.Dial("tcp", req.Host)
	if err != nil {
		w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()
	source, err := p.convertToHijackConn(w)
	if err != nil {
		return
	}
	defer source.Close()
	eof := sync.WaitGroup{}
	eof.Add(2)
	go p.pipeConnection(&conn, &source, &eof)
	go p.pipeConnection(&source, &conn, &eof)
	eof.Wait()
}

func main() {
	log.Fatal(http.ListenAndServe(":1081", &proxy{}))
}
