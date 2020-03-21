package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/quad"

	abciserver "github.com/tendermint/tendermint/abci/server"
	"github.com/tendermint/tendermint/libs/log"
)

var socketAddr string

func init() {
	flag.StringVar(&socketAddr, "socket-addr", "unix://cayley.sock", "Unix domain socket address")
}

func main() {

	// File for your new BoltDB. Use path to regular file and not temporary in the real world
	tmpdir, err := ioutil.TempDir("", "cayleyFiles")
	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(tmpdir) // clean up

	// Initialize the database
	err = graph.InitQuadStore("bolt", tmpdir, nil)
	if err != nil {
		panic(err)
	}

	// Open and use the database
	db, err := cayley.NewGraph("bolt", tmpdir, nil)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	app := NewCayleyApplication(db)

	// Add some test data
	db.AddQuad(quad.Make("phraseoftheday", "isofcourse", "HelloBoltDB!", nil))
	db.AddQuad(quad.Make("phraseoftheday", "isofcourse", "secondHello", nil))
	db.AddQuad(quad.Make("secondhello", "pointsto", "Awesome", "alabel"))

	flag.Parse()

	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

	server := abciserver.NewSocketServer(socketAddr, app)
	server.SetLogger(logger)
	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting socket server: %v", err)
		os.Exit(1)
	}
	defer server.Stop()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	os.Exit(0)
}
