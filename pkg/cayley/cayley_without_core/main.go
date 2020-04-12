package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/cayleygraph/cayley/schema"
	"github.com/cayleygraph/quad"

	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"

	abciserver "github.com/tendermint/tendermint/abci/server"
	"github.com/tendermint/tendermint/libs/log"
)

// Transaction struct
type Transaction struct {
	Hash      []byte `json:"hash" quad:"hash"`
	PrevHash  []byte `json:"prevHash" quad:"prevHash"`
	Timestamp int64  `json:"timestamp"  quad:"timestap"`
	Key       []byte `json:"key" quad:"key"`
	Value     []byte `json:"value" quad:"value"`
}

var socketAddr string

func init() {
	flag.StringVar(&socketAddr, "socket-addr", "unix://cayley.sock", "Unix domain socket address")
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

	//Create and add the Genesis Block
	genesis := Transaction{
		Hash:      []byte{},
		PrevHash:  []byte{},
		Timestamp: time.Now().Unix(),
		Key:       []byte("Genesis"),
		Value:     []byte("Genesis"),
	}
	genesis.setHash()
	app.db.AddQuad(quad.Make(genesis, nil, nil, nil))
	genesis.Print()

	//Add two test blocks after the Genesis
	tx1 := NewTransaction([]byte("tx1"), []byte("test1"), genesis.Hash)
	tx2 := NewTransaction([]byte("tx2"), []byte("test2"), tx1.Hash)
	error := insert(app.db, tx1, genesis)
	if error != nil {
		panic(error)
	}
	error = insert(app.db, tx2, tx1)
	if error != nil {
		panic(error)
	}
	tx1.Print()
	tx2.Print()

	app.ReturnAll()

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

func insert(db *cayley.Handle, tx interface{}, prevTx interface{}) error {
	/*
		qw := graph.NewWriter(db)
		defer qw.Close() // don't forget to close a writer; it has some internal buffering
		_, err := schema.WriteAsQuads(qw, tx)
		return err
	*/
	err := db.AddQuad(quad.Make(tx, "follows", prevTx, nil))
	return err
}

// NewTransaction creates a new Transaction struct
func NewTransaction(key []byte, value []byte, prevHash []byte) *Transaction {
	transaction := &Transaction{[]byte{}, prevHash, time.Now().Unix(), key, value}
	transaction.setHash()
	return transaction
}

func (t *Transaction) setHash() {
	timestamp := []byte(strconv.FormatInt(t.Timestamp, 10))
	headers := bytes.Join([][]byte{t.PrevHash, t.Key, t.Value, timestamp}, []byte{})
	hash := sha512.Sum512(headers)

	t.Hash = hash[:]
}

// Print print the every value of the transaction as a string
func (t *Transaction) Print() {
	fmt.Printf("Key: %s\n", t.Key)
	fmt.Printf("Value: %s\n", t.Value)
	fmt.Printf("Hash: %x\n", t.Hash)
	fmt.Printf("Prev. Hash: %x\n", t.PrevHash)
	fmt.Printf("Timestamp: %d\n", t.Timestamp)
	fmt.Println()
}

//SortAndHash sorts the array and returns the Hash
func SortAndHash(txs []Transaction) []byte {
	sort.Slice(txs, func(i, j int) bool {
		return string(txs[i].Hash) < string(txs[j].Hash)
	})

	var buffer bytes.Buffer
	for i := range txs {
		buffer.Write(txs[i].Hash)
	}
	hash := sha256.Sum256([]byte(buffer.String()))
	return hash[:]
}

func returnJSON(txs []Transaction) []byte {
	json, err := json.Marshal(txs)
	if err != nil {
		panic(err)
	}
	return json
}
