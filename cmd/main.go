package main

import "github.com/xrlin/goproxy"
import "flag"

func main() {
	p := new(proxy.Proxy)
	flag.StringVar(&p.IP, "ip", "127.0.0.1", "the ip address p binding to")
	flag.StringVar(&p.Port, "port", "1081", "the port p binding to")
	flag.StringVar(&p.CertPath, "cert", "", "the path of cert file used for tls")
	flag.StringVar(&p.KeyPath, "key", "", "the path of key file used for tls")
	flag.StringVar(&p.AuthFlag, "auth", "", "auth configuration for p. If not set, no auth is required. Argument format: user1:password;user2:password")
	flag.Parse()
	p.Run()
}
