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

	logging "github.com/op/go-logging"
	"github.com/ANRGUSC/swarmdag/membership"
    "github.com/ANRGUSC/swarmdag/ledger"
    abciserver "github.com/tendermint/tendermint/abci/server"
    "github.com/tendermint/tendermint/libs/service"
    tmlog "github.com/tendermint/tendermint/libs/log"
)

var rootDirStart = os.ExpandEnv("$GOPATH/src/github.com/ANRGUSC/swarmdag/build")
// var ipPrefix = "0.0.0"
var ipPrefix = "192.168.10" // prefix for all containers
var idToIPOffset = 2    // determines IP addr: e.g. node2 has IP  x.x.x.4

type Manager interface {
    NewNetwork(viewID int, membershipID string)
    Init()
}

type manager struct {
    nodeID      int
    log         *logging.Logger
    instances   []*tmn.Node
    abciservers []*service.Service
    stopOld     chan bool
    ledger      *ledger.Ledger
}

func NewManager (nodeID int, log *logging.Logger, ledger *ledger.Ledger) Manager {
    m := &manager {
        nodeID: nodeID,
        log: log,
        stopOld: make(chan bool, 16),
        ledger: ledger,
    }
    return m
}

func (m *manager) Init() {
    go m.initStopService()
}

func stopAll() {
    return
}

func (m *manager) NewNetwork(viewID int, membershipID string) {
    appAddr := fmt.Sprintf("0.0.0.0:200%d", viewID % 100)
    app := ledger.NewABCIApp(m.ledger)
    server := abciserver.NewSocketServer(appAddr, app)

    server.SetLogger(tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout)))
    if err := server.Start(); err != nil {
        fmt.Fprintf(os.Stderr, "error starting socket server: %v\n", err)
        os.Exit(1)
    }
    m.abciservers = append(m.abciservers, &server)

    node, err := m.newTendermint(appAddr, viewID, membershipID)
    if err != nil {
        fmt.Fprintf(os.Stderr, "%v", err)
        os.Exit(2)
    }
    node.Start()

    m.instances = append(m.instances, node)
    m.stopOld <- true
}

func (m *manager) initStopService() {
    var node *tmn.Node
    for  {
        select {
        case <-m.stopOld:
            for len(m.instances) > 1 {
                node, m.instances = m.instances[0], m.instances[1:] 
                m.log.Debug("stopping instance")
                node.Stop()
                node.Wait()
                // TODO: decide on truncation
                // TODO: if a merge reconcile ledger
            }
        }
    }
}

// RPC port 30XX, P2P port 40XX -- where XX = viewID % 100
func (m *manager) newTendermint(appAddr string, viewID int, membershipID string) (*tmn.Node, error) {
    config := cfg.DefaultConfig()
    membershipDir := fmt.Sprintf("tmp/%s/node%d/config", membershipID, m.nodeID)
    config.RootDir = filepath.Dir(filepath.Join(rootDirStart, membershipDir))
    m.log.Debugf("My root dir is %s", config.RootDir)

    nValidators := membership.GetNodeCount(membershipID)
    genVals := make([]types.GenesisValidator, nValidators)
    persistentPeers := make([]string, nValidators)

    for i, nodeID := range membership.GetNodeIDs(membershipID) {
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
                viewID % 100))
    }

    if membership.AmLeader(membershipID) {
        // Write genesis file for all nodes to keep genesis time constant
        genDoc := &types.GenesisDoc{
            ChainID:         "chain-" + membershipID,
            ConsensusParams: types.DefaultConsensusParams(),
            GenesisTime:     tmtime.Now(),
            Validators:      genVals,
        }

        for _, nodeID := range membership.GetNodeIDs(membershipID) {
            nodeDir := filepath.Join(filepath.Dir(config.RootDir), fmt.Sprintf("node%d", nodeID))
            os.MkdirAll(nodeDir + "/config", 0755)
            if err := genDoc.SaveAs(filepath.Join(nodeDir, config.BaseConfig.Genesis)); err != nil {
                return nil, err
            }
        }
    } else {
        m.log.Debug("not leader, waiting for files to generate...")
        time.Sleep(5 * time.Second)
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
    config.RPC.ListenAddress = fmt.Sprintf("tcp://0.0.0.0:300%d", viewID % 100)
    config.P2P.ListenAddress = fmt.Sprintf("tcp://0.0.0.0:400%d", viewID % 100)
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
    logger, err = tmflags.ParseLogLevel("*:debug", logger, "*:debug")
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

