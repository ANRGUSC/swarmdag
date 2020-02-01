// +build go1.11

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"context"
	"crypto/rand"
	"flag"
	"time"
	"net"

	// tmos "github.com/tendermint/tendermint/libs/os"
	// "github.com/tendermint/tendermint/libs/log"

	// abcicli "github.com/tendermint/tendermint/abci/client"
	// "github.com/tendermint/tendermint/abci/example/code

	"github.com/tendermint/tendermint/abci/server"
	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/abci/example/kvstore"

	"github.com/libp2p/go-libp2p"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/multiformats/go-multiaddr"
	logging "github.com/op/go-logging"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/hashicorp/memberlist"
)

var log = logging.MustGetLogger("swarmdag")

func getIPAddr() (addr string, addrEnd int) {
    //need a safer way to grab IP addr
    iface, err := net.InterfaceByName("eth0")

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

var format = logging.MustStringFormatter(
    `%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
)

func main() {
    // For demo purposes, create two backend for os.Stderr.
    f, err := os.OpenFile("swarmdag.log", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
    defer f.Close()

    backend1 := logging.NewLogBackend(os.Stderr, "", 0)
    backend2 := logging.NewLogBackend(f, "", 0)
    // backend2 := logging.NewLogBackend(w, "", 0)

    // For messages written to backend2 we want to add some additional
    // information to the output, including the used log level and the name of
    // the function.
    backend2Formatter := logging.NewBackendFormatter(backend2, format)

    // Only errors and more severe messages should be sent to backend1
    backend1Leveled := logging.AddModuleLevel(backend1)
    backend1Leveled.SetLevel(logging.ERROR, "")

    // Set the backends to be used.
    logging.SetBackend(backend1Leveled, backend2Formatter)

    log.Debugf("debug (%s)", "test prints of the diff levels of logging")
    log.Info("info")
    log.Notice("notice")
    log.Warning("warning")
    log.Error("err")
    log.Critical("crit")

    help := flag.Bool("help", false, "Display Help")

    //TODO auto populate all config
    cfg := parseFlags()

    listenHost, addrEnd := getIPAddr()

    listenPort := 9000 + addrEnd 

    if *help {
        fmt.Printf("Simple example for peer discovery using mDNS. mDNS is great when you have multiple peers in local LAN.")
        fmt.Printf("Usage: \n   Run './chat-with-mdns'\nor Run './chat-with-mdns -host [host] -port [port] -rendezvous [string] -pid [proto ID]'\n")

        os.Exit(0)
    }

    log.Infof("[*] Listening on: %s with port: %d\n", listenHost, listenPort)

    ctx := context.Background()
    r := rand.Reader

    // Creates a new RSA key pair for this host.
    prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
    if err != nil {
        panic(err)
    }

    // 0.0.0.0 will listen on any interface device. (default)
    sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", listenHost, listenPort))


    // libp2p.New constructs a new libp2p Host.
    // Other options can be added here.
    host, _ := libp2p.New(
        ctx,
        libp2p.ListenAddrs(sourceMultiAddr),
        libp2p.Identity(prvKey),
    )


    // if err != nil {
    //     panic(err)
    // }

    psub, err := pubsub.NewGossipSub(ctx, host)

    if err != nil {
        panic(err)
    }

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

    log.Infof("\n[*] Your Multiaddress Is: /ip4/%s/tcp/%v/p2p/%s\n", listenHost, listenPort, host.ID().Pretty())

    config := &Config{
        nodeID: addrEnd - 1,
        membershipBroadcastPeriod: 200 * time.Millisecond,
        proposeHeartbeatInterval: 1,
        proposeTimerMin: 3, //todo
        proposeTimerMax: 5,
        peerTimeout: 1,
        leaderTimeout: 10,
        followerTimeout: 200, //msec
        majorityRatio: 0.51,
    }

    mm := NewMembershipManager(config, ctx, host, psub, log)

    //need to SetConnHandler() before initMDNS()
    host.Network().SetConnHandler(mm.connectHandler)
    peerChan := initMDNS(ctx, host, cfg.RendezvousString, 5 * time.Second)

    mm.OnStart(peerChan)

    c := memberlist.DefaultLANConfig()
    c.ProbeInterval = 1000 * time.Millisecond
    c.ProbeTimeout = 5000 * time.Millisecond
    c.GossipInterval = 1000 * time.Millisecond
    c.PushPullInterval = 5000 * time.Millisecond
    c.SuspicionMult = 10
    c.BindAddr = listenHost
    c.BindPort = 6000 + addrEnd
  
    //testing MMS 
    select {} 

    m, err := memberlist.Create(c)
    if err != nil {
        fmt.Printf("unexpected err: %s", err)
    }
    defer m.Shutdown()

    //when using blockade, docker will assign IP addrs starting at x.x.x.2
    //and that node will be the bootstrapping node for initiating the first 
    //partition using memberlist. need to adapt this for CORE. the bootstrapping
    //node does not need to Join() a cluster.
    if addrEnd > 2 {
        for {
            //Known address of bootstrapper node
            _, err := m.Join([]string{"172.17.0.2:6002"})
            if err != nil {
                fmt.Printf("unexpected err: %s", err)
            } else {
                break
            }

            time.Sleep(1)
        }
    }

    // Ask for members of the cluster
    // for {
    //     for _, member := range m.Members() {
    //         log.Debugf("Member: %s %s\n", member.Name, member.Addr)
    //     }

    //     time.Sleep(time.Second * 5)
    // }

    //TODO: remove to reenable Tendermint code below
    select {} //wait here


    // logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

    dir, err := ioutil.TempDir("/tmp", "kvstore_db")
    if err != nil {
        log.Error(err)
    }

    app := kvstore.NewPersistentKVStoreApplication(dir)
    // app.(*kvstore.PersistentKVStoreApplication).SetLogger(logger.With("module", "kvstore"))

    srv, err := server.NewServer("localhost:26658", "socket", app) 
    if err != nil {
        log.Error(err)
    }

    // srv.SetLogger(logger.With("module", "abci-server")) 
    if err := srv.Start(); err != nil {
        log.Error(err)
    }

    key := "abc"
    // value := "def"
    // tx := []byte(key + "=" + value)

    // app.DeliverTx(tx)

    // make sure query is fine
    resQuery := app.Query(types.RequestQuery{
        Path: "/store",
        Data: []byte(key),
    })
    // assert.Equal(code.CodeTypeOK, resQuery.Code)
    fmt.Println(resQuery.Code)
    // assert.Equal(value, string(resQuery.Value))
    fmt.Println(resQuery.Value)

    // make sure proof is fine
    resQuery = app.Query(types.RequestQuery{
        Path:  "/store",
        Data:  []byte(key),
        Prove: true,
    })
    // assert.EqualValues(code.CodeTypeOK, resQuery.Code)
    fmt.Println(resQuery.Code)
    // assert.Equal(value, string(resQuery.Value))
    fmt.Println(resQuery.Value)

    // Wait forever
    select {}
    // tmos.TrapSignal(log, func() {
    //     // Cleanup
    //     srv.Stop()
    // })
}
