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
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	dhtopt "github.com/libp2p/go-libp2p-kad-dht/opts"
	"github.com/libp2p/go-libp2p-peerstore"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	"github.com/libp2p/go-libp2p-protocol"
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

func main() {
	if err := connect(); err != nil {
		Error.Fatal(err)
	}
}

func connect() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var flags flag.FlagSet
	routed := flags.Bool("routed", false, "enable peer routing")
	verbose := flags.Bool("v", false, "print status messages")
	switch err := flags.Parse(os.Args[1:]); err {
	case nil:
	case flag.ErrHelp:
		return nil
	default:
		os.Exit(2)
	}

	if flags.NArg() != 2 {
		return fmt.Errorf("wrong number of arguments: expected <address> <protocol>")
	}

	if *verbose {
		Info.SetOutput(os.Stderr)
	}

	target, err := multiaddr.NewMultiaddr(flags.Arg(0))
	if err != nil {
		return err
	}

	proto := protocol.ID(flags.Arg(1))

	pinfo, err := peerstore.InfoFromP2pAddr(target)
	if err != nil {
		return err
	}

	node, err := libp2p.New(ctx, libp2p.NoListenAddrs)
	if err != nil {
		return err
	}
	defer func() {
		node.Close()
	}()
	if *routed {
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
	node.Peerstore().AddAddrs(pinfo.ID, pinfo.Addrs, peerstore.TempAddrTTL)
	s, err := node.NewStream(ctx, pinfo.ID, proto)
	if err != nil {
		return err
	}
	Info.Printf("connected to: %s/ipfs/%s", s.Conn().RemoteMultiaddr(), s.Conn().RemotePeer().Pretty())

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := io.Copy(s, os.Stdin); err != nil {
			Error.Print(err)
		} else if err := s.Close(); err != nil {
			s.Reset()
			Error.Print(err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := io.Copy(os.Stdout, s); err != nil {
			Error.Print(err)
		} else if err := os.Stdout.Close(); err != nil {
			s.Reset()
		}
	}()
	wg.Wait()
	return nil
}

func bootstrap(ctx context.Context, node host.Host) error {
	Info.Printf("bootstrapping")

	bctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, len(BootstrapAddresses))
	for _, addrs := range BootstrapAddresses {
		pi, err := pstore.InfoFromP2pAddr(multiaddr.StringCast(addrs))
		if err != nil {
			panic(err)
		}
		go func() {
			done <- node.Connect(bctx, *pi)
		}()
	}
	for _ = range BootstrapAddresses {
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
