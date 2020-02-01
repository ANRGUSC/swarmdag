package main

import (
    "fmt"
    "os"

    tmos "github.com/tendermint/tendermint/libs/os"
    "github.com/tendermint/tendermint/libs/log"

    "github.com/tendermint/tendermint/abci/server"
    . "github.com/ANRGUSC/swarmdag/pkg/cayley/cayley_counter"
)

func main() {
    logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

    app := NewSwarmDAGCountApp("TODO_directory")

    app.SetLogger(logger.With("module", "swarmdag"))

    srv, err := server.NewServer("localhost:26658", "socket", app) 
    if err != nil {
        fmt.Println(err)
    }

    srv.SetLogger(logger.With("module", "abci-server")) 
    if err := srv.Start(); err != nil {
        fmt.Println(err)
    }

    // Wait forever
    tmos.TrapSignal(func() {
        // Cleanup
        srv.Stop()
    })
}
