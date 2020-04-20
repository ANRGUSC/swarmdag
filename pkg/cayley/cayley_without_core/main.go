package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"sort"
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
	Timestamp []byte `json:"timestamp"  quad:"timestap"`
	Key       []byte `json:"key" quad:"key"`
	Value     []byte `json:"value" quad:"value"`
}

var socketAddr string

// IMPORTANT: Finding the latest transction on a restart of the node might be difficult
// We would need to find the longest chain and point to latest transaction of it
var latestTransaction *Transaction = nil

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

	// Create and add the Genesis Block
	// It will always have the same hash b/c it does not include a timestamp
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 0)
	genesis := Transaction{
		Hash:      []byte{},
		PrevHash:  []byte{},
		Timestamp: b,
		Key:       []byte("Genesis"),
		Value:     []byte("Genesis"),
	}
	genesis.setHash()
	app.db.AddQuad(quad.Make(genesis, nil, nil, nil))
	latestTransaction = (&genesis)
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

// Insert adds a new transction to the DAG
func (app *CayleyApplication) Insert(tx Transaction, prevTx Transaction) error {
	err := app.db.AddQuad(quad.Make(tx, "follows", prevTx, nil))
	latestTransaction = (&tx)
	return err
}

// NewTransaction creates a new Transaction struct
func NewTransaction(key []byte, value []byte, prevHash []byte) Transaction {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(time.Now().UnixNano()))
	transaction := Transaction{[]byte{}, prevHash, b, key, value}
	transaction.setHash()
	return transaction
}

//RestoreTransaction restores a transaction fetched from the DAG
func RestoreTransaction(hash []byte, prevHash []byte, timestamp []byte, key []byte, value []byte) Transaction {
	transaction := Transaction{hash, prevHash, timestamp, key, value}
	return transaction
}

func (t *Transaction) setHash() {
	//timestamp := []byte(strconv.FormatInt(t.Timestamp, 10))
	headers := bytes.Join([][]byte{t.PrevHash, t.Key, t.Value, t.Timestamp}, []byte{})
	hash := sha512.Sum512(headers)

	t.Hash = hash[:]
}

// Print print the every value of the transaction as a string
func (t Transaction) Print() {
	fmt.Printf("Key: %s\n", t.Key)
	fmt.Printf("Value: %s\n", t.Value)
	fmt.Printf("Hash: %x\n", t.Hash)
	//fmt.Println(t.Hash)
	fmt.Printf("Prev. Hash: %x\n", t.PrevHash)
	fmt.Printf("Timestamp: %d\n", t.Timestamp)
	fmt.Println()
}

// PrintAll prints all the transactions in the slice
func PrintAll(txs []Transaction) {
	for _, t := range txs {
		t.Print()
	}
}

// SortbyHash sorts the array based on the hash on the transactions
func SortbyHash(txs []Transaction) {
	sort.Slice(txs, func(i, j int) bool {
		//return string(txs[i].Hash) < string(txs[j].Hash)
		return fmt.Sprintf("%x", txs[i].Hash) < fmt.Sprintf("%x", txs[j].Hash)
	})
}

// SortbyDate sorts the array based on the date on the transactions (oldest first)
func SortbyDate(txs []Transaction) {
	sort.Slice(txs, func(i, j int) bool {
		return int64(binary.LittleEndian.Uint64(txs[i].Timestamp)) < int64(binary.LittleEndian.Uint64(txs[j].Timestamp))
	})
}

//SortAndHash sorts the array and returns the Hash
func SortAndHash(txs []Transaction) []byte {
	SortbyDate(txs)
	var buffer bytes.Buffer
	for i := range txs {
		buffer.Write(txs[i].Hash)
	}
	hash := sha256.Sum256(buffer.Bytes())
	return hash[:]
}

// ReturnJSON return the JSON representation of all sorted transactions
func ReturnJSON(txs []Transaction) string {
	SortbyDate(txs)
	json, err := json.Marshal(txs)
	if err != nil {
		panic(err)
	}
	return string(json)
}

// InsertFromJSON inserts new transaction in the JSON into the graph
func (app *CayleyApplication) InsertFromJSON(jsonInput []byte) {
	// convert the JSON to structs
	var txs []Transaction
	err := json.Unmarshal(jsonInput, &txs)
	if err != nil {
		panic(err)
	}
	SortbyDate(txs)

	// Use search function to see if transaction exist already
	// If not, insert into cayley by finding the previous tx
	for i := range txs {
		if app.Search(txs[i].Hash) == nil {

			// What if previous Hash does not exist
			// Find the previous Transaction
			prevTx := app.Search(txs[i].PrevHash)

			if prevTx == nil {
				panic("TODO: Could not insert b/c previous transaction is not in the database")
				// I think we can just insert the tranaction with prev. Tx as nil
				// Would create an unconnected side chain which will become connected later at some point
				// The graph iterator will still find this unconnected transaction
			}

			app.Insert(txs[i], (*prevTx))
			fmt.Println("Transaction inserted")
		} else {
			fmt.Println("Transaction exists")
		}
	}
}
