package main

import (
	"context"
	"fmt"
	"log"

	abcitypes "github.com/tendermint/tendermint/abci/types"

	"github.com/cayleygraph/cayley"
	_ "github.com/cayleygraph/cayley/graph/kv/bolt"
	"github.com/cayleygraph/quad"
)

type CayleyApplication struct {
	db *cayley.Handle
	//currentBatch *badger.Txn
	//cBatch *cayleygraph.Transaction
	currentBatch quad.Quad
}

func NewCayleyApplication(db *cayley.Handle) *CayleyApplication {
	return &CayleyApplication{
		db: db,
	}
}

var _ abcitypes.Application = (*CayleyApplication)(nil)
var transactionsEnabled = true

func (app *CayleyApplication) Info(req abcitypes.RequestInfo) abcitypes.ResponseInfo {
	return abcitypes.ResponseInfo{}
}

func (app *CayleyApplication) SetOption(req abcitypes.RequestSetOption) abcitypes.ResponseSetOption {
	return abcitypes.ResponseSetOption{}
}

func (app *CayleyApplication) DeliverTx(req abcitypes.RequestDeliverTx) abcitypes.ResponseDeliverTx {

	code := app.isValid(req.Tx)

	/*
		Transaction can also be disabled at this point, before they get delivered
		Disable new incoming transactions while tendermint is running by returning
		a non-zero status code in the ResponseCheckTx struct"
	*/

	if code != 0 {
		return abcitypes.ResponseDeliverTx{Code: code}
	}

	/*
		parts := bytes.Split(req.Tx, []byte("="))
		key, value := parts[0], parts[1]

		err := app.currentBatch.Set(key, value)
		if err != nil {
			panic(err)
		}
	*/

	app.currentBatch = quad.Make("phrase of the day", "is of course", string(req.Tx), nil)

	return abcitypes.ResponseDeliverTx{Code: 0}
}

func (app *CayleyApplication) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {
	/*
		Disable new incoming transactions while tendermint is running by returning
		a non-zero status code in the ResponseCheckTx struct"
	*/
	if transactionsEnabled == false {
		fmt.Println("Incoming transactions are currently disabled")
		return abcitypes.ResponseCheckTx{Code: 23, GasWanted: 1}
	}

	code := app.isValid(req.Tx)
	return abcitypes.ResponseCheckTx{Code: code, GasWanted: 1}
}

func (app *CayleyApplication) Commit() abcitypes.ResponseCommit {
	//app.currentBatch.Commit()
	// Save data to cayley
	app.db.AddQuad(app.currentBatch)
	return abcitypes.ResponseCommit{Data: []byte{}}
}

func (app *CayleyApplication) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {
	// Path does not seem to work for me. Always empty
	/*
		switch reqQuery.Path {
		case "disableTx":
			transactionsEnabled = false
			resQuery.Log = "Incoming Transactions are disabled."
			fmt.Println("Incoming Transactions are disabled.")
		case "enableTx":
			transactionsEnabled = true
			resQuery.Log = "Incoming Transactions are enabled."
			fmt.Println("Incoming Transactions are enabled.")
		case "data":
			// Now we create the path, to get to our data
			p := cayley.StartPath(app.db, quad.String("phrase of the day")).Out(quad.String("is of course"))

			ctx := context.TODO()
			// Now we get an iterator for the path and optimize it.
			// The second return is if it was optimized, but we don't care for now.
			it, _ := p.BuildIterator().Optimize()
			//it := its.Iterate()

			// remember to cleanup after yourself
			defer it.Close()

			result := ""

			// While we have items
			for it.Next(ctx) {
				token := it.Result()                // get a ref to a node (backend-specific)
				value := app.db.NameOf(token)       // get the value in the node (RDF)
				nativeValue := quad.NativeOf(value) // convert value to normal Go type

				fmt.Println(nativeValue) // print it!
				result += string(nativeValue.(string))
				result += ";"
				resQuery.Value = []byte(result)
			}
			if err := it.Err(); err != nil {
				log.Fatalln(err)
			}
		default:
			resQuery.Log = fmt.Sprintf("Invalid query path. Got %v", reqQuery.Path)
		}
	*/

	if string(reqQuery.Data) == "disableTx" {
		transactionsEnabled = false
		resQuery.Log = "Incoming Transactions are disabled."
		fmt.Println("Incoming Transactions are disabled.")
	} else if string(reqQuery.Data) == "enableTx" {
		transactionsEnabled = true
		resQuery.Log = "Incoming Transactions are enabled."
		fmt.Println("Incoming Transactions are enabled.")
	} else if string(reqQuery.Data) == "returnAll" {
		// Now we create the path, to get to our data
		p := cayley.StartPath(app.db, quad.String("phrase of the day")).Out(quad.String("is of course"))

		ctx := context.TODO()
		// Now we get an iterator for the path and optimize it.
		// The second return is if it was optimized, but we don't care for now.
		it, _ := p.BuildIterator().Optimize()

		// remember to cleanup after yourself
		defer it.Close()

		result := ""

		// While we have items
		for it.Next(ctx) {
			token := it.Result()                // get a ref to a node (backend-specific)
			value := app.db.NameOf(token)       // get the value in the node (RDF)
			nativeValue := quad.NativeOf(value) // convert value to normal Go type

			fmt.Println(nativeValue) // print it!
			result += string(nativeValue.(string))
			result += ";"
			resQuery.Value = []byte(result)
		}
		if err := it.Err(); err != nil {
			log.Fatalln(err)
		}

		fmt.Println("Returned all values")
	}

	return
}

func (app *CayleyApplication) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	return abcitypes.ResponseInitChain{}
}

func (app *CayleyApplication) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	//app.currentBatch = app.db.NewTransaction(true)

	// Create an empty quad? Will be overwritten with an actual quad with data
	// Make the old data inacessible
	app.currentBatch = quad.Make(nil, nil, nil, nil)
	return abcitypes.ResponseBeginBlock{}
}

func (app *CayleyApplication) EndBlock(req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {
	return abcitypes.ResponseEndBlock{}
}

func (app *CayleyApplication) isValid(tx []byte) (code uint32) {
	// Currently always valid
	return 0

	// We would need to create a new validator for cayley values

	/*
		// check format
		parts := bytes.Split(tx, []byte("="))
		if len(parts) != 2 {
			return 1
		}

		key, value := parts[0], parts[1]

		// check if the same key=value already exists
		err := app.db.View(func(txn *badger.Txn) error {
			item, err := txn.Get(key)
			if err != nil && err != badger.ErrKeyNotFound {
				return err
			}
			if err == nil {
				return item.Value(func(val []byte) error {
					if bytes.Equal(val, value) {
						code = 2
					}
					return nil
				})
			}
			return nil
		})
		if err != nil {
			panic(err)
		}

		return code
	*/
}
