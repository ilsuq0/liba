package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ilsuq0/liba"
	
	"golang.org/x/sys/windows/registry"
)

func tcpLocal(addr string, getAddr func(net.Conn) (liba.Addr, error)) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
		return
	}
	for {
		c, err := l.Accept()
		if err != nil {
			log.Printf("failed to accept: %s", err)
			continue
		}
		go func() {
			defer c.Close()
			tgt, err := getAddr(c)
			if err != nil {
				// UDP: keep the connection until disconnect then free the UDP socket
				if err == liba.InfoUDPAssociate {
					buf := make([]byte, 1)
					// block here
					for {
						_, err := c.Read(buf)
						if err, ok := err.(net.Error); ok && err.Timeout() {
							continue
						}
						log.Printf("UDP Associate End.")
						return
					}
				}
				log.Printf("failed to get target address: %v", err)
				return
			}
			liba.CStreamDeal(c, tgt, csf.gens)
		}()
	}
}

var (
	tls                = flag.Bool("tls", true, "Connection uses TLS if true, else plain TCP")
	caFile             = flag.String("ca_file", "./ca.crt", "The file containing the CA root cert file")
	serverAddr         = flag.String("server_addr", "194.87.238.218:10892", "The server address in the format of host:port")
	serverHostOverride = flag.String("server_host_override", "194.87.238.218", "The server name used to verify the hostname returned by the TLS handshake")
	socksAddr          = flag.String("socks_addr", ":10818", "SOCKS listen address")
	pacAddr            = flag.String("pac_addr", ":10819", "pac server listen address")
	verbose            = flag.Bool("verbose", true, "verbose log")
	pprofAddr          = flag.String("pprof_addr", "", "pprof addr")
)

func main() {
	flag.Parse()
	liba.Verbose = *verbose
	go liba.StartPprof(*pprofAddr)

	csf.init(*tls, *caFile, *serverAddr, *serverHostOverride)

	go tcpLocal(*socksAddr, func(c net.Conn) (liba.Addr, error) { return liba.Handshake(c) })

	go pacLocal(*socksAddr, *pacAddr)

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-c
	pacSysWindows("")
}

//serve pac
func pacLocal(socksAddr, pacAddr string) {
	pacSysWindows("http://127.0.0.1" + pacAddr)

	var pac = []byte(
		`function FindProxyForURL(url, host) {return "SOCKS5 127.0.0.1` + socksAddr + `; direct"}`)
	var myHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Write(pac)
	}
	server := &http.Server{
		Addr:    pacAddr,
		Handler: myHandler,
	}
	server.ListenAndServe()
}

//windows proxy setting(PAC)
func pacSysWindows(pacURL string) {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		log.Fatal(err)
	}
	defer k.Close()

	err = k.SetStringValue("AutoConfigURL", pacURL)
	if err != nil {
		log.Fatal(err)
	}

	s, _, err := k.GetStringValue("AutoConfigURL")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Windows AutoConfigURL is %q\n", s)
}
