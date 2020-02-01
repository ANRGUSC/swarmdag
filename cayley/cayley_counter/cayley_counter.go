package cayley_counter

import (
    // "bytes"
    "encoding/binary"
    "encoding/hex"
    "fmt"
    "os"
    "strconv"

    "crypto/rand"
    "crypto/sha1"
    cmn "github.com/tendermint/tendermint/libs/common"
    "github.com/tendermint/tendermint/libs/log"

    "github.com/tendermint/tendermint/abci/example/code"
    "github.com/tendermint/tendermint/abci/types"
    "github.com/tendermint/tendermint/version"

    "github.com/cayleygraph/cayley"
    "github.com/cayleygraph/cayley/graph"
    _ "github.com/cayleygraph/cayley/graph/kv/bolt"
    "github.com/cayleygraph/cayley/quad"
)

type State struct {
    store   *cayley.Handle
    Size    int64  `json:"size"`
    Height  int64  `json:"height"`
    AppHash []byte `json:"app_hash"`
}

func loadState(store *cayley.Handle) State {
    var state State
    state.store = store
    return state
}

//---------------------------------------------------

var _ types.Application = (*SwarmDAGCountApp)(nil)

type SwarmDAGCountApp struct {
    types.BaseApplication

    hashCount int
    txCount   int
    state     State
    logger log.Logger
    node_id   string
    lastHash  string
}

func NewSwarmDAGCountApp(dbDir string) *SwarmDAGCountApp {

    // Some globally applicable things
    graph.IgnoreMissing = true
    graph.IgnoreDuplicates = true

    fmt.Println("starting up SwarmDAG counter app...")

    // TODO: find a container-friendly location for the database 
    store := initializeAndOpenGraph("db.boltdb") // initialize the graph

    state := loadState(store)
    node_id := os.Getenv("ID")
    return &SwarmDAGCountApp{state: state, node_id: node_id, logger: log.NewNopLogger()}
}

func (app *SwarmDAGCountApp) SetLogger(l log.Logger) {
    app.logger = l
}

func (app *SwarmDAGCountApp) Info(req types.RequestInfo) (resInfo types.ResponseInfo) {
    resInfo.Data = fmt.Sprintf("{\"hashes\":%v,\"txs\":%v,\"size\":%v}", 
                                app.hashCount, app.txCount, app.state.Size)
    resInfo.Version = version.ABCIVersion
    resInfo.LastBlockHeight = app.state.Height
    resInfo.LastBlockAppHash = app.state.AppHash
    return
}

func (app *SwarmDAGCountApp) SetOption(req types.RequestSetOption) types.ResponseSetOption {
    return types.ResponseSetOption{}
}

func (app *SwarmDAGCountApp) DeliverTx(tx []byte) types.ResponseDeliverTx {
    fmt.Println("DeliverTx():")
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
    b := make([]byte, 10)
    _, err := rand.Read(b)
    if err != nil {
        fmt.Println("error:", err)
        return types.ResponseDeliverTx{
            Code: code.CodeTypeEncodingError,
            Log: fmt.Sprintf("error creating hash")}
    }

    h := sha1.New()
    h.Write(b)
    currHash := hex.EncodeToString(h.Sum(nil))
    fmt.Println(currHash)
    app.txCount++

    if app.lastHash != "" {
        app.state.store.AddQuad(quad.Make(currHash, "follows", app.lastHash, "ledger"))
    } else {
        app.state.store.AddQuad(quad.Make(currHash, "follows", "genesis", "ledger"))
    }

    app.state.store.AddQuad(quad.Make(strconv.FormatUint(txValue, 10), "value", currHash, "ledger"))
    app.state.store.AddQuad(quad.Make(strconv.FormatInt(app.state.Height, 10), "height", currHash, "ledger"))

    app.lastHash = currHash
    app.state.Size += 1
    app.state.Height += 1

    tags := []cmn.KVPair{
        {Key: []byte("app.creator"), Value: []byte("Cosmoshi Netowoko")},
        {Key: []byte("app.key"), Value: tx8},
    }
    return types.ResponseDeliverTx{Code: code.CodeTypeOK, Tags: tags}
}

func (app *SwarmDAGCountApp) CheckTx(tx []byte) types.ResponseCheckTx {
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

func (app *SwarmDAGCountApp) Commit() (resp types.ResponseCommit) {
    fmt.Println("Commit():")
    app.hashCount++
    if app.txCount == 0 {
        return types.ResponseCommit{}
    }
    hash := make([]byte, 8)
    binary.BigEndian.PutUint64(hash, uint64(app.txCount))
    app.state.AppHash = hash
    return types.ResponseCommit{Data: hash}
}

func (app *SwarmDAGCountApp) Query(reqQuery types.RequestQuery) (resQuery types.ResponseQuery) {
    switch reqQuery.Path {
    case "hash":
        resQuery.Value = []byte(fmt.Sprintf("%v", app.hashCount))
    case "txCount":
        resQuery.Value = []byte(fmt.Sprintf("%v", app.txCount))
    case "countLookup":
       // resQuery.Key = reqQuery.Data
       // resQuery.Value = value
       // TODO: look up "value" of currHash (or lastHash?)
    default:
        resQuery.Log = fmt.Sprintf("Invalid query path. Got %v", reqQuery.Path)
    }

    return 
}

func initializeAndOpenGraph(atLoc string) *cayley.Handle {
    // Initialize the database
    graph.InitQuadStore("bolt", atLoc, nil)

    // Open and use the database
    store, err := cayley.NewGraph("bolt", atLoc, nil)
    if err != nil {
        fmt.Println(err)
    }

    return store
}
