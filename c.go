package liba

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/ilsuq0/liba/bb"

	"net/http"
	_ "net/http/pprof"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	CMD_CONNECT = iota
	CMD_CONNECTED
	CMD_DATA
	CMD_DONE
)

type Stream interface {
	Send(*bb.Snap) error
	Recv() (*bb.Snap, error)
}

type wrapc struct {
	id int64
	c  net.Conn

	nr  int
	fly []byte
	er  error //read
	ew  error //write

	snap *bb.Snap
}

func (wc *wrapc) rest() {
	if wc.c != nil {
		wc.c.Close()
	}
	wc.id = 0
	wc.c = nil
	wc.er = nil
	wc.ew = nil
	wc.nr = 0
	wc.snap.Id = 0
	wc.snap.Cmd = CMD_DATA
}

var pool sync.Pool = sync.Pool{
	New: func() interface{} {
		return &wrapc{fly: make([]byte, 32*1024), snap: &bb.Snap{Cmd: CMD_DATA}}
	},
}

//socket -> stream
func socket2stream(wc *wrapc, s Stream, d string, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		wc.nr, wc.er = wc.c.Read(wc.fly)
		if wc.nr > 0 {
			wc.snap.Dta = wc.fly[:wc.nr]
			if err := s.Send(wc.snap); err != nil {
				logf("%d %s [%d] %v", wc.id, d, wc.nr, err)
				return
			}
			// log.Printf("%d <~ [%d]", wc.id, wc.nr)
			wc.nr = 0
		}
		if wc.er != nil {
			if wc.er == io.EOF || errors.Is(wc.er, os.ErrDeadlineExceeded) {
				wc.er = nil
			}
			break
		}
	}
	//send DONE
	wc.snap.Cmd = CMD_DONE
	s.Send(wc.snap)

	logf("- %d - done %v", wc.id, wc.er)
}

//stream -> socket
func stream2socket(wc *wrapc, s Stream, d string, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		snap, err := s.Recv()
		if err != nil {
			logf("%d %s %v", wc.id, d, err)
			return
		}
		switch snap.Cmd {
		case CMD_DONE:
			logf("%d %s done", snap.Id, d)
			wc.c.SetReadDeadline(time.Now().Add(1400 * time.Millisecond))
			return
		case CMD_DATA:
			if _, wc.ew = wc.c.Write(snap.Dta); wc.ew != nil {
				logf("%d %s done %v", snap.Id, d, wc.ew)
				return
			}
			// log.Printf("%v ~> [%d]", snap.Id, n)
		default:
			logf("default: %v", snap)
			return
		}
	}
}

var errCmd error = status.New(codes.InvalidArgument, "unexpected cmd").Err()

func sconnect(wc *wrapc, s Stream) error {
	snap, err := s.Recv()
	if err != nil {
		logf("~ ~> %v", err)
		return err
	}
	if snap.Cmd != CMD_CONNECT {
		logf("sconnect: id=%d cmd=%d", snap.Id, snap.Cmd)
		return errCmd
	}
	rc, err := net.Dial("tcp", SplitAddr(snap.Dta).String())
	if err != nil {
		logf("failed to connect to target: %v", err)
		return err
	}
	wc.snap.Id = wc.id
	wc.c = rc
	snap.Id = wc.id
	snap.Cmd = CMD_CONNECTED
	err = s.Send(snap)

	logf("~ ~ %d %v", snap.Id, err)
	return err
}

//server stream deal
func SStreamDeal(id int64, s bb.Bb_ChatServer) (err error) {
	wc := pool.Get().(*wrapc)
	defer pool.Put(wc)
	defer wc.rest()

	wc.id = id
	if err = sconnect(wc, s); err != nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go socket2stream(wc, s, "<~", &wg)
	go stream2socket(wc, s, "~>", &wg)

	wg.Wait()
	return
}

func cconnect(wc *wrapc, s Stream, tgt []byte) (err error) {
	wc.snap.Cmd = CMD_CONNECT
	wc.snap.Dta = tgt
	if err = s.Send(wc.snap); err != nil {
		logf("~ ~> %v dta=%v", err, wc.snap.Dta)
		return
	}
	snap, err := s.Recv()
	if err != nil {
		logf("<~ ~ %v", err)
		return
	}
	if snap.Cmd != CMD_CONNECTED || snap.Id == 0 {
		logf("Connect <~ id=%d cmd=%v dta=%v", snap.Id, snap.Cmd, snap.Dta)
		return errCmd
	}
	wc.id = snap.Id
	wc.snap.Id = snap.Id
	wc.snap.Cmd = CMD_DATA
	logf("~ ~ %d", snap.Id)
	return
}

//client stream deal
func CStreamDeal(c net.Conn, tgt []byte, gens func(ctx context.Context) (Stream, error)) (err error) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	s, err := gens(ctx)
	if err != nil {
		logf("fail to chat proxy server: %v", err)
		return
	}

	wc := pool.Get().(*wrapc)
	defer pool.Put(wc)
	wc.c = c
	defer wc.rest()

	if err = cconnect(wc, s, tgt); err != nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go socket2stream(wc, s, "~>", &wg)
	go stream2socket(wc, s, "<~", &wg)

	wg.Wait()

	return
}

//pprof analysis
func StartPprof(pprofAddr string) {
	if pprofAddr != "" {
		http.ListenAndServe(pprofAddr, nil)
	}
}
