package node

import (
	"os"
	"net"
	"fmt"
	"context"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/multiformats/go-multiaddr"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/ANRGUSC/swarmdag/partition"
	"github.com/ANRGUSC/swarmdag/membership"
	"github.com/ANRGUSC/swarmdag/ledger"
	logging "github.com/op/go-logging"
)



func getIPAddr(log *logging.Logger) (addr string, addrEnd int) {
    //need a safer way to grab IP addr
    iface, err := net.InterfaceByName("eth0") // docker containers
    // iface, err := net.InterfaceByName("enp0s31f6") // CORE?

    if err != nil {
         log.Error(err)
         return "", 0
    }

    addrs, _ := iface.Addrs()
    for _, a := range addrs {
        if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
            if ipnet.IP.To4() != nil {
                addr := ipnet.IP.String()
                //for now determine node number (and listen port) by last ip
                //addr field, TODO: need scalable solution for this
                addrEnd = int(ipnet.IP.To4()[3])
                return addr, addrEnd
            }
        } else {
            log.Error("error finding interface addr")
            return "", 0
        }
    }

    return addr, addrEnd
}

type Config struct {
    Membership membership.Config
    ReconcileBcastInterval time.Duration
    Orchestrator string
}

type Node struct {
    dag   *ledger.DAG
    pmanager partition.Manager
    mmanager membership.Manager
}

func NewNode(cfg *Config, gossipPort int, keyfile string) *Node {
    var gossipPrivKey crypto.PrivKey
    var nodeID int
    log := logging.MustGetLogger("swarmdag")
    format := logging.MustStringFormatter(`%{level}:%{message}`)
    logging.SetBackend(logging.NewLogBackend(os.Stdout, "", 0))
    logging.SetLevel(logging.DEBUG, "swarmdag")
    logging.SetFormatter(format)
    gossipHost, addrEnd := getIPAddr(log)

    switch cfg.Orchestrator {
    case "core":
        nodeID = addrEnd - 1
    case "docker":
        nodeID = addrEnd - 2
    default:
        panic("NewNode(): invalid orchestrator")
    }

    k, err := getPrivKey(keyfile, nodeID)
    if err != nil {
        log.Debug("no libp2p key found, generating fresh key")
        gossipPrivKey, _, _ = crypto.GenerateKeyPair(crypto.Ed25519, -1)
    } else {
        pk, _ := crypto.ConfigDecodeKey(k)
        gossipPrivKey, _ = crypto.UnmarshalPrivateKey(pk)
    }

   	srcMultiAddr, _ := multiaddr.NewMultiaddr(
   		fmt.Sprintf("/ip4/%s/tcp/%d", gossipHost, gossipPort),
   	)

    ctx := context.Background()
  	host, _ := libp2p.New(
        ctx,
        libp2p.ListenAddrs(srcMultiAddr),
        libp2p.Identity(gossipPrivKey),
  	)

    psub, err := pubsub.NewGossipSub(ctx, host)

    log.Infof("\n[*] Your Multiaddress Is: /ip4/%s/tcp/%v/p2p/%s\n",
    		  gossipHost, gossipPort, host.ID().Pretty())

    libp2pIDs, err := readLibp2pIDs(keyfile)
    if err != nil {
        panic(err)
    }

    dag := ledger.NewDAG(
        log,
        cfg.ReconcileBcastInterval,
        psub,
        host,
        ctx,
    )
    pmanager := partition.NewManager(nodeID, log, dag, cfg.Orchestrator)
    mmanager := membership.NewManager(cfg.Membership, pmanager, ctx, host, psub, log,
                                      libp2pIDs)
    n := &Node{
        dag: dag,
        pmanager: pmanager,
        mmanager: mmanager,
    }
    n.mmanager.Start()

    return n
}