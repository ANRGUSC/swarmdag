package app

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/cayley/schema"
	"github.com/cayleygraph/quad"

	abciserver "github.com/tendermint/tendermint/abci/server"
	"github.com/tendermint/tendermint/libs/log"

	. "github.com/ANRGUSC/swarmdag/pkg/cayley/cayleyapp"
)

var socketAddr string

func init() {
	flag.StringVar(&socketAddr, "socket-addr", "127.0.0.1:26658", "Unix domain socket address")
	schema.RegisterType("Transaction", Transaction{})
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

	// Create and add the Genesis Block
	// It will always have the same hash b/c timestamp is always 0
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 0)
	genesis := Transaction{
		Hash:      []byte{},
		PrevHash:  []byte{},
		Timestamp: b,
		Key:       []byte("Genesis"),
		Value:     []byte("Genesis"),
	}
	genesis.SetHash()
	app.DB.AddQuad(quad.Make(genesis, nil, nil, nil))
	LatestTransaction = (&genesis)
	genesis.Print()

	//Test start

	//Add two test blocks after the Genesis
	tx1 := NewTransaction([]byte("tx1"), []byte("test1"), genesis.Hash)
	tx2 := NewTransaction([]byte("tx2"), []byte("test2"), tx1.Hash)
	error := app.Insert(tx1, genesis)
	if error != nil {
		panic(error)
	}
	error = app.Insert(tx2, tx1)
	if error != nil {
		panic(error)
	}
	tx1.Print()
	tx2.Print()

	txs := app.ReturnAll()

	tx3 := NewTransaction([]byte("tx3"), []byte("test3"), tx2.Hash)
	txs = append(txs, tx3)

	SortbyHash(txs)
	PrintAll(txs)
	fmt.Printf("Total Hash: %x\n", SortAndHash(txs))

	json := ReturnJSON(txs)
	fmt.Println(json)
	app.InsertFromJSON([]byte(json))
	PrintAll(app.ReturnAll())

	// Test end

	flag.Parse()

	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

	server := abciserver.NewSocketServer(socketAddr, app)
	server.SetLogger(logger)
	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting socket server: %v\n", err)
		os.Exit(1)
	}
	defer server.Stop()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	os.Exit(0)
}
