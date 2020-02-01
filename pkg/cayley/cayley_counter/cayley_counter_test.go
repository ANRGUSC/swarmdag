package cayley_counter

import (
    "testing"
    "context"
    "fmt"
    "io/ioutil"
    "os"

    "github.com/tendermint/tendermint/libs/log"
    "github.com/tendermint/tendermint/abci/server"

    "github.com/cayleygraph/cayley"
    . "github.com/cayleygraph/cayley/graph/path"
    _ "github.com/cayleygraph/cayley/graph/kv/bolt"
    "github.com/cayleygraph/cayley/quad"
)

func getTempfileName() string {
    tmpfile, err := ioutil.TempFile("", "example")
    if err != nil {
        fmt.Println(err)
    }

    return tmpfile.Name()
}

func addQuads(store *cayley.Handle) {
    // store.AddQuad(quad.Make("tx1", nil, nil, "ledger"))
    store.AddQuad(quad.Make("1", "value", "tx1", "ledger"))
    store.AddQuad(quad.Make("tx2", "follows", "tx1", "ledger"))
    store.AddQuad(quad.Make("2", "value", "tx2", "ledger"))
    store.AddQuad(quad.Make("tx3", "follows", "tx2", "ledger"))
    store.AddQuad(quad.Make("3", "value", "tx3", "ledger"))
    store.AddQuad(quad.Make("tx4", "follows", "tx3", "ledger"))
    store.AddQuad(quad.Make("4", "value", "tx4", "ledger"))
}

func TestSwarmdagCountApp(t *testing.T) {
    logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))


    app := NewSwarmDAGCountApp("TODO_directory")

    app.SetLogger(logger.With("module", "SwarmDAG"))

    srv, err := server.NewServer("localhost:26658", "socket", app) 
    if err != nil {
        fmt.Println(err)
    }

    srv.SetLogger(logger.With("module", "abci-server")) 
    if err := srv.Start(); err != nil {
        fmt.Println(err)
    }

    resp := app.DeliverTx([]byte{0})
    fmt.Println(resp.Code)
    resp = app.DeliverTx([]byte{1})
    fmt.Println(resp.Code)
    resp = app.DeliverTx([]byte{2})
    fmt.Println(resp.Code)
    resp = app.DeliverTx([]byte{3})
    fmt.Println(resp.Code)
    commit_resp := app.Commit()
    fmt.Println(commit_resp.Data)

    fmt.Println("*** iterating from a tip to the genesis ***")

    p := cayley.StartPath(app.state.store, quad.String(app.lastHash)).FollowRecursive(
            StartMorphism().Out("follows"), 0, nil)
    it, _ := p.BuildIterator().Optimize()
    it, _ = app.state.store.OptimizeIterator(it)
    defer it.Close()

    ctx := context.TODO()
    // While we have items
    for it.Next(ctx) {
        fmt.Println(app.state.store.NameOf(it.Result())) // print it!
    }
    
    if err := it.Err(); err != nil {
        fmt.Println(err)
    }
}

