package proxy

func Example() {
	// Simple proxy with http
	simpleProxy = &Proxy{IP: "127.0.0.1", Port: "1081"}
	simpleProxy.Run()

	// Simple proxy with tls support
	tlsProxy = &Proxy{IP: "127.0.0.1", Port: "1082", CertPath: "localhost.crt", KeyPath: "localhost.key"}
	tlsProxy.Run()

	// Simple proxy with auth
	authTLSProxy = &Proxy{IP: "127.0.0.1", Port: "1083", CertPath: "localhost.crt", KeyPath: "localhost.key", AuthFlag: "user:password"}

}
