package node

import (
    "net"
    "os"
    "fmt"
    "context"
    "crypto/rand"
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

var log = logging.MustGetLogger("swarmdag")
var format = logging.MustStringFormatter(
    `%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
)

func getIPAddr() (addr string, addrEnd int) {
    //need a safer way to grab IP addr
    // iface, err := net.InterfaceByName("eth0")
    iface, err := net.InterfaceByName("enp0s31f6")

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


type Node struct {
    dag   *ledger.DAG
    pmanager partition.Manager
    mmanager membership.Manager
}

func NewNode(gossipPort int) *Node {
    f, err := os.OpenFile("swarmdag.log", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
    backend1 := logging.NewLogBackend(f, "", 0)
    backend2 := logging.NewLogBackend(os.Stdout, "", 0)

    b1 := logging.AddModuleLevel(backend1)
    b2 := logging.AddModuleLevel(backend2)

    b1.SetLevel(logging.DEBUG, "")
    b2.SetLevel(logging.DEBUG, "")

    logging.SetBackend(b1, b2)

    r := rand.Reader
    gossipHost, addrEnd := getIPAddr()
    gossipPrivKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
    if err != nil {
		panic(err)
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
    // subChan2, _ := psub.Subscribe("next_membership")

    // //gossip msg printer routine
    // go func(sub *pubsub.Subscription) {
    //     for {
    //         got, _ := sub.Next(ctx)
    //         log.Info(string(got.Data))
    //     }
    // }(subChan2)

    // mpChan, _ := psub.Subscribe("membership_propose")

    // //gossip msg printer routine
    // go func(sub *pubsub.Subscription) {
    //     for {
    //         got, _ := sub.Next(ctx)
    //         log.Info("proposal: ", string(got.Data))
    //     }
    // }(mpChan)

    log.Infof("\n[*] Your Multiaddress Is: /ip4/%s/tcp/%v/p2p/%s\n", 
    		  gossipHost, gossipPort, host.ID().Pretty())

    nodeID := addrEnd - 1

    cfg := &membership.Config{
        NodeID: nodeID,
        BroadcastPeriod: 200 * time.Millisecond,
        ProposeHeartbeatInterval: 1,
        ProposeTimerMin: 3, //todo
        ProposeTimerMax: 5,
        PeerTimeout: 1,
        LeaderTimeout: 10,
        FollowerTimeout: 200, //msec
        MajorityRatio: 0.51,
    }

    dag := ledger.NewDAG(log, host, psub)
    pmanager := partition.NewManager(nodeID, log, dag)
    mmanager := membership.NewManager(cfg, ctx, host, psub, log)
    n := &Node{
        dag: dag,
        pmanager: pmanager,
        mmanager: mmanager,
    }

    // SetConnHandler() required before initMDNS()
    n.mmanager.OnStart()

    n.pmanager.NewNetwork(0, "aaaaaa")
    return n
}