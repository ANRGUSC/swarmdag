package main

import (
    "context"
    "fmt"
    "io/ioutil"
    "log"
    "os"

    "github.com/cayleygraph/cayley"
    "github.com/cayleygraph/cayley/graph"
    "github.com/cayleygraph/cayley/query"
    "github.com/cayleygraph/cayley/query/gizmo"
    _ "github.com/cayleygraph/cayley/graph/kv/bolt"
    "github.com/cayleygraph/cayley/quad"
)

func main() {
    // File for your new BoltDB. Use path to regular file and not temporary in the real world
    tmpdir, err := ioutil.TempDir("", "example")
    if err != nil {
        log.Fatal(err)
    }

    defer os.RemoveAll(tmpdir) // clean up

    // Initialize the database
    err = graph.InitQuadStore("bolt", tmpdir, nil)
    if err != nil {
        log.Fatal(err)
    }

    // Open and use the database
    store, err := cayley.NewGraph("bolt", tmpdir, nil)
    if err != nil {
        log.Fatalln(err)
    }

    store.AddQuad(quad.Make("phrase of the day", "is of course", "Hello BoltDB!", nil))
    store.AddQuad(quad.Make("phrase of the day", "is of course", "second hello", nil))
    store.AddQuad(quad.Make("second hello", "points to", "Hello BoltDB!", "a label"))

    js := gizmo.NewSession(store.QuadStore)

    c := make(chan query.Result, 1)
    go js.Execute(context.TODO(), `g.V().GetLimit(10)`, c, -1)


    for res := range c {
        if err := res.Err(); err != nil {
            fmt.Println("ERROR!")
            continue 
        }
        fmt.Println(res)
        data := res.(*gizmo.Result)
        if data.Val == nil {
            fmt.Println("index is ", data.Tags["id"])
            fmt.Println(store.QuadStore.NameOf(data.Tags["id"]))
        } else {
            switch v := data.Val.(type) {
            case string:
                fmt.Println(v)
            default:
                fmt.Println(v)
            }
        }

    }

    // testpath := cayley.StartPath(store, quad.String("phrase of the day").GetLimit(2))

    // Now we create the path, to get to our data
    // p := cayley.StartPath(store, quad.String("phrase of the day")).Out(quad.String("is of course"))
    // p := cayley.StartPath(store, quad.String("Hello BoltDB!")).In().In()
    p := cayley.StartPath(store, quad.String("phrase of the day")).Out()


    // This is more advanced example of the query.
    // Simpler equivalent can be found in hello_world example.

    // Now we get an iterator for the path and optimize it.
    // The second return is if it was optimized, but we don't care for now.
    fmt.Printf("%#v", p.BuildIterator())
    it, _ := p.BuildIterator().Optimize()


    // Optimize iterator on quad store level.
    // After this step iterators will be replaced with backend-specific ones.
    it, _ = store.OptimizeIterator(it)

    // remember to cleanup after yourself
    defer it.Close()

    ctx := context.TODO()
    // While we have items
    for it.Next(ctx) {
        token := it.Result()                // get a ref to a node (backend-specific)
        value := store.NameOf(token)        // get the value in the node (RDF)
        nativeValue := quad.NativeOf(value) // convert value to normal Go type

        fmt.Println(nativeValue) // print it!
    }
    if err := it.Err(); err != nil {
        log.Fatalln(err)
    }
}
