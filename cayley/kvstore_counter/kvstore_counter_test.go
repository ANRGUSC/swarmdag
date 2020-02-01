package kvstore_counter

import (
    // "bytes"
    "encoding/binary"
    "encoding/hex"
    "context"
    "fmt"
    "io/ioutil"
    "os"
    "strconv"

    "crypto/rand"
    "crypto/sha1"
    cmn "github.com/tendermint/tendermint/libs/common"
    "github.com/tendermint/tendermint/libs/log"

    "github.com/tendermint/tendermint/abci/example/code"
    "github.com/tendermint/tendermint/abci/server"
    "github.com/tendermint/tendermint/abci/types"
    "github.com/tendermint/tendermint/version"
)

func TestSwarmdagKVstoreApp() {
    //Test implementation incomplete
    // logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

    // dir, err := ioutil.TempDir("./", "kvstore_db")
    // if err != nil {
    //     fmt.Println(err)
    // }

    // app := NewSwarmdagCountApp(dir)

    // app.SetLogger(logger.With("module", "kvstore"))

    // srv, err := server.NewServer("localhost:26658", "socket", app) 
    // if err != nil {
    //     fmt.Println(err)
    // }

    // srv.SetLogger(logger.With("module", "abci-server")) 
    // if err := srv.Start(); err != nil {
    //     fmt.Println(err)
    // }

    // app.DeliverTx([]byte{1, 0, 0})

    // // make sure query is fine
    // resQuery := app.Query(types.RequestQuery{
    //     Path: "/store",
    //     Data: []byte{1, 2, 3},
    // })
    // // assert.Equal(code.CodeTypeOK, resQuery.Code)
    // fmt.Println(resQuery.Code)
    // // assert.Equal(value, string(resQuery.Value))
    // fmt.Println(resQuery.Value)

    // // make sure proof is fine
    // resQuery = app.Query(types.RequestQuery{
    //     Path:  "/store",
    //     Data:  []byte("103"),
    //     Prove: true,
    // })
    // // assert.EqualValues(code.CodeTypeOK, resQuery.Code)
    // fmt.Println(resQuery.Code)
    // // assert.Equal(value, string(resQuery.Value))
    // fmt.Println(resQuery.Value)

    // // Wait forever
    // cmn.TrapSignal(func() {
    //     // Cleanup
    //     srv.Stop()
    // })
}