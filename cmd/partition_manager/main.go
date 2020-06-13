package main

import (
    "os"
    "os/signal"
    "syscall"

    logging "github.com/op/go-logging"
    "github.com/ANRGUSC/swarmdag/pkg/partition"
)

var log = logging.MustGetLogger("swarmdag")

func main() {

    // initiate ledger service

    // initiate partition manager
    go partition.StartPartitionManager(log)

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