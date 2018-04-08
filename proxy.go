package main

import (
	"net"
	"log"
	"fmt"
	"bytes"
	"net/url"
	"strings"
	"io"
)

func main() {
	proxyServer, err := net.Listen("tcp", ":1081")
	if err != nil {
		log.Panic(err)
	}
	for {
		client, err := proxyServer.Accept()
		if err != nil {
			log.Println(err)
		}
		go handleProxy(client)
	}
}

func handleProxy(client net.Conn) {
	defer client.Close()
	// 获取tcp报文头部信息，tcp的body数据在1025字节之后
	var headerData [1024]byte
	n, err := client.Read(headerData[:])
	if err != nil {
		log.Println(err)
		return
	}
	var method, host, address string
	fmt.Sscanf(string(headerData[:bytes.IndexByte(headerData[:], '\n')]), "%s%s", &method, &host)
	urlInfo, _ := url.Parse(host)
	// 解析http请求的域名、端口信息
	if urlInfo.Opaque == "443" {
		address = urlInfo.Scheme + ":443"
	} else {
		if strings.Index(urlInfo.Scheme, ":") == -1 {
			address = urlInfo.Scheme + ":80"
		} else {
			address = urlInfo.Scheme
		}
	}

	proxyClient, err := net.Dial("tcp", address)
	if err != nil {
		log.Println(err)
		return
	}

	if method == "CONNECT" {
		// 连接代理时返回的信息
		fmt.Fprint(client, "HTTP/1.1 200 Connection established\r\n\r\n")
	} else {
		proxyClient.Write(headerData[:n])
	}
	go io.Copy(proxyClient, client)
	io.Copy(client, proxyClient)
}