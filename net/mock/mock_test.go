package mocknet

import (
	"bytes"
	"io"
	"math/rand"
	"sync"
	"testing"

	inet "github.com/jbenet/go-ipfs/net"
	peer "github.com/jbenet/go-ipfs/peer"
	testutil "github.com/jbenet/go-ipfs/util/testutil"

	context "github.com/jbenet/go-ipfs/Godeps/_workspace/src/code.google.com/p/go.net/context"
)

// func TestNetworkSetup(t *testing.T) {

// 	p1 := testutil.RandPeer()
// 	p2 := testutil.RandPeer()
// 	p3 := testutil.RandPeer()
// 	peers := []peer.Peer{p1, p2, p3}

// 	nets, err := MakeNetworks(context.Background(), peers)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// check things

// 	if len(nets) != 3 {
// 		t.Error("nets must be 3")
// 	}

// 	for i, n := range nets {
// 		if n.local != peers[i] {
// 			t.Error("peer mismatch")
// 		}

// 		if len(n.conns) != len(nets) {
// 			t.Error("conn mismatch")
// 		}

// 		for _, c := range n.conns {
// 			if c.remote.conns[n.local] == nil {
// 				t.Error("conn other side fail")
// 			}
// 			if c.remote.conns[n.local].remote.local != n.local {
// 				t.Error("conn other side fail")
// 			}
// 		}

// 	}

// }

func TestStreams(t *testing.T) {

	mn, err := FullMeshConnected(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}

	handler := func(s inet.Stream) {
		go func() {
			b := make([]byte, 4)
			if _, err := io.ReadFull(s, b); err != nil {
				panic(err)
			}
			if !bytes.Equal(b, []byte("beep")) {
				panic("bytes mismatch")
			}
			if _, err := s.Write([]byte("boop")); err != nil {
				panic(err)
			}
			s.Close()
		}()
	}

	nets := mn.Nets()
	for _, n := range nets {
		n.SetHandler(inet.ProtocolDHT, handler)
	}

	s, err := nets[0].NewStream(inet.ProtocolDHT, nets[1].LocalPeer())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Write([]byte("beep")); err != nil {
		panic(err)
	}
	b := make([]byte, 4)
	if _, err := io.ReadFull(s, b); err != nil {
		panic(err)
	}
	if !bytes.Equal(b, []byte("boop")) {
		panic("bytes mismatch 2")
	}

}

func makePinger(st string, n int) func(inet.Stream) {
	return func(s inet.Stream) {
		go func() {
			defer s.Close()

			for i := 0; i < n; i++ {
				b := make([]byte, 4+len(st))
				if _, err := s.Write([]byte("ping" + st)); err != nil {
					panic(err)
				}
				if _, err := io.ReadFull(s, b); err != nil {
					panic(err)
				}
				if !bytes.Equal(b, []byte("pong"+st)) {
					panic("bytes mismatch")
				}
			}
		}()
	}
}

func makePonger(st string) func(inet.Stream) {
	return func(s inet.Stream) {
		go func() {
			defer s.Close()

			for {
				b := make([]byte, 4+len(st))
				if _, err := io.ReadFull(s, b); err != nil {
					if err == io.EOF {
						return
					}
					panic(err)
				}
				if !bytes.Equal(b, []byte("ping"+st)) {
					panic("bytes mismatch")
				}
				if _, err := s.Write([]byte("pong" + st)); err != nil {
					panic(err)
				}
			}
		}()
	}
}

func TestStreamsStress(t *testing.T) {

	mn, err := FullMeshConnected(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}

	protos := []inet.ProtocolID{
		inet.ProtocolDHT,
		inet.ProtocolBitswap,
		inet.ProtocolDiag,
	}

	nets := mn.Nets()
	for _, n := range nets {
		for _, p := range protos {
			n.SetHandler(p, makePonger(string(p)))
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from := rand.Intn(len(nets))
			to := rand.Intn(len(nets))
			p := rand.Intn(3)
			proto := protos[p]
			// log.Debug("%d (%s) %d (%s) %d (%s)", from, nets[from], to, nets[to], p, protos[p])
			s, err := nets[from].NewStream(protos[p], nets[to].LocalPeer())
			if err != nil {
				panic(err)
			}

			log.Infof("%d start pinging", i)
			makePinger(string(proto), rand.Intn(100))(s)
			log.Infof("%d done pinging", i)
		}(i)
	}

	wg.Done()
}

func TestAdding(t *testing.T) {

	mn := New(context.Background())

	p1 := testutil.RandPeer()
	p2 := testutil.RandPeer()
	p3 := testutil.RandPeer()
	peers := []peer.Peer{p1, p2, p3}

	for _, p := range peers {
		if _, err := mn.AddPeer(p.ID()); err != nil {
			t.Error(err)
		}
	}

	// link them
	for _, p1 := range peers {
		for _, p2 := range peers {
			if _, err := mn.LinkPeers(p1, p2); err != nil {
				t.Error(err)
			}
		}
	}

	// set the new stream handler on p2
	n2 := mn.Net(p2.ID())
	if n2 == nil {
		t.Fatalf("no network for %s", p2.ID())
	}
	n2.SetHandler(inet.ProtocolBitswap, func(s inet.Stream) {
		go func() {
			defer s.Close()

			b := make([]byte, 4)
			if _, err := io.ReadFull(s, b); err != nil {
				panic(err)
			}
			if string(b) != "beep" {
				panic("did not beep!")
			}

			if _, err := s.Write([]byte("boop")); err != nil {
				panic(err)
			}
		}()
	})

	// connect p1 to p2
	if err := mn.ConnectPeers(p1, p2); err != nil {
		t.Fatal(err)
	}

	// talk to p2
	n1 := mn.Net(p1.ID())
	if n1 == nil {
		t.Fatalf("no network for %s", p1.ID())
	}

	s, err := n1.NewStream(inet.ProtocolBitswap, p2)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Write([]byte("beep")); err != nil {
		t.Error(err)
	}
	b := make([]byte, 4)
	if _, err := io.ReadFull(s, b); err != nil {
		t.Error(err)
	}
	if !bytes.Equal(b, []byte("boop")) {
		t.Error("bytes mismatch 2")
	}

}
