package ledger

import (
	"crypto/sha1"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"encoding/hex"

	abcitypes "github.com/tendermint/tendermint/abci/types"
	_ "github.com/cayleygraph/cayley/graph/kv/bolt"
	"github.com/cayleygraph/quad"
	logging "github.com/op/go-logging"
)

// ABCIApp stores db and current struct
type ABCIApp struct {
	dag       		*DAG
	currentBatch 	quad.Quad
	log			 	*logging.Logger
	chainID 	string
	headHash	 	string
}

// NewABCIApp creates new Instance
func NewABCIApp(dag *DAG, log *logging.Logger, chainID string) *ABCIApp {
	dag.CreateAlphaTx(chainID)
    return &ABCIApp{
    	dag: dag,
    	log: log,
    	chainID: chainID,
    }
}

// LatestTransaction Important: Finding the latest transction on a restart of the node might be difficult
// if there are multiple chains. We probably want to append to the longest chain.
// We would need to find the longest chain and point to latest transaction of it
var LatestTransaction *Transaction = nil

var _ abcitypes.Application = (*ABCIApp)(nil)
var transactionsEnabled = true

// Info Tendermint ABCI
func (app *ABCIApp) Info(req abcitypes.RequestInfo) abcitypes.ResponseInfo {
	return abcitypes.ResponseInfo{}
}

// SetOption Tendermint ABCI
func (app *ABCIApp) SetOption(req abcitypes.RequestSetOption) abcitypes.ResponseSetOption {
	return abcitypes.ResponseSetOption{}
}

// DeliverTx Tendermint ABCI
func (app *ABCIApp) DeliverTx(req abcitypes.RequestDeliverTx) abcitypes.ResponseDeliverTx {
	/*
		Transaction can also be disabled at this point, before they get delivered
		Disable new incoming transactions while tendermint is running by returning
		a non-zero status code in the ResponseCheckTx struct"
	*/
	if transactionsEnabled == false {
		app.log.Info("Incoming transactions are currently disabled")
		return abcitypes.ResponseDeliverTx{Code: 23}
	}

	txBody := string(req.Tx)

	if strings.HasPrefix(txBody, "{") {
		fmt.Println("jason:" + txBody)
		app.InsertFromJSON([]byte(txBody))
	} else {
		app.log.Warning("Transaction body invalid")
		return abcitypes.ResponseDeliverTx{Code: 23}
	}

	return abcitypes.ResponseDeliverTx{Code: 0}
}

// CheckTx Tendermint ABCI
func (app *ABCIApp) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {
	/*
		Disable new incoming transactions while tendermint is running by returning
		a non-zero status code in the ResponseCheckTx struct"
	*/
	if transactionsEnabled == false {
		fmt.Println("Incoming transactions are currently disabled")
		return abcitypes.ResponseCheckTx{Code: 23, GasWanted: 1}
	}
	if strings.HasPrefix(string(req.Tx), "{") {
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
func (app *ABCIApp) Commit() abcitypes.ResponseCommit {
	return abcitypes.ResponseCommit{Data: []byte{}}
}

// Query Tendermint ABCI
func (app *ABCIApp) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {
	if string(reqQuery.Data) == "disableTx" {
		transactionsEnabled = false
		resQuery.Log = "Incoming Transactions are disabled."
		fmt.Println("Incoming Transactions are disabled.")
	} else if string(reqQuery.Data) == "enableTx" {
		transactionsEnabled = true
		resQuery.Log = "Incoming Transactions are enabled."
		fmt.Println("Incoming Transactions are enabled.")
	} else if string(reqQuery.Data) == "returnAll" {

		txs := app.dag.ReturnAll()
		PrintAll(txs)
		SortbyDate(txs)
		result := ReturnJSON(txs)
		// fmt.Println(result)

		resQuery.Value = []byte(result)
		resQuery.Log = "Returned all Transaction sorted (oldest first)"
	} else if string(reqQuery.Data) == "search" {
		if string(reqQuery.Data) == "" {
			resQuery.Log = "Error: Empty search string"
			return
		}
		// Find transacton based on hash
		query := string(reqQuery.Data)
		hash, err := base64.StdEncoding.DecodeString(query)
		if err != nil {
			resQuery.Log = "Error: Cannot convert Hash"
			return
		}
		tx := app.dag.Search(string(hash))
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
	} else if string(reqQuery.Data) == "returnHash" {
		txs := app.dag.ReturnAll()
		hash := SortAndHash(txs)
		PrintAll(txs)
		fmt.Println(base64.StdEncoding.EncodeToString([]byte(hash)))
		resQuery.Value = []byte(base64.StdEncoding.EncodeToString([]byte(hash)))
		resQuery.Log = "Returned base64 encoded hash"
	}
	return
}

// InitChain Tendermint ABCI
func (app *ABCIApp) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	return abcitypes.ResponseInitChain{}
}

// BeginBlock Tendermint ABCI
func (app *ABCIApp) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	// Create an empty quad. Will be overwritten with an actual quad with data
	// Make the old data inaccessible
	// This has to be HERE. Cannot be in EndBlock, otherwise will not be commited
	app.currentBatch = quad.Make(nil, nil, nil, nil)
	return abcitypes.ResponseBeginBlock{}
}

// EndBlock Tendermint ABCI
func (app *ABCIApp) EndBlock(req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {
	return abcitypes.ResponseEndBlock{}
}



func (app *ABCIApp) isValid(tx []byte) (code uint32) {
	// check format
	parts := bytes.Split(tx, []byte("="))
	if len(parts) != 2 {
		fmt.Println("Malformed")
		return 1
	}

	// If desired, check if the same key=value already exists
	return 0
}

// Insert adds a new transction to the DAG
func (app *ABCIApp) Insert(tx Transaction, prevTx Transaction) error {
	err := app.dag.DB.AddQuad(quad.Make(tx, "follows", prevTx, nil))

	LatestTransaction = (&tx)
	return err
}

//RestoreTransaction restores a transaction fetched from the DAG
func RestoreTransaction(hash, parentHash string, timestamp int64,
						chainID, key, value string) Transaction {
	transaction := Transaction{hash, parentHash, timestamp, chainID, key, value}
	return transaction
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
		return txs[i].Hash < txs[j].Hash
	})
}

// SortbyDate sorts the array based on the date on the transactions (oldest first)
func SortbyDate(txs []Transaction) {
	sort.Slice(txs, func(i, j int) bool {
		return txs[i].Timestamp < txs[j].Timestamp
	})
}

//SortAndHash sorts the array and returns the Hash
func SortAndHash(txs []Transaction) string {
	// TODO: determine if this sort and hash will provide consistent results
	// across network
	SortbyDate(txs)
	var buffer bytes.Buffer
	for i := range txs {
		buffer.Write([]byte(txs[i].Hash))
	}
	hash := sha1.Sum(buffer.Bytes())
	return hex.EncodeToString(hash[:])
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
func (app *ABCIApp) InsertFromJSON(jsonInput []byte) {
	// convert the JSON to structs
	var tx Transaction
	err := json.Unmarshal(jsonInput, &tx)
	if err != nil {
		panic(err)
	}


	if tx.MembershipID == "" {
		tx.MembershipID = app.chainID
	}

	// Use search function to see if transaction exist already
	// If not, insert into cayley by finding the previous tx
	if app.dag.Search(tx.Hash) == nil {

		// What if previous Hash does not exist
		// Find the previous Transaction
		prevTx := app.dag.Search(tx.ParentHash)

		if prevTx == nil {
			app.log.Warningf("parentHash does not exist: %s", tx.ParentHash)
		}

		app.dag.InsertTx(&tx)
		app.log.Debug("Transaction inserted")
	} else {
		app.log.Info("Transaction exists, skipping insertion!")
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
