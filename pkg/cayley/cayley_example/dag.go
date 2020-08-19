package main

import (
	"strings"
	"context"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	. "github.com/cayleygraph/cayley/graph/path"
	_ "github.com/cayleygraph/cayley/graph/kv/bolt"
	"github.com/cayleygraph/cayley/quad"
)

func main() {
    // Some globally applicable things
    graph.IgnoreMissing = true
    graph.IgnoreDuplicates = true

    store := initializeAndOpenGraph("db.boltdb") // initialize the graph

    addQuads(store) // add quads to the graph
    countOuts(store, "tx1")
    lookAtOuts(store, "tx1")
    lookAtIns(store, "tx1")

    fmt.Println("*** iterator printout below ***")

    p := cayley.StartPath(store, quad.String("tx1")).FollowRecursive(
            StartMorphism().In("follows"), 0, nil).
            HasReverse(quad.String("membershipID"))
    it, _ := p.BuildIterator().Optimize()
    defer it.Close()

    ctx := context.TODO()
    // While we have items
    for it.Next(ctx) {
        fmt.Println(store.NameOf(it.Result())) // print it!
    }
    txHash := strings.Trim(store.NameOf(it.Result()).String(), "\"")
    p = cayley.StartPath(store, quad.String(txHash)).
        In(quad.String("membershipID"))
    mid, _ := p.Iterate(nil).FirstValue(nil)
    membershipID := strings.Trim(mid.String(), "\"")
    fmt.Println("mid is " + membershipID)




    if err := it.Err(); err != nil {
        log.Fatalln(err)
    }

    fmt.Println("Make sure to delete db.boltdb/")
}

// countOuts ... well, counts Outs
func countOuts(store *cayley.Handle, to string) {
    p := cayley.StartPath(store, quad.Raw(to)).Out().Count()
    fmt.Printf("countOuts: ")
    p.Iterate(nil).EachValue(store, func(v quad.Value) {
        fmt.Printf("%d\n", quad.NativeOf(v))
    })
    fmt.Printf("========================================\n\n")
}

// lookAtOuts looks at the outbound links from the "to" node
func lookAtOuts(store *cayley.Handle, to string) {
    p := cayley.StartPath(store, quad.Raw(to)) // start from a single node, but we could start from multiple
    // this gives us a path with all the output predicates from our starting point
    p = p.Tag("subject").OutWithTags([]string{"predicate"}).Tag("object")
    fmt.Printf("lookAtOuts: subject -predicate-> object\n")
    fmt.Printf("========================================\n")
    p.Iterate(nil).TagValues(nil, func(m map[string]quad.Value) {
        fmt.Printf("%s -%s-> %s\n", m["subject"], m["predicate"], m["object"])
    })
}

// lookAtIns looks at the inbound links to the "to" node
func lookAtIns(store *cayley.Handle, to string) {
    fmt.Printf("\nlookAtIns: object <-predicate- subject\n")
    fmt.Printf("========================================\n")
    cayley.StartPath(store, quad.Raw(to)).Tag("object").InWithTags([]string{"predicate"}).Tag("subject").Iterate(nil).TagValues(nil, func(m map[string]quad.Value) {
        fmt.Printf("%s <-%s- %s\n", m["object"], m["predicate"], m["subject"])
    })
}

func traverseDAG(store *cayley.Handle, to string) {
    // p := cayley.StartPath(store, quad.Raw("robertmeta")) // start from a single node, but we could start from multiple
}

func initializeAndOpenGraph(atLoc string) *cayley.Handle {
    // Initialize the database
    graph.InitQuadStore("bolt", atLoc, nil)

    // Open and use the database
    store, err := cayley.NewGraph("bolt", atLoc, nil)
    if err != nil{ 
        log.Fatalln(err)
    }

    return store
}

func getTempfileName() string {
    tmpfile, err := ioutil.TempFile("", "example")
    if err != nil {
        log.Fatal(err)
    }

    return tmpfile.Name()
}

func addQuads(store *cayley.Handle) {
    store.AddQuad(quad.Make("tx1", "follows", "genesis_blocK", "ledger"))
    store.AddQuad(quad.Make("1", "value", "tx1", "ledger"))
    store.AddQuad(quad.Make("tx2", "follows", "tx1", "ledger"))
    store.AddQuad(quad.Make("2", "value", "tx2", "ledger"))
    store.AddQuad(quad.Make("tx3", "follows", "tx2", "ledger"))
    store.AddQuad(quad.Make("3", "value", "tx3", "ledger"))
    store.AddQuad(quad.Make("tx4", "follows", "tx3", "ledger"))
    store.AddQuad(quad.Make("4", "value", "tx4", "ledger"))
    store.AddQuad(quad.Make("abcdef", "membershipID", "tx1", "ledger"))
    store.AddQuad(quad.Make("abcdef", "membershipID", "tx2", "ledger"))
    store.AddQuad(quad.Make("abcdef", "membershipID", "tx4", "ledger"))
    store.AddQuad(quad.Make("myKey", "key", "tx4", "ledger"))
}   