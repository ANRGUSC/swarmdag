package main

import (
	"os"
	"os/signal"
	"syscall"
    "flag"
    "fmt"

	logging "github.com/op/go-logging"
	"github.com/ANRGUSC/swarmdag/partition"
    "github.com/ANRGUSC/swarmdag/membership"
	// tmrand "github.com/tendermint/tendermint/libs/rand"
    "github.com/ANRGUSC/swarmdag/ledger"
)

var log = logging.MustGetLogger("swarmdag")
var nodeID int

func init() {
    flag.IntVar(&nodeID, "node-id", 0, "Node ID")
}

func main() {
    flag.Parse()
    // TODO: removeme
    if nodeID == 0 {
        membership.SetLeader()
    }
    // TODO: need to synchronize the config file copying and stuff

    // Create a node interface/struct. Then, initialize all the 
    viewID := 0
    membershipID := "jSDFgS"

    ledger := ledger.NewLedger()
    // initiate partition manager
    pm := partition.NewManager(nodeID, log, ledger)
    pm.NewNetwork(viewID, membershipID)

    // start tendermint spawn service with gochannel input
    // upon request, new tmcore and abci apps are spawned


    // start mms. 

    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    <-c
    os.Exit(0)
}

// p2p port
// 26656

// each group has a unique mid as well as a non-decreasing view ID

// take the modulus of the membership counter to determine the port to use in the
// membership.