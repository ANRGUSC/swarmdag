package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	abcitypes "github.com/tendermint/tendermint/abci/types"

	"github.com/cayleygraph/cayley"
	_ "github.com/cayleygraph/cayley/graph/kv/bolt"
	"github.com/cayleygraph/quad"
)

// CayleyApplication stores db and current struct
type CayleyApplication struct {
	db           *cayley.Handle
	currentBatch quad.Quad
}

// NewCayleyApplication creates new Instance
func NewCayleyApplication(db *cayley.Handle) *CayleyApplication {
	return &CayleyApplication{
		db: db,
	}
}

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

	fmt.Println(req.Tx)
	fmt.Println(string(req.Tx))

	request := req.Tx
	requestString := string(request)

	// Detect the JSON format using [{"hash" prefix
	// Otherwise, create a new key, value transaction

	if strings.HasPrefix(requestString, "[{\"hash\"") {
		// Is JSON
		app.InsertFromJSON(request)
	} else {
		// <key>=<value>
		parts := bytes.Split(request, []byte("="))
		key, value := parts[0][1:len(parts[0])-1], parts[1][1:len(parts[1])-1]
		fmt.Println("New Transaction with " + string(key) + " " + string(value))
		if (len(key) == 0) || (len(value) == 0) {
			// Failed
			return abcitypes.ResponseDeliverTx{Code: 1}
		}
		newTx := NewTransaction(key, value, latestTransaction.Hash)
		err := app.Insert(newTx, (*latestTransaction))
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

	return abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}
}

// Commit Tendermint ABCI
func (app *CayleyApplication) Commit() abcitypes.ResponseCommit {
	// Save data to cayley graph
	app.db.AddQuad(app.currentBatch)
	return abcitypes.ResponseCommit{Data: []byte{}}
}

// Query Tendermint ABCI
func (app *CayleyApplication) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {
	// Debug: See if data and path work
	fmt.Println("Request Query: Data=" + string(reqQuery.Data) + " Path=" + reqQuery.Path)

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
		SortbyDate(txs)
		result := ReturnJSON(txs)

		resQuery.Value = []byte(result)
		resQuery.Log = "Returned all Transaction sorted (oldest first)"
	} else if string(reqQuery.Path) == "find" {
		if string(reqQuery.Data) == "" {
			resQuery.Log = "Error: Empty search string"
		} else {
			// Find transacton based on hash
			//fmt.Println(reqQuery.Data)
			//fmt.Println(string(reqQuery.Data))
			query := string(reqQuery.Data)
			req := strings.Replace(query, " ", "+", -1)
			hash, err := base64.StdEncoding.DecodeString(req)
			if err != nil {
				resQuery.Log = "Error: Cannot convert Hash"
			}
			tx := app.Search(hash)
			if tx == nil {
				resQuery.Log = "Error: Cannot find Transaction"
			}
			tx.Print()
			var txs []Transaction
			txs = append(txs, (*tx))
			out := ReturnJSON(txs)
			resQuery.Value = []byte(out)
		}
	} else if string(reqQuery.Path) == "returnHash" {
		txs := app.ReturnAll()
		hash := SortAndHash(txs)
		fmt.Printf("Hash: %x\n", hash)
		resQuery.Value = []byte(fmt.Sprintf("%x", hash))
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

	p = cayley.StartPath(app.db)

	ctx := context.TODO()

	// Now we get an iterator for the path and optimize it.
	// The second return is if it was optimized, but we don't care for now.
	it, _ := p.BuildIterator().Optimize()

	// remember to cleanup after yourself
	defer it.Close()

	//result := ""
	/*
		iterator := app.db.QuadsAllIterator()
		defer iterator.Close()
		for iterator.Next(ctx) {

				token := iterator.Result()    // get a ref to a node (backend-specific)
				value := app.db.NameOf(token) // get the value in the node (RDF)
				nativeValue := quad.NativeOf(value)
				tx := nativeValue.(Transaction)
				fmt.Println(tx)
				tx.Print()

			fmt.Println(app.db.Quad(it.Result()))
		}


			err := schema.LoadTo(ctx, app.db, &txs)
			if err != nil {
				panic(err)
			}

			for _, t := range txs {
				t.Print()
			}
	*/
	var txs []Transaction
	// While we have items
	for it.Next(ctx) {
		token := it.Result()          // get a ref to a node (backend-specific)
		value := app.db.NameOf(token) // get the value in the node (RDF)
		//fmt.Print("Type is: ")
		//fmt.Println(reflect.TypeOf(value))
		//go build
		//fmt.Println(value)
		nativeValue := quad.NativeOf(value) // convert value to normal Go type

		if nativeValue != "follows" {
			//fmt.Println(nativeValue) // print it!
			//fmt.Print("Type is: ")
			//fmt.Println(reflect.TypeOf(nativeValue))

			str, okay := nativeValue.(string)
			if okay == false {
				fmt.Println("Could not convert to string")
			} else {
				//fmt.Println("okay")
				//fmt.Println(str)
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
			//newTx.Print()
			txs = append(txs, newTx)

			/*
				t, ok := nativeValue.(Transaction)
				if ok == true {
					fmt.Println("ok")
					t.Print()
				}
				txs = append(txs, t)
			*/
		}
	}

	PrintAll(txs)

	return txs
}

// Search returns nil if not found
func (app *CayleyApplication) Search(hash []byte) *Transaction {
	var p *cayley.Path
	p = cayley.StartPath(app.db)
	ctx := context.TODO()
	// Now we get an iterator for the path and optimize it.
	// The second return is if it was optimized, but we don't care for now.
	it, _ := p.BuildIterator().Optimize()
	// remember to cleanup after yourself
	defer it.Close()

	// While we have items
	for it.Next(ctx) {
		token := it.Result()                // get a ref to a node (backend-specific)
		value := app.db.NameOf(token)       // get the value in the node (RDF)
		nativeValue := quad.NativeOf(value) // convert value to normal Go type

		if nativeValue != "follows" {

			str, okay := nativeValue.(string)
			if okay == false {
				fmt.Println("Could not convert to string")
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
	//fmt.Println("Value: " + value)
	return value, (endIndex + 1)
}
