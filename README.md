A simple netcat-like program for libp2p.

Example usage with [protocat](https://github.com/Stebalien/protocat):

```bash
> export PATH="$GOPATH/bin:$PATH"       # make sure your GOPATH is in your PATH
> go get github.com/Stebalien/{p2pcat,protocat} github.com/libp2p/go-libp2p/p2p/protocol/identify/pb
> MADDR=/ipfs/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ
> PROTOBUF=github.com/libp2p/go-libp2p/p2p/protocol/identify/pb/identify.Identify
> p2pcat -v -routed $MADDR /ipfs/id/1.0.0 <&-| protocat -l -d $PROTOBUF
```
