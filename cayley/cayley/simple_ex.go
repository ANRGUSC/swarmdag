package main

import (
    "fmt"
    "io/ioutil"
    "log"
    "os"

    "github.com/cayleygraph/cayley"
    "github.com/cayleygraph/cayley/graph"
    _ "github.com/cayleygraph/cayley/graph/kv/bolt"
    "github.com/cayleygraph/cayley/quad"
)

func main() {
    // Some globally applicable things
    graph.IgnoreMissing = true
    graph.IgnoreDuplicates = true

    // File for your new BoltDB. Use path to regular file and not temporary in the real world
    t := getTempfileName()
    defer os.Remove(t)                 // clean up
    store := initializeAndOpenGraph("db.boltdb") // initialize the graph

    addQuads(store) // add quads to the graph
    countOuts(store, "robertmeta")
    lookAtOuts(store, "robertmeta")
    lookAtIns(store, "robertmeta")
}

// countOuts ... well, counts Outs
func countOuts(store *cayley.Handle, to string) {
    p := cayley.StartPath(store, quad.Raw("robertmeta")).Out().Count()
    fmt.Printf("countOuts: ")
    p.Iterate(nil).EachValue(store, func(v quad.Value) {
        fmt.Printf("%d\n", quad.NativeOf(v))
    })
    fmt.Printf("========================================\n\n")
}

// lookAtOuts looks at the outbound links from the "to" node
func lookAtOuts(store *cayley.Handle, to string) {
    p := cayley.StartPath(store, quad.Raw("robertmeta")) // start from a single node, but we could start from multiple
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
    cayley.StartPath(store, quad.Raw("robertmeta")).Tag("object").InWithTags([]string{"predicate"}).Tag("subject").Iterate(nil).TagValues(nil, func(m map[string]quad.Value) {
        fmt.Printf("%s <-%s- %s\n", m["object"], m["predicate"], m["subject"])
    })
}

func initializeAndOpenGraph(atLoc string) *cayley.Handle {
    // Initialize the database
    graph.InitQuadStore("bolt", atLoc, nil)

    // Open and use the database
    store, err := cayley.NewGraph("bolt", atLoc, nil)
    if err != nil {
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
    store.AddQuad(quad.MakeRaw("barakmich", "drinks_with", "robertmeta", "demo graph"))
    store.AddQuad(quad.MakeRaw("barakmich", "is_a", "cayley creator", "demo graph"))
    store.AddQuad(quad.MakeRaw("barakmich", "knows", "robertmeta", "demo graph"))
    store.AddQuad(quad.MakeRaw("betawaffle", "knows", "robertmeta", "demo graph"))
    store.AddQuad(quad.MakeRaw("betawaffle", "is_a", "cayley advocate", "demo graph"))
    store.AddQuad(quad.MakeRaw("dennwc", "is_a", "cayley coding machine", "demo graph"))
    store.AddQuad(quad.MakeRaw("dennwc", "knows", "robertmeta", "demo graph"))
    store.AddQuad(quad.MakeRaw("henrocdotnet", "is_a", "cayley doubter", "demo graph"))
    store.AddQuad(quad.MakeRaw("henrocdotnet", "knows", "robertmeta", "demo graph"))
    store.AddQuad(quad.MakeRaw("henrocdotnet", "works_with", "robertmeta", "demo graph"))
    store.AddQuad(quad.MakeRaw("oren", "is_a", "cayley advocate", "demo graph"))
    store.AddQuad(quad.MakeRaw("oren", "knows", "robertmeta", "demo graph"))
    store.AddQuad(quad.MakeRaw("oren", "makes_talks_with", "robertmeta", "demo graph"))
    store.AddQuad(quad.MakeRaw("robertmeta", "is_a", "cayley advocate", "demo graph"))
    store.AddQuad(quad.MakeRaw("robertmeta", "knows", "barakmich", "demo graph"))
    store.AddQuad(quad.MakeRaw("robertmeta", "knows", "betawaffle", "demo graph"))
    store.AddQuad(quad.MakeRaw("robertmeta", "knows", "dennwc", "demo graph"))
    store.AddQuad(quad.MakeRaw("robertmeta", "knows", "dennwc", "demo graph")) // purposeful dup, will be ignored
    store.AddQuad(quad.MakeRaw("robertmeta", "knows", "dennwc", "demo graph")) // purposeful dup, will be ignored
    store.AddQuad(quad.MakeRaw("robertmeta", "knows", "dennwc", "demo graph")) // purposeful dup, will be ignored
    store.AddQuad(quad.MakeRaw("robertmeta", "knows", "henrocdotnet", "demo graph"))
    store.AddQuad(quad.MakeRaw("robertmeta", "knows", "oren", "demo graph"))
    store.AddQuad(quad.MakeRaw("cayley advocate", "is_a", "hard job without docs", "demo graph"))
    store.AddQuad(quad.MakeRaw("cayley coding machine", "is_a", "amazing gig", "demo graph"))
}