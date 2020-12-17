package main

import (
	"os"
	"context"
	"time"
	"github.com/ANRGUSC/swarmdag/ledger"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	logging "github.com/op/go-logging"
	cfg "github.com/tendermint/tendermint/config"
	abciserver "github.com/tendermint/tendermint/abci/server"
	tmlog "github.com/tendermint/tendermint/libs/log"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
)

var log = logging.MustGetLogger("mock_abci")
var format = logging.MustStringFormatter(
    `%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
)

// 1) Install Tendermint to be run via command line
// 2) `tendermint node` (no flags needed)
// 2) Run this program
// 3) Send transactions via tx_gen.go
func main() {
    ctx := context.Background()
  	host, _ := libp2p.New(ctx)
    psub, _ := pubsub.NewGossipSub(ctx, host)
	dag := ledger.NewDAG(log, 100 * time.Millisecond, psub, host, ctx)
	app := ledger.NewABCIApp(dag, log, "mock_abci")
    server := abciserver.NewSocketServer("127.0.0.1:26658", app)
    logger := tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout))
    logger, _ = tmflags.ParseLogLevel(
    	"main:info,state:info,*:error",
    	logger,
    	cfg.DefaultLogLevel(),
    )
    server.SetLogger(logger)
    if err := server.Start(); err != nil {
       log.Fatalf("error starting socket server: %v\n", err)
    }

    select {}
}