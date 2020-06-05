package main

import (
    "flag"
    "fmt"
    "os"
    "io"
    "os/signal"
    "path/filepath"
    "syscall"

    // abci "github.com/tendermint/tendermint/abci/types"
    cfg "github.com/tendermint/tendermint/config"
    tmflags "github.com/tendermint/tendermint/libs/cli/flags"
    "github.com/tendermint/tendermint/libs/log"
    nm "github.com/tendermint/tendermint/node"
    "github.com/tendermint/tendermint/p2p"
    "github.com/tendermint/tendermint/privval"
    "github.com/tendermint/tendermint/proxy"
    "github.com/tendermint/tendermint/types"
    tmtime "github.com/tendermint/tendermint/types/time"
    tmrand "github.com/tendermint/tendermint/libs/rand"
    // "github.com/davecgh/go-spew/spew"
)

var rootDirStart = os.ExpandEnv("$GOPATH/src/github.com/ANRGUSC/swarmdag/cmd/partition_manager/")

func main() {
    flag.Parse()

    node, err := newTendermint("127.0.0.1:26658", "socket", os.ExpandEnv(rootDirStart))
    if err != nil {
        fmt.Fprintf(os.Stderr, "%v", err)
        os.Exit(2)
    }

    node.Start()
    defer func() {
        node.Stop()
        node.Wait()
    }()

    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    <-c
    os.Exit(0)
}


func newTendermint(addr, transport, rootDir string) (*nm.Node, error) {
    // read config
    config := cfg.DefaultConfig()
    config.RootDir = filepath.Dir(rootDirStart + "/t1/node0/config") 

    nValidators := 1
    genVals := make([]types.GenesisValidator, nValidators)

    for i := 0; i < nValidators; i++ {
        // TODO: set nodeDirName using membership list
        nodeDirName := fmt.Sprintf("node%d", i)
        nodeDir := filepath.Join(rootDirStart, "mytestnet", nodeDirName)


        pvKeyFile := filepath.Join(nodeDir, config.BaseConfig.PrivValidatorKey)
        pvStateFile := filepath.Join(nodeDir, config.BaseConfig.PrivValidatorState)
        pv := privval.LoadFilePV(pvKeyFile, pvStateFile)

        pubKey, err := pv.GetPubKey()
        if err != nil {
            return nil, err
        }
        genVals[i] = types.GenesisValidator{
            Address: pubKey.Address(),
            PubKey:  pubKey,
            Power:   1,
            Name:    nodeDirName,
        }
    }

    genDoc := &types.GenesisDoc{
        ChainID:         "chain-" + tmrand.Str(6),
        ConsensusParams: types.DefaultConsensusParams(),
        GenesisTime:     tmtime.Now(),
        Validators:      genVals,
    }

    err := os.MkdirAll(filepath.Join(config.RootDir, "config"), 0755)
    if err != nil {
        _ = os.RemoveAll(rootDirStart)
        return nil, err
    }
    err = os.MkdirAll(filepath.Join(config.RootDir, "data"), 0755)
    if err != nil {
        _ = os.RemoveAll(rootDirStart)
        return nil, err
    }

    // TODO: copy node_key.json, priv_validator_key.json, priv_validator_state.json
    CopyFile(rootDirStart + "/mytestnet/node0/config/node_key.json", config.RootDir + "/config/node_key.json")
    CopyFile(rootDirStart + "/mytestnet/node0/config/priv_validator_key.json", config.RootDir + "/config/priv_validator_key.json")
    CopyFile(rootDirStart + "/mytestnet/node0/data/priv_validator_state.json", config.RootDir + "/data/priv_validator_state.json")

    // create logger
    logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))
    logger, err = tmflags.ParseLogLevel(config.LogLevel, logger, cfg.DefaultLogLevel())
    if err != nil {
        return nil, fmt.Errorf("failed to parse log level: %w", err)
    }

    // read private validator
    pv := privval.LoadFilePV(
        config.PrivValidatorKeyFile(),
        config.PrivValidatorStateFile(),
    )

    // read node key
    nodeKey, err := p2p.LoadNodeKey(config.NodeKeyFile())
    if err != nil {
        return nil, fmt.Errorf("failed to load node's key: %w", err)
    }

    // create node
    node, err := nm.NewNode(
        config,
        pv,
        nodeKey,
        proxy.NewRemoteClientCreator(addr, transport, true),
        func() (*types.GenesisDoc, error) {
            return genDoc, nil
        },
        nm.DefaultDBProvider,
        nm.DefaultMetricsProvider(config.Instrumentation),
        logger)
    if err != nil {
        return nil, fmt.Errorf("failed to create new Tendermint node: %w", err)
    }

    return node, nil
}

func CopyFile(src, dst string) error {
    srcFile, err := os.Open(src)
    if err != nil {
        return err
    }
    defer srcFile.Close()
 
    dstFile, err := os.Create(dst)
    if err != nil {
        return err
    }
    defer dstFile.Close()
 
    _, err = io.Copy(dstFile, srcFile)
    if err != nil {
        return err
    }
 
    err = dstFile.Sync()
    if err != nil {
        return err
    }

    return dstFile.Close()
}