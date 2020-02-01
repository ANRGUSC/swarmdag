package kvstore_counter

import (
    "encoding/binary"
    "encoding/json"
    "fmt"
    "os"

    kv "github.com/tendermint/tendermint/libs/kv"
    dbm "github.com/tendermint/tendermint/libs/db"
    "github.com/tendermint/tendermint/libs/log"

    "github.com/tendermint/tendermint/abci/example/code"
    "github.com/tendermint/tendermint/abci/types"
    // "github.com/tendermint/tendermint/abci/example/kvstore"
    "github.com/tendermint/tendermint/version"

)

//NOTE: SwarmDAG kvstore counter example has not been tested to work yet.

var (
    stateKey        = []byte("stateKey")
    // kvPairPrefixKey = []byte("swarmdagKey:")
)

type State struct {
    db      dbm.DB
    Size    int64  `json:"size"`
    Height  int64  `json:"height"`
    AppHash []byte `json:"app_hash"`
}

func loadState(db dbm.DB) State {
    stateBytes := db.Get(stateKey)
    var state State
    if len(stateBytes) != 0 {
        err := json.Unmarshal(stateBytes, &state)
        if err != nil {
            panic(err)
        }
    }
    state.db = db
    return state
}

func saveState(state State) {
    stateBytes, err := json.Marshal(state)
    if err != nil {
        panic(err)
    }
    state.db.Set(stateKey, stateBytes)
}

// func prefixKey(key []byte) []byte {
//     return append(kvPairPrefixKey, key...)
// }

//---------------------------------------------------

var _ types.Application = (*SwarmDAGKVStoreCountApp)(nil)

type SwarmDAGKVStoreCountApp struct {
    types.BaseApplication

    hashCount int
    txCount   int
    state     State
    logger log.Logger
    node_id   string
}

func NewKVStoreCountApp(dbDir string) *SwarmDAGKVStoreCountApp {
    //create persistent kvstore
    fmt.Println("starting up swarmdag counter app...")
    name := "kvstore"
    db, err := dbm.NewGoLevelDB(name, dbDir)
    if err != nil {
        panic(err)
    }
    state := loadState(db)
    node_id := os.Getenv("ID")
    return &SwarmDAGKVStoreCountApp{state: state, node_id: node_id, logger: log.NewNopLogger()}
}

func (app *SwarmDAGKVStoreCountApp) SetLogger(l log.Logger) {
    app.logger = l
}

func (app *SwarmDAGKVStoreCountApp) Info(req types.RequestInfo) (resInfo types.ResponseInfo) {
    resInfo.Data = fmt.Sprintf("{\"hashes\":%v,\"txs\":%v,\"size\":%v}", 
                                app.hashCount, app.txCount, app.state.Size)
    resInfo.Version = version.ABCIVersion
    resInfo.LastBlockHeight = app.state.Height
    resInfo.LastBlockAppHash = app.state.AppHash
    return
}

func (app *SwarmDAGKVStoreCountApp) SetOption(req types.RequestSetOption) types.ResponseSetOption {
    return types.ResponseSetOption{}
}

func (app *SwarmDAGKVStoreCountApp) DeliverTx(tx []byte) types.ResponseDeliverTx {
    fmt.Println("DeliveTx():")
    if len(tx) > 8 {
        return types.ResponseDeliverTx{
            Code: code.CodeTypeEncodingError,
            Log:  fmt.Sprintf("Max tx size is 8 bytes, got %d", len(tx))}
    }
    tx8 := make([]byte, 8)
    copy(tx8[len(tx8)-len(tx):], tx)
    txValue := binary.BigEndian.Uint64(tx8)
    if txValue != uint64(app.txCount) {
        return types.ResponseDeliverTx{
            Code: code.CodeTypeBadNonce,
            Log:  fmt.Sprintf("Invalid nonce. Expected %v, got %v", app.txCount, txValue)}
    }
    app.txCount++
    app.state.db.Set(tx8, []byte(app.node_id)) 
    app.state.Size += 1

    tags := []kv.KVPair{
        {Key: []byte("app.creator"), Value: []byte("Cosmoshi Netowoko")},
        {Key: []byte("app.key"), Value: tx8},
    }
    return types.ResponseDeliverTx{Code: code.CodeTypeOK, Tags: tags}
}

func (app *SwarmDAGKVStoreCountApp) CheckTx(tx []byte) types.ResponseCheckTx {
    fmt.Println("CheckTx():")
    if len(tx) > 8 {
        return types.ResponseCheckTx{
            Code: code.CodeTypeEncodingError,
            Log:  fmt.Sprintf("Max tx size is 8 bytes, got %d", len(tx))}
    }
    tx8 := make([]byte, 8)
    copy(tx8[len(tx8)-len(tx):], tx)
    txValue := binary.BigEndian.Uint64(tx8)
    if txValue != uint64(app.txCount) {
        return types.ResponseCheckTx{
            Code: code.CodeTypeBadNonce,
            Log:  fmt.Sprintf("Invalid nonce. Expected >= %v, got %v", app.txCount, txValue)}
    }

    return types.ResponseCheckTx{Code: code.CodeTypeOK}
}

func (app *SwarmDAGKVStoreCountApp) Commit() (resp types.ResponseCommit) {
    fmt.Println("Commit():")
    app.hashCount++
    if app.txCount == 0 {
        return types.ResponseCommit{}
    }
    hash := make([]byte, 8)
    binary.BigEndian.PutUint64(hash, uint64(app.txCount))
    app.state.AppHash = hash
    app.state.Height += 1
    saveState(app.state)
    return types.ResponseCommit{Data: hash}
}

func (app *SwarmDAGKVStoreCountApp) Query(reqQuery types.RequestQuery) (resQuery types.ResponseQuery) {
    switch reqQuery.Path {
    case "hash":
        resQuery.Value = []byte(fmt.Sprintf("%v", app.hashCount))
    case "txCount":
        resQuery.Value = []byte(fmt.Sprintf("%v", app.txCount))
    case "countLookup":
       resQuery.Key = reqQuery.Data
       value := app.state.db.Get(reqQuery.Data)
       resQuery.Value = value
       if value != nil {
        resQuery.Log = "exists"
       } else {
            resQuery.Log = "does not exist"
       }
    default:
        resQuery.Log = fmt.Sprintf("Invalid query path. Got %v", reqQuery.Path)
    }

    return 
}
