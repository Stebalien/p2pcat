package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-kad-dht"
	dhtopt "github.com/libp2p/go-libp2p-kad-dht/opts"
	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	"github.com/multiformats/go-multiaddr"
)

// TODO: Put this in a different package.
var BootstrapAddresses = []string{
	"/dnsaddr/bootstrap.libp2p.io/ipfs/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd",
	"/ip4/104.131.131.82/tcp/4001/ipfs/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",            // mars.i.ipfs.io
	"/ip4/104.236.179.241/tcp/4001/ipfs/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",           // pluto.i.ipfs.io
	"/ip4/128.199.219.111/tcp/4001/ipfs/QmSoLSafTMBsPKadTEgaXctDQVcqN88CNLHXMkTNwMKPnu",           // saturn.i.ipfs.io
	"/ip4/104.236.76.40/tcp/4001/ipfs/QmSoLV4Bbm51jM9C4gDYZQ9Cy3U6aXMJDAbzgu2fzaDs64",             // venus.i.ipfs.io
	"/ip4/178.62.158.247/tcp/4001/ipfs/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd",            // earth.i.ipfs.io
	"/ip6/2604:a880:1:20::203:d001/tcp/4001/ipfs/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",  // pluto.i.ipfs.io
	"/ip6/2400:6180:0:d0::151:6001/tcp/4001/ipfs/QmSoLSafTMBsPKadTEgaXctDQVcqN88CNLHXMkTNwMKPnu",  // saturn.i.ipfs.io
	"/ip6/2604:a880:800:10::4a:5001/tcp/4001/ipfs/QmSoLV4Bbm51jM9C4gDYZQ9Cy3U6aXMJDAbzgu2fzaDs64", // venus.i.ipfs.io
	"/ip6/2a03:b0c0:0:1010::23:1001/tcp/4001/ipfs/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd", // earth.i.ipfs.io
}

var Info = log.New(ioutil.Discard, "I: ", 0)
var Error = log.New(os.Stderr, "E: ", 0)

const bufSize = 1 << 20

func main() {
	var flags flag.FlagSet
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: p2pcat [-v] [-routed] MULTIADDR PROTOCOL")
		fmt.Fprintln(flags.Output(), "  or:  p2pcat [-v] [-routed] -listen PROTOCOL")
		flags.PrintDefaults()
	}
	listener := flags.Bool("listen", false, "listen mode")
	routed := flags.Bool("routed", false, "enable peer routing")
	verbose := flags.Bool("v", false, "print status messages")
	switch err := flags.Parse(os.Args[1:]); err {
	case nil:
	case flag.ErrHelp:
		os.Exit(0)
	default:
		os.Exit(2)
	}

	if (!*listener && flags.NArg() != 2) || (*listener && flags.NArg() != 1) {
		fmt.Fprintln(flags.Output(), "unexpected number of arguments")
		flags.Usage()
		os.Exit(2)
	}

	if *verbose {
		Info.SetOutput(os.Stderr)
	}

	if *listener {
		if err := listen(*routed, flags.Arg(0)); err != nil {
			Error.Fatal(err)
		}
	} else {
		if err := connect(*routed, flags.Arg(0), flags.Arg(1)); err != nil {
			Error.Fatal(err)
		}
	}
}

func listen(routed bool, protocolstring string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proto := protocol.ID(protocolstring)

	node, err := libp2p.New()
	if err != nil {
		return err
	}
	defer func() {
		node.Close()
	}()
	if routed {
		dhtclient, err := dht.New(ctx, node, dhtopt.Client(true))
		if err != nil {
			return err
		}
		node = rhost.Wrap(node, dhtclient)
		err = bootstrap(ctx, node)
		if err != nil {
			return err
		}

		go dhtclient.Bootstrap(ctx)
	}

	var once sync.Once

	var wg sync.WaitGroup
	wg.Add(1)

	Error.Printf("listening on: /ipfs/%s %s", node.ID().Pretty(), protocolstring)

	node.SetStreamHandler(proto, func(stream network.Stream) {
		once.Do(func() {
			defer wg.Done()

			node.RemoveStreamHandler(proto)
			Info.Printf("connection from: /ipfs/%s", stream.Conn().RemotePeer().Pretty())

			pipe(stream)
		})
	})
	wg.Wait()

	return nil
}

func connect(routed bool, addrstring string, protocolstring string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	target, err := multiaddr.NewMultiaddr(addrstring)
	if err != nil {
		return err
	}

	proto := protocol.ID(protocolstring)

	ainfo, err := peer.AddrInfoFromP2pAddr(target)
	if err != nil {
		return err
	}

	node, err := libp2p.New(libp2p.NoListenAddrs)
	if err != nil {
		return err
	}
	defer func() {
		node.Close()
	}()
	if routed {
		dhtclient, err := dht.New(ctx, node, dhtopt.Client(true))
		if err != nil {
			return err
		}
		node = rhost.Wrap(node, dhtclient)
		err = bootstrap(ctx, node)
		if err != nil {
			return err
		}
	}

	Info.Printf("connecting to: %s", target)
	node.Peerstore().AddAddrs(ainfo.ID, ainfo.Addrs, peerstore.TempAddrTTL)
	s, err := node.NewStream(ctx, ainfo.ID, proto)
	if err != nil {
		return err
	}
	Info.Printf("connected to: %s/ipfs/%s", s.Conn().RemoteMultiaddr(), s.Conn().RemotePeer().Pretty())

	pipe(s)
	return nil
}

func pipe(s network.Stream) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := io.CopyBuffer(s, os.Stdin, make([]byte, bufSize)); err != nil {
			Error.Print(err)
		} else if err := s.Close(); err != nil {
			s.Reset()
			Error.Print(err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := io.CopyBuffer(os.Stdout, s, make([]byte, bufSize)); err != nil {
			Error.Print(err)
		} else if err := os.Stdout.Close(); err != nil {
			s.Reset()
		}
	}()
	wg.Wait()
}

func bootstrap(ctx context.Context, node host.Host) error {
	Info.Printf("bootstrapping")

	bctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, len(BootstrapAddresses))
	for _, addrs := range BootstrapAddresses {
		ai, err := peer.AddrInfoFromP2pAddr(multiaddr.StringCast(addrs))
		if err != nil {
			panic(err)
		}
		go func() {
			done <- node.Connect(bctx, *ai)
		}()
	}
	for range BootstrapAddresses {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			if err == nil {
				return nil
			}
			Info.Printf("bootstrap error: %s", err)
		}
	}
	return fmt.Errorf("failed to booststrap")
}
