package ledger

import (
    "io/ioutil"
    "github.com/cayleygraph/cayley"
    "github.com/cayleygraph/cayley/graph"
)

type Ledger struct {
    DB *cayley.Handle 
}

func NewLedger() *Ledger {
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

    return &Ledger{
        DB: db,
    }
}

