package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/ilsuq0/liba"
	"github.com/ilsuq0/liba/bb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

var csf csfactory

type csfactory struct {
	serverAddr string
	opts       []grpc.DialOption

	mu     sync.RWMutex
	count  int
	client bb.BbClient
}

var kacp = keepalive.ClientParameters{
	Time:                10 * time.Second, // send pings every 10 seconds if there is no activity
	Timeout:             time.Second,      // wait 1 second for ping ack before considering the connection dead
	PermitWithoutStream: true,             // send pings even without active streams
}

func (csf *csfactory) init(tls bool, caFile, serverAddr, serverHostOverride string) {
	if tls {
		if caFile == "" {
			log.Fatalf("caFile=%s", caFile)
		}
		creds, err := credentials.NewClientTLSFromFile(caFile, serverHostOverride)
		if err != nil {
			log.Fatalf("Failed to create TLS credentials %v", err)
		}
		csf.opts = append(csf.opts, grpc.WithTransportCredentials(creds))
	} else {
		csf.opts = append(csf.opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	csf.opts = append(csf.opts, grpc.WithBlock(), grpc.WithKeepaliveParams(kacp))
	csf.serverAddr = serverAddr

	conn, err := grpc.Dial(csf.serverAddr, csf.opts...)
	if err != nil {
		log.Printf("fail to dial proxy server: %v", err)
	} else {
		csf.client = bb.NewBbClient(conn)
	}
}

func (csf *csfactory) gens(ctx context.Context) (cs liba.Stream, err error) {
	csf.mu.RLock()
	c := csf.count
	if csf.client != nil {
		cs, err = csf.client.Chat(ctx)
	}
	csf.mu.RUnlock()
	if err != nil || csf.client == nil {
		//retry once
		csf.mu.Lock()
		if c == csf.count {
			conn, errc := grpc.Dial(csf.serverAddr, csf.opts...)
			if errc != nil {
				csf.mu.Unlock()
				return nil, errc
			}
			csf.client = bb.NewBbClient(conn)
			csf.count++
		}
		csf.mu.Unlock()
		cs, err = csf.client.Chat(ctx)
	}
	return
}
