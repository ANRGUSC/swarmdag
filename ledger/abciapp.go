package ledger

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	logging "github.com/op/go-logging"
)

// ABCIApp stores db and current struct
type ABCIApp struct {
	dag       		*DAG
	log			 	*logging.Logger
	chainID 		string
	txEnabled		bool
}

var _ abcitypes.Application = (*ABCIApp)(nil)

// NewABCIApp creates new Instance
func NewABCIApp(dag *DAG, log *logging.Logger, chainID string) *ABCIApp {
	dag.CreateAlphaTx(chainID)
    return &ABCIApp{
    	dag: dag,
    	log: log,
    	chainID: chainID,
    	txEnabled: true,
    }
}

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
	if app.txEnabled == false {
		app.log.Info("Incoming transactions are currently disabled")
		return abcitypes.ResponseDeliverTx{Code: 23}
	}

	var tx Transaction
	txBody := req.Tx
	if strings.HasPrefix(string(txBody), "{") {
		err := json.Unmarshal(txBody, &tx)
		if err != nil {
			app.log.Warning("Transaction unmarshal error")
		} else {
			tx.MembershipID = app.chainID
			app.dag.InsertTx(&tx)
		}
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
	if app.txEnabled == false {
		app.log.Info("Incoming transactions are currently disabled")
		return abcitypes.ResponseCheckTx{Code: 23, GasWanted: 1}
	}
	if strings.HasPrefix(string(req.Tx), "{") {
		// Is JSON
		return abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}
	}
	return abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}
}

// Commit Tendermint ABCI
func (app *ABCIApp) Commit() abcitypes.ResponseCommit {
	return abcitypes.ResponseCommit{Data: []byte{}}
}

// Query Tendermint ABCI
func (app *ABCIApp) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {
	switch string(reqQuery.Data) {
	case "disableTx":
		app.txEnabled = false
		resQuery.Log = "Incoming Transactions are disabled."
		app.log.Info("Incoming Transactions are disabled.")
	case "enableTx":
		app.txEnabled = true
		resQuery.Log = "Incoming Transactions are enabled."
		app.log.Info("Incoming Transactions are enabled.")
	case "returnAll":
		txs := app.dag.ReturnAll()
		txJson, _ := json.MarshalIndent(txs, "", "    ")
		app.log.Info(string(txJson))
		resQuery.Value = txJson
		resQuery.Log = "Returned all Transaction sorted (oldest first)"
	case "search":
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
		txJson, _ := json.Marshal(tx)
		resQuery.Value = txJson
		resQuery.Log = "Transaction Found"
	default:
		resQuery.Log = "Error: invalid query"
	}
	return
}

// InitChain Tendermint ABCI
func (app *ABCIApp) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	return abcitypes.ResponseInitChain{}
}

// BeginBlock Tendermint ABCI
func (app *ABCIApp) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	return abcitypes.ResponseBeginBlock{}
}

// EndBlock Tendermint ABCI
func (app *ABCIApp) EndBlock(req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {
	return abcitypes.ResponseEndBlock{}
}
