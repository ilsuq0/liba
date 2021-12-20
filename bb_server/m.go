package main

import (
	"flag"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/ilsuq0/liba"
	"github.com/ilsuq0/liba/bb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type chatServer struct {
	bb.UnimplementedBbServer

	nextId int64
}

func (s *chatServer) Chat(stream bb.Bb_ChatServer) error {
	return liba.SStreamDeal(atomic.AddInt64(&s.nextId, 1), stream)
}

var (
	tls        = flag.Bool("tls", true, "Connection uses TLS if true, else plain TCP")
	certFile   = flag.String("cert_file", "./server.crt", "The TLS cert file")
	keyFile    = flag.String("key_file", "./server.key", "The TLS key file")
	serverAddr = flag.String("server_addr", "", "The server listen addr")
	verbose    = flag.Bool("verbose", true, "verbose log")
	pprofAddr  = flag.String("pprof_addr", "", "pprof addr")
)

var kaep = keepalive.EnforcementPolicy{
	PermitWithoutStream: true, // Allow pings even when there are no active streams
}

var kasp = keepalive.ServerParameters{
	MaxConnectionAgeGrace: 5 * time.Second, // Allow 5 seconds for pending RPCs to complete before forcibly closing connections
	Time:                  5 * time.Second, // Ping the client if it is idle for 5 seconds to ensure the connection is still active
	Timeout:               1 * time.Second, // Wait 1 second for the ping ack before assuming the connection is dead
}

func main() {
	flag.Parse()
	liba.Verbose = *verbose
	go liba.StartPprof(*pprofAddr)

	lis, err := net.Listen("tcp", *serverAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts = []grpc.ServerOption{
		grpc.KeepaliveEnforcementPolicy(kaep), grpc.KeepaliveParams(kasp),
		grpc.InitialConnWindowSize(1024 * 1024 * 1024), grpc.InitialWindowSize(1024 * 1024 * 512),
		grpc.MaxConcurrentStreams(1024),
	}
	if *tls {
		if *certFile == "" || *keyFile == "" {
			log.Fatalf("cert=%s key=%s", *certFile, *keyFile)
		}
		creds, err := credentials.NewServerTLSFromFile(*certFile, *keyFile)
		if err != nil {
			log.Fatalf("Failed to generate credentials %v", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}
	grpcServer := grpc.NewServer(opts...)
	bb.RegisterBbServer(grpcServer, &chatServer{})

	grpcServer.Serve(lis)
}
