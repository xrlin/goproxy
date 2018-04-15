package main

import (
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
)

// The Proxy struct contains the main function to proxy client's request and configuration
type Proxy struct {
	// The ip address Proxy listen to
	IP   string
	Port string
}

// Address return the address(includes port) according the configuration in Proxy
func (p Proxy) Address() string {
	return p.IP + ":" + p.Port
}

// Run the proxy server
func (p *Proxy) Run() {
	log.Fatal(http.ListenAndServe(p.Address(), p))
}

// Main method to listen and handle the incoming connection, includes http, https, ws
func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodConnect {
		p.handleTunnel(w, req)
		return
	}
	p.handleHTTP(w, req)
}

func (p *Proxy) getIP(req *http.Request) (ip string, err error) {
	ip, _, err = net.SplitHostPort(req.RemoteAddr)
	return
}

func (p Proxy) addHeader(sourceHeader http.Header, header http.Header) {
	for key, value := range header {
		for _, v := range value {
			sourceHeader.Add(key, v)
		}
	}
}

// Handle normal http connection
func (p *Proxy) handleHTTP(w http.ResponseWriter, req *http.Request) {
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

func (p Proxy) pipeConnection(dst *net.Conn, src *net.Conn, wg *sync.WaitGroup) {
	io.Copy(*dst, *src)
	wg.Done()
}

func (p Proxy) convertToHijackConn(w http.ResponseWriter) (conn net.Conn, err error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		err = errors.New("hijacking not supported")
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

// Use tunnel to Proxy https、ws and other proto..
func (p *Proxy) handleTunnel(w http.ResponseWriter, req *http.Request) {
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
	proxy := new(Proxy)
	flag.StringVar(&proxy.IP, "ip", "127.0.0.1", "the ip address proxy binding to")
	flag.StringVar(&proxy.Port, "port", "1081", "the port proxy binding to")
	flag.Parse()
	proxy.Run()
}
