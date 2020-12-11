package partition

import (
	"fmt"
	"os"
	"io"
	"path/filepath"
	"strings"
    "time"

	cfg "github.com/tendermint/tendermint/config"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	"github.com/tendermint/tendermint/libs/log"
	tmn "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"
	tmtime "github.com/tendermint/tendermint/types/time"
    "github.com/libp2p/go-libp2p-core/peer"

	logging "github.com/op/go-logging"
    "github.com/ANRGUSC/swarmdag/ledger"
    abciserver "github.com/tendermint/tendermint/abci/server"
    "github.com/tendermint/tendermint/libs/service"
    tmlog "github.com/tendermint/tendermint/libs/log"
)

var rootDirStart = os.ExpandEnv("$GOPATH/src/github.com/ANRGUSC/swarmdag/build")
// var ipPrefix = "0.0.0"
var ipPrefix = "192.168.10" // prefix for all containers
var idToIPOffset = 2    // determines IP addr: e.g. node2 has IP  x.x.x.4 , 2 works for docker, core not yet finished
var tmLogLevel = "main:info,state:info,*:error" // tendermint/abci log level

type MembershipInfo struct {
    ViewID          int
    ChainID         string
    MemberNodeIDs   []int
    Libp2pIDs       []peer.ID
    AmLeader        bool
    ReconcileNeeded bool
}

type Manager interface {
    NewNetwork(info MembershipInfo)
}

type manager struct {
    nodeID      int
    log         *logging.Logger
    instances   map[*tmn.Node]service.Service
    dag         *ledger.DAG
}

func NewManager (nodeID int, log *logging.Logger, dag *ledger.DAG) Manager {
    m := &manager {
        nodeID: nodeID,
        log: log,
        instances: make(map[*tmn.Node]service.Service),
        dag: dag,
    }
    return m
}

// Membership manager should be able to call this on new view installation
func (m *manager) NewNetwork(info MembershipInfo) {
    if info.ReconcileNeeded {
        ledger.Reconcile(info.Libp2pIDs, m.dag, m.log)
    }

    // Stop all old instances for a clean start
    m.stopOldNetworks()

    // start ABCI app
    appAddr := fmt.Sprintf("0.0.0.0:200%d", info.ViewID % 100)
    app := ledger.NewABCIApp(m.dag, m.log, info.ChainID)
    server := abciserver.NewSocketServer(appAddr, app)
    logger := tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout))
    logger, _ = tmflags.ParseLogLevel(tmLogLevel, logger, cfg.DefaultLogLevel())
    server.SetLogger(logger)
    if err := server.Start(); err != nil {
        m.log.Fatalf("error starting socket server: %v\n", err)
    }

    // start tendermint
    node, err := m.newTendermint(appAddr, info)
    if err != nil {
        m.log.Fatalf("error starting new tendermint net: %v", err)
    }

    node.Start()
    m.instances[node] = server

    m.dag.Idx.InsertChainID(info.ChainID)
}

func (m *manager) stopOldNetworks() {

    for node, server := range m.instances {
        m.log.Infof("Stopping network instance %p", node)
        node.Stop()
        node.Wait() //TODO: does this wait for a block in a round to finish?
        delete(m.instances, node)
        // TODO: decide on truncation (essentially dequeue block in the queue set inside the abciapp)


        // TODO: if a merge reconcile ledger (maybe through a passed flag by membership)


        // - gossip mID with a hash ofallTxs
        // - if discrepancy found, gossip back a request with LAST block you know of
        //   in that membership id. a response should be given with all missing blocks
        if err := server.Stop(); err != nil {
            m.log.Fatalf("error starting socket server: %v\n", err)
        }
    }
}

// RPC port 30XX, P2P port 40XX -- where XX = MembershipInfo.ViewID % 100
func (m *manager) newTendermint(appAddr string, info MembershipInfo) (*tmn.Node, error) {
    config := cfg.DefaultConfig()
    membershipDir := fmt.Sprintf("tmp/chain-%s/node%d/config", info.ChainID, m.nodeID)
    config.RootDir = filepath.Dir(filepath.Join(rootDirStart, membershipDir))
    m.log.Debugf("My root dir is %s", config.RootDir)

    nValidators := len(info.MemberNodeIDs)
    genVals := make([]types.GenesisValidator, nValidators)
    persistentPeers := make([]string, nValidators)

    for i, nodeID := range info.MemberNodeIDs {
        nodeName := fmt.Sprintf("node%d", nodeID)
        nodeDir := filepath.Join(rootDirStart, "templates", nodeName)

        // Gather validator address in group for genesis file
        pvKeyFile := filepath.Join(nodeDir, config.BaseConfig.PrivValidatorKey)
        pvStateFile := filepath.Join(nodeDir, config.BaseConfig.PrivValidatorState)
        pv := privval.LoadFilePV(pvKeyFile, pvStateFile)

        pubKey, err := pv.GetPubKey()
        if err != nil {
            return nil, err
        }

        // Used for leader to generate genesis files
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

        persistentPeers[i] = p2p.IDAddressString(nodeKey.ID(),
            fmt.Sprintf("%s.%d:400%d", ipPrefix, nodeID + idToIPOffset,
                info.ViewID % 100))
    }

    if info.AmLeader {
        m.log.Debug("I am the leader, generating genesis and config files...")
        genDoc := &types.GenesisDoc{
            ChainID:         "chain-" + info.ChainID,
            ConsensusParams: types.DefaultConsensusParams(),
            GenesisTime:     tmtime.Now(),
            Validators:      genVals,
        }

        for _, nodeID := range info.MemberNodeIDs {
            nodeDir := filepath.Join(filepath.Dir(config.RootDir), fmt.Sprintf("node%d", nodeID))
            os.MkdirAll(nodeDir + "/config", 0775)
            if err := genDoc.SaveAs(filepath.Join(nodeDir, config.BaseConfig.Genesis)); err != nil {
                return nil, err
            }
        }
    } else {
        // Wait for leader to create directories
        retries := 0
        genesisFile := filepath.Join(
            filepath.Dir(config.RootDir),
            fmt.Sprintf("node%d", m.nodeID),
            "config/genesis.json",
        )
        for {
            if _, err := os.Stat(genesisFile); os.IsNotExist(err) {
                time.Sleep(500 * time.Millisecond)
                retries += 1
                if retries > 20 {
                    panic("Directories not created by leader, exiting...")
                }
            } else {
                m.log.Debug("Tendermint directories created")
                break
            }
        }
    }

    err := os.MkdirAll(filepath.Join(config.RootDir, "config"), 0775)
    if err != nil {
        return nil, err
    }
    err = os.MkdirAll(filepath.Join(config.RootDir, "data"), 0775)
    if err != nil {
        return nil, err
    }

    // containers
    config.RPC.ListenAddress = fmt.Sprintf("tcp://0.0.0.0:300%d", info.ViewID % 100)
    config.P2P.ListenAddress = fmt.Sprintf("tcp://0.0.0.0:400%d", info.ViewID % 100)
    config.P2P.AddrBookStrict = false
    config.P2P.AllowDuplicateIP = true
    config.P2P.PersistentPeers = strings.Join(persistentPeers, ",")
    config.Consensus.WalPath = filepath.Join(config.RootDir, "/data/cs.wal/wal")
    config.Consensus.CreateEmptyBlocks = false
    config.Moniker = fmt.Sprintf("node%d", m.nodeID)

    // copy template files
    templateDir := filepath.Join(rootDirStart,
        fmt.Sprintf("/templates/node%d", m.nodeID))
    copyFile(filepath.Join(templateDir, "config/node_key.json"),
        filepath.Join(config.RootDir, "config/node_key.json"))
    copyFile(filepath.Join(templateDir, "config/priv_validator_key.json"),
        filepath.Join(config.RootDir, "config/priv_validator_key.json"))
    copyFile(filepath.Join(templateDir, "data/priv_validator_state.json"),
        filepath.Join(config.RootDir, "data/priv_validator_state.json"))

    // create logger
    logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))
    logger, err = tmflags.ParseLogLevel(tmLogLevel, logger, cfg.DefaultLogLevel())
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
    node, err := tmn.NewNode(
        config,
        pv,
        nodeKey,
        proxy.NewRemoteClientCreator(appAddr, "socket", true),
        tmn.DefaultGenesisDocProviderFunc(config),
        tmn.DefaultDBProvider,
        tmn.DefaultMetricsProvider(config.Instrumentation),
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

