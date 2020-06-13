package partition

import (
    "fmt"
    "os"
    "io"
    "path/filepath"
    "strings"

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

    logging "github.com/op/go-logging"
)

var rootDirStart = os.ExpandEnv("$GOPATH/src/github.com/ANRGUSC/swarmdag/cmd/partition_manager/")

type partitionManager struct {
    log *logging.Logger
}

var pm *partitionManager

func StartPartitionManager (log *logging.Logger) {
    pm := &partitionManager {
        log: log,
    }

    _ = pm
}

func Start() {
    // abci port first
    node, err := newTendermint(fmt.Sprintf("0.0.0.0:26658"), "socket", 
        os.ExpandEnv(rootDirStart))
    if err != nil {
        fmt.Fprintf(os.Stderr, "%v", err)
        os.Exit(2)
    }

    node.Start()
}

func Stop(node *nm.Node) {
    node.Stop()
    node.Wait()
}

func newTendermint(appAddr, transport, rootDir string) (*nm.Node, error) {
    // read config
    config := cfg.DefaultConfig()

    groupDir := fmt.Sprintf("tmp/%s/node%d/config", getMembershipID(), getNodeID())
    config.RootDir = filepath.Dir(rootDirStart + groupDir) 

    // TODO: get actual num validators in current membership
    nValidators := 2
    genVals := make([]types.GenesisValidator, nValidators)
    persistentPeers := make([]string, nValidators)

    // TODO: make this loop through nodes only in current membership
    for i := 0; i < nValidators; i++ {
        nodeName := fmt.Sprintf("node%d", i)
        nodeDir := filepath.Join(rootDirStart, "templates", nodeName)

        // Gather validator address for genesis file
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
            Name:    nodeName,
        }

        // Gather persistent peer addresses
        nodeKey, err := p2p.LoadNodeKey(nodeDir + "/config/node_key.json")
        if err != nil {
            return nil, err
        }

        // TODO: set addrs from list
        persistentPeers[i] = p2p.IDAddressString(nodeKey.ID(), fmt.Sprintf("0.0.0.0:%d", 26600 + i))   
    }

    err := os.MkdirAll(filepath.Join(config.RootDir, "config"), 0755)
    if err != nil {
        return nil, err
    }
    err = os.MkdirAll(filepath.Join(config.RootDir, "data"), 0755)
    if err != nil {
        return nil, err
    }

    // containers
    config.RPC.ListenAddress = "tcp://0.0.0.0:26657"
    config.P2P.ListenAddress = "tcp://0.0.0.0:26656"

    config.P2P.AddrBookStrict = false
    config.P2P.AllowDuplicateIP = true
    config.P2P.PersistentPeers = strings.Join(persistentPeers, ",")
    config.Consensus.WalPath = config.RootDir + "/data/cs.wal/wal"
    config.Moniker = fmt.Sprintf("node%d", getNodeID())

    if amLeader() {
        // Write genesis file for all nodes to keep genesis time constant
        genDoc := &types.GenesisDoc{
            ChainID:         "chain-" + getChainID(),
            ConsensusParams: types.DefaultConsensusParams(),
            GenesisTime:     tmtime.Now(),
            Validators:      genVals,
        }

        for i := 0; i < nValidators; i++ {
            nodeDir := filepath.Join(filepath.Dir(config.RootDir), fmt.Sprintf("node%d", i)) 
            fmt.Println(nodeDir)
            os.MkdirAll(nodeDir + "/config", 0755)
            if err := genDoc.SaveAs(filepath.Join(nodeDir, config.BaseConfig.Genesis)); err != nil {
                return nil, err
            }
        }
    }

    // copy template files
    templateDir := config.RootDir + fmt.Sprintf("/templates/node%d", getNodeID())
    copyFile(templateDir + "/config/node_key.json", 
        config.RootDir + "/config/node_key.json")
    copyFile(templateDir + "/config/priv_validator_key.json", 
        config.RootDir + "/config/priv_validator_key.json")
    copyFile(templateDir + "/data/priv_validator_state.json", 
        config.RootDir + "/data/priv_validator_state.json")

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
        proxy.NewRemoteClientCreator(appAddr, transport, true),
        nm.DefaultGenesisDocProviderFunc(config),
        nm.DefaultDBProvider,
        nm.DefaultMetricsProvider(config.Instrumentation),
        logger)
    if err != nil {
        return nil, fmt.Errorf("failed to create new Tendermint node: %w", err)
    }

    return node, nil
}

func copyFile(src, dst string) error {
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

func getMembershipID() string {
    // TODO: implement actual function that gets mid
    return "kGdVaN"
    // return tmrand.Str(6)
}

func getNodeID() int {
    // TODO: implement me
    return 1
}

func amLeader() bool {
    // TODO
    if getNodeID() == 0 {
        return true
    }
    return false
}

func getChainID() string {
    // TODO
    tmrand.Str(6)
    return "kGdVaN"
    // return tmrand.Str(6)
}

