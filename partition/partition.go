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
)

var rootDirStart = os.ExpandEnv("$GOPATH/src/github.com/ANRGUSC/swarmdag/build")
var ipPrefix = "192.168.10" // prefix for all containers

type Manager interface {
    NewNetwork(viewID int, membershipID string)
}

type manager struct {
    nodeID      int
    log         *logging.Logger
    instances   []*tmn.Node
    abciservers []*service.Service
    stopOld     chan bool
}

func NewManager (nodeID int, log *logging.Logger) Manager {
    m := &manager {
        nodeID: nodeID,
        log: log,
        stopOld: make(chan bool, 16),
    }

    return m
}

func stopAll() {
    return
}

func (m *manager) newLedgerApp() {
    // File for your new BoltDB. Use path to regular file and not temporary in the real world
    tmpdir, err := ioutil.TempDir("", "cayleyFiles")
    if err != nil {
        panic(err)
    }

    defer os.RemoveAll(tmpdir) // clean up

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

    app := ledger.NewApplication(db)


    server := abciserver.NewSocketServer(fmt.Sprintf("0.0.0.0:200%d", viewID % 100), app)

    server.SetLogger(m.log)
    if err := server.Start(); err != nil {
        fmt.Fprintf(os.Stderr, "error starting socket server: %v\n", err)
        os.Exit(1)
    }

    m.abciservers = append(m.abciservers, app)
}

func (m *manager) NewNetwork(viewID int, membershipID string) {


    node, err := m.newTendermint(fmt.Sprintf("0.0.0.0:200%d", viewID % 100), 
        viewID, membershipID)
    if err != nil {
        fmt.Fprintf(os.Stderr, "%v", err)
        os.Exit(2)
    }

    m.instances = append(m.instances, node)

    m.stopOld <- true
}

func (m *manager) tmStopService() {
    var node *tmn.Node
    for  {
        select {
        case <-m.stopOld:
            for len(m.instances) > 1 {
                node, m.instances = m.instances[0], m.instances[1:] 
                node.Stop()
                node.Wait()
                // TODO: decide on truncation
                // TODO: if a merge reconcile ledger
            }
        }
    }
}

// func (m * manager) newABCIA

// RPC port 30XX, P2P port 40XX, XX = viewID % 100
func (m *manager) newTendermint(appAddr string, viewID int, membershipID string) (*tmn.Node, error) {
    config := cfg.DefaultConfig()

    membershipDir := fmt.Sprintf("tmp/%s/node%d/config", membershipID, m.nodeID)
    config.RootDir = filepath.Dir(filepath.Join(rootDirStart, membershipDir))

    nValidators := membership.GetNodeCount(membershipID)
    genVals := make([]types.GenesisValidator, nValidators)
    persistentPeers := make([]string, nValidators)

    for i, nodeID := range membership.GetNodeIDs(membershipID) {
        nodeName := fmt.Sprintf("node%d", nodeID)
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

        persistentPeers[i] = p2p.IDAddressString(nodeKey.ID(), 
            fmt.Sprintf("%s.%s:400%d", ipPrefix, nodeID, viewID % 100))
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
    config.Moniker = fmt.Sprintf("node%d", m.nodeID)

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
    }

    // copy template files
    templateDir := filepath.Join(rootDirStart, 
        fmt.Sprintf("/templates/node%d", m.nodeID))
    m.log.Debug(templateDir)
    copyFile(filepath.Join(templateDir, "config/node_key.json"), 
        filepath.Join(config.RootDir, "config/node_key.json"))
    copyFile(filepath.Join(templateDir, "config/priv_validator_key.json"), 
        filepath.Join(config.RootDir, "config/priv_validator_key.json"))
    copyFile(filepath.Join(templateDir, "data/priv_validator_state.json"), 
        filepath.Join(config.RootDir, "data/priv_validator_state.json"))

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

