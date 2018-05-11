package main

import (
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

// The Proxy struct contains the main function to proxy client's request and configuration
type Proxy struct {
	// The ip address Proxy listen to
	IP   string
	Port string
	// If both CertPath and KeyPath is present, auto enable tls
	CertPath string
	KeyPath  string
	// Received from flag. format "user:password", multi values split with `;`
	authFlag string
	// Basic auth configuration.
	basicAuth map[string]string
}

func (p Proxy) auth(req *http.Request) bool {
	log.Println("Checking auth ...")
	log.Println(req.Header.Get("Proxy-Authorization"))
	basicAuthHeader := req.Header.Get("Proxy-Authorization")
	username, password, ok := ParseBasicAuth(basicAuthHeader)
	if len(p.basicAuth) == 0 {
		return true
	}
	if !ok {
		return false
	}
	for k, v := range p.basicAuth {
		if k == username && v == password {
			return true
		}
	}
	return false
}

func (p *Proxy) parseAuthConfig() {
	if p.authFlag == "" {
		return
	}
	cfg := strings.Split(p.authFlag, ";")
	var username, password string
	for _, c := range cfg {
		parsedResult := strings.Split(c, ":")
		if len(parsedResult) != 2 {
			panic("Auth config failed: " + c + " is invalid")
		}
		username, password = parsedResult[0], parsedResult[1]
		if p.basicAuth == nil {
			p.basicAuth = make(map[string]string)
		}
		p.basicAuth[username] = password
	}
}

func (p Proxy) tlsEnable() bool {
	return p.CertPath != "" && p.KeyPath != ""
}

// Address return the address(includes port) according the configuration in Proxy
func (p Proxy) Address() string {
	return p.IP + ":" + p.Port
}

// Run the proxy server
func (p *Proxy) Run() {
	p.parseAuthConfig()
	if p.tlsEnable() {
		log.Fatal(http.ListenAndServeTLS(p.Address(), p.CertPath, p.KeyPath, p))
		return
	}
	log.Fatal(http.ListenAndServe(p.Address(), p))
}

// Main method to listen and handle the incoming connection, includes http, https, ws
func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if ok := p.auth(req); !ok {
		w.WriteHeader(403)
		return
	}
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
	flag.StringVar(&proxy.CertPath, "cert", "", "the path of cert file used for tls")
	flag.StringVar(&proxy.KeyPath, "key", "", "the path of key file used for tls")
	flag.StringVar(&proxy.authFlag, "auth", "", "auth configuration for proxy. If not set, no auth is required. Argument format: user1:password;user2:password")
	flag.Parse()
	proxy.Run()
}
