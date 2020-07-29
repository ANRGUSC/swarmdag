package ledger

import (
    "io/ioutil"
    "github.com/cayleygraph/cayley"
    "github.com/cayleygraph/cayley/graph"
    "github.com/cayleygraph/quad"
    logging "github.com/op/go-logging"
)

type Ledger struct {
    DB *cayley.Handle
    log *logging.Logger
}

type Transaction struct {
    Hash        string `json:"hash" quad:"hash"`
    ParentHash  string `json:"parentHash" quad:"parentHash"`
    Timestamp   int64  `json:"timestamp"  quad:"timestap"`
    Key         string `json:"key" quad:"key"`
    Value       string `json:"value" quad:"value"`
}

func NewLedger(log *logging.Logger) *Ledger {
    // File for your new BoltDB. Use path to regular file and not temporary in the real world
    tmpdir, err := ioutil.TempDir("", "cayleyFiles")
    if err != nil {
        panic(err)
    }

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

    ledger := &Ledger{
        DB: db,
        log: log,
    }

    genesis := Transaction{
        Hash:      "",
        ParentHash:  "",
        Timestamp: 0,
        Key:       "Genesis",
        Value:     "Genesis",
    }

    genesis.SetHash()
    ledger.insertTx(&genesis, "")


    LatestTransaction = (&genesis)
    genesis.Print()

    return ledger
}

func (ledger *Ledger) insertTx (tx *Transaction, parentHash string) {
    t := cayley.NewTransaction()
    t.AddQuad(quad.Make(tx.Hash, "child_of", parentHash, nil))
    t.AddQuad(quad.Make(tx.Hash, "timestamp", tx.Timestamp, nil))
    t.AddQuad(quad.Make(tx.Hash, "key", tx.Key, nil))
    t.AddQuad(quad.Make(tx.Hash, "value", tx.Value, nil))
    err := ledger.DB.ApplyTransaction(t)
    if err != nil {
        ledger.log.Fatal(err)
    }
}

