package partition

import (
	"errors"
	"fmt"
	"os"
	"io"
	"path/filepath"
	"strings"
    "strconv"
	"time"

	cfg "github.com/tendermint/tendermint/config"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	tmn "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"
	tmtime "github.com/tendermint/tendermint/types/time"
	"github.com/libp2p/go-libp2p-core/peer"
	dbm "github.com/tendermint/tm-db"

	logging "github.com/op/go-logging"
	"github.com/ANRGUSC/swarmdag/ledger"
	abciserver "github.com/tendermint/tendermint/abci/server"
	"github.com/tendermint/tendermint/libs/service"
	tmlog "github.com/tendermint/tendermint/libs/log"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

const (
    // ipPrefix = "0.0.0"
    ipPrefix = "192.168.10" // prefix for all containers
    tmLogLevel = "main:info,state:info,*:error" // tendermint/abci log level
    abciAppAddr = "0.0.0.0:20000"
    coreDir = "/home/jasonatran/go/src/github.com/ANRGUSC/swarmdag/build"
    folderPollInterval = 500 * time.Millisecond
    folderPollRetries = 480 // 480 * 500ms is 4 min
)

var rootDirStart string
var dockerDir = os.ExpandEnv("$GOPATH/src/github.com/ANRGUSC/swarmdag/build")

type NetworkInfo struct {
    ViewID          int
    ChainID         string
    MemberNodeIDs   []int
    Libp2pIDs       []peer.ID
    AmLeader        bool
    ReconcileNeeded bool
}

type Manager interface {
    NewNetwork(info NetworkInfo) error
}

type manager struct {
    nodeID      int
    log         *logging.Logger
    instances   map[*Node]service.Service
    dag         *ledger.DAG
    logfd       *os.File
    idToIPOffset int
}

// Wrapper around Tendermint node's closeable resources (database file desc.)
type Node struct {
    *tmn.Node
    closers []interface{
        Close() error
    }
}

func DBProvider(ID string, backendType dbm.BackendType, dbDir string) dbm.DB {
    return dbm.NewDB(ID, backendType, dbDir)
}

// Since Tendermint doesn't close its DB connections
func (n *Node) DBProvider(ctx *tmn.DBContext) (dbm.DB, error) {
    db := DBProvider(ctx.ID, dbm.BackendType(ctx.Config.DBBackend), ctx.Config.DBDir())
    n.closers = append(n.closers, db)
    return db, nil
}

func (n *Node) Close() {
    for _, closer := range n.closers {
        closer.Close()
    }
}

func (m *manager) createDoneFile(nodeIDs []int) {
    p := filepath.Join(coreDir, "partition_done", strconv.Itoa(m.nodeID) + ".done")
    os.MkdirAll(filepath.Dir(p), 0755)
    f, _ := os.OpenFile(
        p,
        os.O_CREATE|os.O_RDWR,
        0766,
    )
    for _, i := range nodeIDs {
        f.WriteString(strconv.Itoa(i))
        f.Write([]byte("\n"))
    }
    f.Close()
}


func NewManager (nodeID int, log *logging.Logger, dag *ledger.DAG, orchestrator string) Manager {
    var offset int
    switch orchestrator {
    case "core":
        offset = 1
        rootDirStart = coreDir
    case "docker":
        offset = 2  // Bridge takes up x.x.x.1, start IP addrs at x.x.x.2
        rootDirStart = dockerDir
    default:
        log.Critical("NewManager(): invalid orchestrator")
    }
    m := &manager {
        nodeID: nodeID,
        log: log,
        instances: make(map[*Node]service.Service, 8),
        dag: dag,
        idToIPOffset: offset,
    }
    return m
}

// To be called by Membership Manager upon view installation
func (m *manager) NewNetwork(info NetworkInfo) error {
    var c0, c1 string
    var ts int64

    // stop all Txs to allow reconciliation process
    if len(m.instances) > 0 {
        c, _ := rpchttp.New("tcp://0.0.0.0:30000", "")
        cmd := []byte("disableTx")
        _, err := c.ABCIQuery("", cmd)
        if err != nil {
            panic("disableTx failed: " + err.Error())
        }
    }

    if len(info.Libp2pIDs) == 1 {
        m.stopOldNetworks()
        m.log.Info("partition: lone node, not creating new network")
        m.createDoneFile([]int{m.nodeID})
        return nil
    }

    if info.ReconcileNeeded {
        // Barrier-providing function: requires 100% partition agreement before
        // returning
        c0, c1, ts = ledger.Reconcile(info.Libp2pIDs, info.AmLeader, m.dag, m.log)
        if c0 == "" && c1 == "" && ts == -1 {
            m.log.Info("reconciler timed out (at least 1 node failed)")
            m.stopOldNetworks()
            return errors.New("timeout")
        }
    }
    m.stopOldNetworks()

    // start ABCI app
    app := ledger.NewABCIApp(m.dag, m.log, info.ChainID, c0, c1, ts)
    server := abciserver.NewSocketServer(abciAppAddr, app)
    logfile := fmt.Sprintf("tmlog%d.log", m.nodeID)
    if m.logfd != nil {
        m.logfd.Close() // close existing file descr.
    }
    m.logfd, _ = os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY, 0766)
    l := tmlog.NewTMLogger(tmlog.NewSyncWriter(m.logfd))
    tmlogger, _ := tmflags.ParseLogLevel(tmLogLevel, l, cfg.DefaultLogLevel())
    server.SetLogger(tmlogger)
    if err := server.Start(); err != nil {
        m.log.Fatalf("error starting socket server: %v\n", err)
    }

    // start tendermint
    node, err := m.newTendermint(abciAppAddr, info, tmlogger)
    if err != nil {
        m.log.Fatalf("error starting new tendermint net: %v", err)
    }

    m.createDoneFile(info.MemberNodeIDs)
    node.Start()
    m.instances[node] = server
    return nil
}

func (m *manager) stopOldNetworks() {
    for node, server := range m.instances {
        m.log.Infof("Stopping network instance %p", node)
        node.Stop()
        node.Wait()
        node.Close()
        if err := server.Stop(); err != nil {
            m.log.Fatalf("error starting socket server: %v\n", err)
        }
        delete(m.instances, node)
    }
}

func (m *manager) newTendermint(
    appAddr string,
    info NetworkInfo,
    tmlogger tmlog.Logger,
) (*Node, error) {
    config := cfg.DefaultConfig()
    chainDir := fmt.Sprintf("tmp/chain-%s/node%d/config", info.ChainID, m.nodeID)
    config.RootDir = filepath.Dir(filepath.Join(rootDirStart, chainDir))
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
            fmt.Sprintf("%s.%d:40000", ipPrefix, nodeID + m.idToIPOffset))
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
                // Wait for up to 2 minutes
                time.Sleep(folderPollInterval)
                retries += 1
                if retries > folderPollRetries {
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
    config.RPC.ListenAddress = "tcp://0.0.0.0:30000"
    config.P2P.ListenAddress = "tcp://0.0.0.0:40000"
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

    nde := &Node{}

    // create node
    nde.Node, err = tmn.NewNode(
        config,
        pv,
        nodeKey,
        proxy.NewRemoteClientCreator(appAddr, "socket", true),
        tmn.DefaultGenesisDocProviderFunc(config),
        nde.DBProvider,
        tmn.DefaultMetricsProvider(config.Instrumentation),
        tmlogger,
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create new Tendermint node: %w", err)
    }

    return nde, nil
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

