package cayleyapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	abcitypes "github.com/tendermint/tendermint/abci/types"

	"github.com/cayleygraph/cayley"
	_ "github.com/cayleygraph/cayley/graph/kv/bolt"
	"github.com/cayleygraph/quad"
)

// CayleyApplication stores db and current struct
type CayleyApplication struct {
	DB           *cayley.Handle
	currentBatch quad.Quad
}

// NewCayleyApplication creates new Instance
func NewCayleyApplication(db *cayley.Handle) *CayleyApplication {
	return &CayleyApplication{
		DB: db,
	}
}

// Transaction struct
type Transaction struct {
	Hash      []byte `json:"hash" quad:"hash"`
	PrevHash  []byte `json:"prevHash" quad:"prevHash"`
	Timestamp []byte `json:"timestamp"  quad:"timestap"`
	Key       []byte `json:"key" quad:"key"`
	Value     []byte `json:"value" quad:"value"`
}

// LatestTransaction Important: Finding the latest transction on a restart of the node might be difficult
// if there are multiple chains. We probably want to append to the longest chain.
// We would need to find the longest chain and point to latest transaction of it
var LatestTransaction *Transaction = nil

var _ abcitypes.Application = (*CayleyApplication)(nil)
var transactionsEnabled = true

// Info Tendermint ABCI
func (app *CayleyApplication) Info(req abcitypes.RequestInfo) abcitypes.ResponseInfo {
	return abcitypes.ResponseInfo{}
}

// SetOption Tendermint ABCI
func (app *CayleyApplication) SetOption(req abcitypes.RequestSetOption) abcitypes.ResponseSetOption {
	return abcitypes.ResponseSetOption{}
}

// DeliverTx Tendermint ABCI
func (app *CayleyApplication) DeliverTx(req abcitypes.RequestDeliverTx) abcitypes.ResponseDeliverTx {

	/*
		Transaction can also be disabled at this point, before they get delivered
		Disable new incoming transactions while tendermint is running by returning
		a non-zero status code in the ResponseCheckTx struct"
	*/
	if transactionsEnabled == false {
		fmt.Println("Incoming transactions are currently disabled")
		return abcitypes.ResponseDeliverTx{Code: 23}
	}

	request := req.Tx
	requestString := string(request)

	// Detect the JSON format using "[{"" prefix and "}]" suffix
	// Otherwise, create a new key, value transaction

	if strings.HasPrefix(requestString, "[{") && strings.HasSuffix(requestString, "}]") {
		// Is JSON
		fmt.Println("JSON received")
		// Tendermint replaces + with a space
		replacedString := strings.Replace(requestString, " ", "+", -1)
		app.InsertFromJSON([]byte(replacedString))
	} else {
		code := app.isValid(req.Tx)
		if code != 0 {
			return abcitypes.ResponseDeliverTx{Code: 1}
		}
		// <key>=<value>
		parts := bytes.Split(request, []byte("="))
		key, value := parts[0][1:len(parts[0])-1], parts[1][1:len(parts[1])-1]
		fmt.Println("New Transaction with " + string(key) + " " + string(value))
		if (len(key) == 0) || (len(value) == 0) {
			// Failed
			return abcitypes.ResponseDeliverTx{Code: 1}
		}
		newTx := NewTransaction(key, value, LatestTransaction.Hash)
		err := app.Insert(newTx, (*LatestTransaction))
		if err != nil {
			// Failed
			return abcitypes.ResponseDeliverTx{Code: 1}
		}
		newTx.Print()
	}

	return abcitypes.ResponseDeliverTx{Code: 0}
}

// CheckTx Tendermint ABCI
func (app *CayleyApplication) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {
	/*
		Disable new incoming transactions while tendermint is running by returning
		a non-zero status code in the ResponseCheckTx struct"
	*/

	if transactionsEnabled == false {
		fmt.Println("Incoming transactions are currently disabled")
		return abcitypes.ResponseCheckTx{Code: 23, GasWanted: 1}
	}
	if strings.HasPrefix(string(req.Tx), "[{") && strings.HasSuffix(string(req.Tx), "}]") {
		// Is JSON
		return abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}
	}
	code := app.isValid(req.Tx)
	if code != 0 {
		return abcitypes.ResponseCheckTx{Code: 1}
	}
	return abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}
}

// Commit Tendermint ABCI
func (app *CayleyApplication) Commit() abcitypes.ResponseCommit {
	// Save data to cayley graph
	app.DB.AddQuad(app.currentBatch)
	return abcitypes.ResponseCommit{Data: []byte{}}
}

// Query Tendermint ABCI
func (app *CayleyApplication) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {
	if string(reqQuery.Path) == "disableTx" {
		transactionsEnabled = false
		resQuery.Log = "Incoming Transactions are disabled."
		fmt.Println("Incoming Transactions are disabled.")
	} else if string(reqQuery.Path) == "enableTx" {
		transactionsEnabled = true
		resQuery.Log = "Incoming Transactions are enabled."
		fmt.Println("Incoming Transactions are enabled.")
	} else if string(reqQuery.Path) == "returnAll" {

		txs := app.ReturnAll()
		PrintAll(txs)
		SortbyDate(txs)
		result := ReturnJSON(txs)
		fmt.Println(result)

		resQuery.Value = []byte(result)
		resQuery.Log = "Returned all Transaction sorted (oldest first)"
	} else if string(reqQuery.Path) == "search" {
		if string(reqQuery.Data) == "" {
			resQuery.Log = "Error: Empty search string"
			return
		}
		// Find transacton based on hash
		query := string(reqQuery.Data)
		// Tendermint replaces + with a space
		req := strings.Replace(query, " ", "+", -1)
		hash, err := base64.StdEncoding.DecodeString(req)
		if err != nil {
			resQuery.Log = "Error: Cannot convert Hash"
			return
		}
		tx := app.Search(hash)
		if tx == nil {
			resQuery.Log = "Error: Cannot find Transaction"
			return
		}
		tx.Print()
		var txs []Transaction
		txs = append(txs, (*tx))
		out := ReturnJSON(txs)
		fmt.Println(out)
		resQuery.Value = []byte(out)
		resQuery.Log = "Found Transaction"
	} else if string(reqQuery.Path) == "returnHash" {
		txs := app.ReturnAll()
		hash := SortAndHash(txs)
		PrintAll(txs)
		fmt.Println(base64.StdEncoding.EncodeToString(hash))
		resQuery.Value = []byte(base64.StdEncoding.EncodeToString(hash))
		resQuery.Log = "Returned base64 encoded hash"
	}
	return
}

// InitChain Tendermint ABCI
func (app *CayleyApplication) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	return abcitypes.ResponseInitChain{}
}

// BeginBlock Tendermint ABCI
func (app *CayleyApplication) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	// Create an empty quad. Will be overwritten with an actual quad with data
	// Make the old data inaccessible
	// This has to be HERE. Cannot be in EndBlock, otherwise will not be commited
	app.currentBatch = quad.Make(nil, nil, nil, nil)
	return abcitypes.ResponseBeginBlock{}
}

// EndBlock Tendermint ABCI
func (app *CayleyApplication) EndBlock(req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {
	return abcitypes.ResponseEndBlock{}
}

// ReturnAll returns every transactions
func (app *CayleyApplication) ReturnAll() []Transaction {
	//schema.RegisterType("Transaction", Transaction{})
	var p *cayley.Path

	p = cayley.StartPath(app.DB)

	ctx := context.TODO()

	// Now we get an iterator for the path and optimize it.
	// The second return is if it was optimized, but we don't care for now.
	it, _ := p.BuildIterator().Optimize()

	// remember to cleanup after yourself
	defer it.Close()

	var txs []Transaction
	// While we have items
	for it.Next(ctx) {
		token := it.Result()          // get a ref to a node (backend-specific)
		value := app.DB.NameOf(token) // get the value in the node (RDF)

		nativeValue := quad.NativeOf(value) // convert value to normal Go type

		if nativeValue != "follows" {

			str, okay := nativeValue.(string)
			if okay == false {
				fmt.Println("Could not convert to string")
				continue
			}

			valH, next := extractByteArray(str)
			str = str[next:]

			valP, next := extractByteArray(str)
			str = str[next:]

			valT, next := extractByteArray(str)
			str = str[next:]

			valK, next := extractByteArray(str)
			str = str[next:]

			valV, next := extractByteArray(str)
			str = str[next:]

			newTx := RestoreTransaction(stringToByte(valH), stringToByte(valP), stringToByte(valT), stringToByte(valK), stringToByte(valV))
			txs = append(txs, newTx)

		}
	}

	return txs
}

// Search returns nil if not found
func (app *CayleyApplication) Search(hash []byte) *Transaction {
	var p *cayley.Path
	p = cayley.StartPath(app.DB)
	ctx := context.TODO()
	// Now we get an iterator for the path and optimize it.
	// The second return is if it was optimized, but we don't care for now.
	it, _ := p.BuildIterator().Optimize()
	// remember to cleanup after yourself
	defer it.Close()

	// While we have items
	for it.Next(ctx) {
		token := it.Result()                // get a ref to a node (backend-specific)
		value := app.DB.NameOf(token)       // get the value in the node (RDF)
		nativeValue := quad.NativeOf(value) // convert value to normal Go type

		if nativeValue != "follows" {

			str, okay := nativeValue.(string)
			if okay == false {
				fmt.Println("Could not convert to string")
				continue
			}

			valH, next := extractByteArray(str)
			str = str[next:]

			if bytes.Compare(stringToByte(valH), hash) == 0 {
				// We found the transaction we were looking for
				valP, next := extractByteArray(str)
				str = str[next:]

				valT, next := extractByteArray(str)
				str = str[next:]

				valK, next := extractByteArray(str)
				str = str[next:]

				valV, next := extractByteArray(str)
				str = str[next:]

				newTx := RestoreTransaction(stringToByte(valH), stringToByte(valP), stringToByte(valT), stringToByte(valK), stringToByte(valV))
				return &newTx
			}
		}
	}
	return nil
}
func (app *CayleyApplication) isValid(tx []byte) (code uint32) {

	// check format
	parts := bytes.Split(tx, []byte("="))
	if len(parts) != 2 {
		fmt.Println("Malformed")
		return 1
	}
	for i := 0; i < 2; i++ {
		if string(parts[i][0]) != "<" || string(parts[i][len(parts[i])-1]) != ">" {
			fmt.Println("Malformed")
			return 1
		}
	}

	// If desired, check if the same key=value already exists

	return 0
}

// Insert adds a new transction to the DAG
func (app *CayleyApplication) Insert(tx Transaction, prevTx Transaction) error {
	err := app.DB.AddQuad(quad.Make(tx, "follows", prevTx, nil))
	LatestTransaction = (&tx)
	return err
}

// NewTransaction creates a new Transaction struct
func NewTransaction(key []byte, value []byte, prevHash []byte) Transaction {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(time.Now().UnixNano()))
	transaction := Transaction{[]byte{}, prevHash, b, key, value}
	transaction.SetHash()
	return transaction
}

//RestoreTransaction restores a transaction fetched from the DAG
func RestoreTransaction(hash []byte, prevHash []byte, timestamp []byte, key []byte, value []byte) Transaction {
	transaction := Transaction{hash, prevHash, timestamp, key, value}
	return transaction
}

// SetHash sets the hash of a transaction
func (t *Transaction) SetHash() {
	headers := bytes.Join([][]byte{t.PrevHash, t.Key, t.Value, t.Timestamp}, []byte{})
	hash := sha256.Sum256(headers)
	t.Hash = hash[:]
}

// Print print the every value of the transaction as a string
func (t Transaction) Print() {
	fmt.Printf("Key: %s\n", t.Key)
	fmt.Printf("Value: %s\n", t.Value)
	fmt.Println("Hash: " + base64.StdEncoding.EncodeToString(t.Hash))
	fmt.Println("Prev. Hash: " + base64.StdEncoding.EncodeToString(t.PrevHash))
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
		return base64.StdEncoding.EncodeToString(txs[i].Hash) < base64.StdEncoding.EncodeToString(txs[j].Hash)
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
				fmt.Println("TODO: Could not insert b/c previous transaction is not in the database. Skipping this Tx")
				// I think we can just insert the tranaction with prev. Tx as nil
				// Would create an unconnected side chain which will become connected later at some point
				// The graph iterator will still find this unconnected transaction
				continue
			}

			app.Insert(txs[i], (*prevTx))
			fmt.Println("Transaction inserted")
		} else {
			fmt.Println("Transaction exists")
		}
	}
}

// Helper Functions

// Returns the string representation of a []byte as a []byte
func stringToByte(str string) []byte {
	var bb []byte
	if str == "[]" {
		return []byte{}
	}
	for _, ps := range strings.Split(strings.Trim(str, "[]"), " ") {
		pi, _ := strconv.Atoi(ps)
		bb = append(bb, byte(pi))
	}
	return bb
}

// Returns the first []byte in the string as a string
// next is the index after the end of the []byte string
func extractByteArray(str string) (string, int) {
	startIndex := strings.Index(str, "[")
	endIndex := strings.Index(str, "]")
	value := str[startIndex : endIndex+1]
	return value, (endIndex + 1)
}
