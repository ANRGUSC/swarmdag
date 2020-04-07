package main

import (
	"bytes"
	"context"
	"fmt"
	"log"

	abcitypes "github.com/tendermint/tendermint/abci/types"

	"github.com/cayleygraph/cayley"
	_ "github.com/cayleygraph/cayley/graph/kv/bolt"
	"github.com/cayleygraph/quad"
)

type CayleyApplication struct {
	db           *cayley.Handle
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

	parts := bytes.Split(req.Tx, []byte("="))
	subject, predicate, object, tag := parts[0][1:len(parts[0])-1], parts[1][1:len(parts[1])-1],
		parts[2][1:len(parts[2])-1], parts[3][1:len(parts[3])-1]

	if len(subject) == 0 {
		subject = nil
	}
	if len(predicate) == 0 {
		predicate = nil
	}
	if len(object) == 0 {
		object = nil
	}
	if len(tag) == 0 {
		tag = nil
	}

	app.currentBatch = quad.Make(string(subject), string(predicate), string(object), string(tag))

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
	// Save data to cayley graph
	app.db.AddQuad(app.currentBatch)
	return abcitypes.ResponseCommit{Data: []byte{}}
}

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
	} else if (string(reqQuery.Path) == "find") || (string(reqQuery.Path) == "returnAll") {
		//<subject>=<predicate> returns object

		var p *cayley.Path

		// Return all case
		if string(reqQuery.Path) == "returnAll" {
			p = cayley.StartPath(app.db)
		} else {
			parts := bytes.Split(reqQuery.Data, []byte("="))
			// Check format
			if len(parts) != 2 {
				return
			}
			for i := 0; i < 2; i++ {
				if string(parts[i][0]) != "<" || string(parts[i][len(parts[i])-1]) != ">" {
					fmt.Println("Malformed")
					return
				}
			}

			subject := parts[0][1 : len(parts[0])-1]
			predicate := parts[1][1 : len(parts[1])-1]
			p = cayley.StartPath(app.db, quad.String(subject)).Out(quad.String(predicate))
		}

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
		}
		if err := it.Err(); err != nil {
			log.Fatalln(err)
		}
		resQuery.Value = []byte(result)
	}

	return
}

func (app *CayleyApplication) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	return abcitypes.ResponseInitChain{}
}

func (app *CayleyApplication) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	// Create an empty quad. Will be overwritten with an actual quad with data
	// Make the old data inaccessible
	// This has to be HERE. Cannot be in EndBlock, otherwise will not be commited
	app.currentBatch = quad.Make(nil, nil, nil, nil)
	return abcitypes.ResponseBeginBlock{}
}

func (app *CayleyApplication) EndBlock(req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {
	return abcitypes.ResponseEndBlock{}
}

func (app *CayleyApplication) isValid(tx []byte) (code uint32) {

	// check format
	parts := bytes.Split(tx, []byte("="))
	if len(parts) != 4 {
		fmt.Println("Malformed")
		return 1
	}
	for i := 0; i < 4; i++ {
		if string(parts[i][0]) != "<" || string(parts[i][len(parts[i])-1]) != ">" {
			fmt.Println("Malformed")
			return 1
		}
	}

	// If desired, check if the same key=value already exists

	return 0
}
