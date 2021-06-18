package ledger

import (
	"errors"
	"time"
	"context"
	"strings"
	"sort"
	"fmt"
	"encoding/hex"
	"encoding/json"
	"crypto/sha1"
	"sync"
	"io/ioutil"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/cayley/graph/path"
	"github.com/cayleygraph/quad"
	"github.com/libp2p/go-libp2p-core/host"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	logging "github.com/op/go-logging"
	_ "github.com/cayleygraph/cayley/graph/kv/bolt"
)

// Future Goals:
// * rename MembershipID to ChainID everywhere except in membership.go

// *Notes: ChainID and MembershipID are synonymous*. ParentHash1 only exists for
// alpha Tx. Hash 0 and 1 are to be lexicographically ordered.
type Transaction struct {
    Hash         string `json:"hash" quad:"hash"`
    ParentHash0  string `json:"parentHash0" quad:"parentHash0"`
    ParentHash1  string `json:"parentHash1" quad:"parentHash1"`
    Timestamp    int64  `json:"timestamp"  quad:"timestap"`
    MembershipID string `json:"membershipID" quad:"membershipID"`
    Key          string `json:"key" quad:"key"`
    Value        string `json:"value" quad:"value"`
}

type DAG struct {
    Idx                     Index
    DB                      *cayley.Handle
    psub                    *pubsub.PubSub
    host                    host.Host
    ctx                     context.Context
    log                     *logging.Logger
    reconcileBcastInterval  time.Duration
    timeLock                []*Transaction
}

var (
    headTx string
    headTxLock = &sync.Mutex{}
    headTxMembershipID string
    timeLockLen = 3
    timeLockMtx = &sync.Mutex{}
    dbLock = &sync.Mutex{} // cayleygraph api with boltDB is not thread safe
)

func NewDAG(
    log *logging.Logger,
    reconcileBcastInterval time.Duration,
    psub *pubsub.PubSub,
    host host.Host,
    ctx context.Context,
) *DAG {
    // File for your new BoltDB. Use path to regular file and not temporary in the real world
    tmpdir, err := ioutil.TempDir("", "cayleyFiles")
    if err != nil {
        panic(err)
    }

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

    d := &DAG{
        Idx: NewIndex(),
        DB: db,
        psub: psub,
        host: host,
        ctx: ctx,
        log: log,
        reconcileBcastInterval: reconcileBcastInterval,
        timeLock: make([]*Transaction, timeLockLen),
    }

    genesis := Transaction{
        // Hash is 1f16bc361b48fabd1c71a9aa7944da7f09fe6902
        Hash:           "",
        ParentHash0:     "g3ns1s",
        ParentHash1:    "",
        Timestamp:      int64(1608123456),
        // 0d54c6bcdfad49ec071ba01601d44df398fc19db
        MembershipID:   "98fc19db",
        Key:            "Genesis",
        Value:          "Genesis",
    }

    genesis.setHash()
    // t := cayley.NewTransaction()
    // t.AddQuad(quad.Make(genesis.Hash, "timestamp", genesis.Timestamp , nil))
    // t.AddQuad(quad.Make(genesis.Hash, "membershipID", genesis.MembershipID , nil))
    // t.AddQuad(quad.Make(genesis.Hash, "key", genesis.Key , nil))
    // t.AddQuad(quad.Make(genesis.Hash, "value", genesis.Value , nil))
    // d.DB.ApplyTransaction(t)
    headTx = genesis.Hash
    headTxMembershipID = genesis.MembershipID
    d.InsertTx(&genesis)
    genesis.Print()
    return d
}

func (d *DAG) ChainID() string {
    return headTxMembershipID
}

func (d *DAG) QueueTx (tx *Transaction) {
    timeLockMtx.Lock()
    defer timeLockMtx.Unlock()
    d.timeLock = append(d.timeLock, tx)
    for len(d.timeLock) > timeLockLen {
        var t *Transaction
        t, d.timeLock = d.timeLock[0], d.timeLock[1:]
        d.InsertTx(t)
    }
}

func (d *DAG) TruncateTimeLock() {
    timeLockMtx.Lock()
    defer timeLockMtx.Unlock()
    d.timeLock = d.timeLock[:0] // len = 0 but capacity preserved
}

func (d *DAG) InsertTx (tx *Transaction) bool {
    dbLock.Lock()
    defer dbLock.Unlock()
    headTxLock.Lock()
    defer headTxLock.Unlock()
    if tx.ParentHash0 == "" {
        // typical append to DAG (not a reconciling insert)
        if (tx.Key != "alphatx") && (headTxMembershipID != tx.MembershipID) {
            d.log.Fatal("Appending to tx with different membership ID!" +
                "If a new membership is created, start with CreateAlpha()")
        }
        if tx.Hash != "" {
            d.log.Fatal("tx hash should not be set on a typical append!")
        }
        tx.ParentHash0 = headTx
        tx.setHash()
        headTx = tx.Hash
        headTxMembershipID = tx.MembershipID
    }
    if d.Search(tx.Hash) != nil {
        // d.log.Debugf("Tx %s already exists", tx.Hash)
        return false
    }
    t := cayley.NewTransaction()
    t.AddQuad(quad.Make(tx.Hash, "parentHash0", tx.ParentHash0 , nil))
    t.AddQuad(quad.Make(tx.Hash, "parentHash1", tx.ParentHash1 , nil))
    t.AddQuad(quad.Make(tx.Hash, "timestamp", tx.Timestamp , nil))
    t.AddQuad(quad.Make(tx.Hash, "membershipID", tx.MembershipID , nil))
    t.AddQuad(quad.Make(tx.Hash, "key", tx.Key , nil))
    t.AddQuad(quad.Make(tx.Hash, "value", tx.Value , nil))
    err := d.DB.ApplyTransaction(t)
    if err != nil {
        d.log.Fatal(err)
    }

    // catolog new tx
    d.Idx.InsertTxHash(tx.MembershipID, tx.Hash)
    // d.log.Infof("Inserted tx %s, parent %s, ledgerhash %d\n", tx.Hash[:6], tx.ParentHash0[:6], d.Idx.HashLedger())
    return true
}

// setHash sets the hash of a transaction
func (t *Transaction) setHash() {
    jsonBytes, _ := json.Marshal(t)
    hash := sha1.Sum(jsonBytes)
    t.Hash = hex.EncodeToString(hash[:])
}

// Print print the every value of the transaction as a string
func (t Transaction) Print() {
    tx, _ := json.MarshalIndent(t, "", "  ")
    fmt.Println(string(tx))
}

// SortbyDate sorts the array based on the date on the transactions (oldest first)
func SortbyDate(txs []Transaction) {
    sort.Slice(txs, func(i, j int) bool {
        return txs[i].Timestamp < txs[j].Timestamp
    })
}

// SortbyHash sorts the array based on the hash on the transactions
func SortbyHash(txs []Transaction) {
    sort.Slice(txs, func(i, j int) bool {
        return txs[i].Hash < txs[j].Hash
    })
}

// direction of 1 means to search up the DAG, -1 means to search down the DAG
func (d *DAG) recursiveSearch(membershipID string, direction int) (string, error){
    var hash string

    // Grab a tx with this membershipID and start iterating from there
    p := cayley.StartPath(d.DB, quad.String(membershipID)).
        In(quad.String("membershipID"))
    tx, err := p.Iterate(nil).FirstValue(nil)
    if err != nil || tx == nil {
        panic("error no Tx for chain " + membershipID)
        return "", errors.New("Error: recursiveSearch() no Tx for chain " + membershipID)
    }
    d.log.Warningf("tx is: %v, %v\n", tx, err)
    txHash := strings.Trim(tx.String(), "\"")
    if direction == 1 {
        p = path.StartPath(d.DB, quad.String(txHash)).
            FollowRecursive(cayley.StartMorphism().Out(quad.String("parentHash0")), 0, nil).
            Has(quad.String("membershipID"), quad.String(membershipID))
    } else if direction == -1 {
        p = path.StartPath(d.DB, tx).
            FollowRecursive(cayley.StartMorphism().In(quad.String("parentHash0")), 0, nil).
            Has(quad.String("membershipID"), quad.String(membershipID))
    } else {
        panic("recursiveSearch: invalid direction")
    }

    ctx := context.TODO()
    p.Iterate(ctx).EachValue(d.DB, func(qv quad.Value) {
        hash = strings.Trim(qv.String(), "\"")
    })

    // Only one tx with this membershipID
    if hash == "" {
        return txHash, nil
    }
    return hash, nil
}

func (d *DAG) getAlpha(membershipID string) (string, error) {
    if membershipID == "" {
        return "", nil
    }
    r, err := d.recursiveSearch(membershipID, 1)
    return r, err
}

// Get hash of tip of membership's chain
func (d *DAG) GetTip(membershipID string) (string, error) {
    if membershipID == "" {
        return "", nil
    }

    r, err := d.recursiveSearch(membershipID, -1)
    return r, err
}

// Search returns nil if not found
func (d *DAG) Search(hash string) *Transaction {
    var tx *Transaction
    p := cayley.StartPath(d.DB).Tag("hash").
        Has(quad.String("timestamp")).
        Save("parentHash0", "parentHash0").
        Save("parentHash1", "parentHash1").
        Save("timestamp", "timestamp").
        Save("membershipID", "membershipID").
        Save("key", "key").
        Save("value", "value")
    err := p.Iterate(nil).TagValues(nil, func(tags map[string]quad.Value) {
        if strings.Compare(quad.NativeOf(tags["hash"]).(string), hash) == 0 {
            tx = &Transaction {
                quad.NativeOf(tags["hash"]).(string),
                quad.NativeOf(tags["parentHash0"]).(string),
                quad.NativeOf(tags["parentHash1"]).(string),
                quad.NativeOf(tags["timestamp"]).(int64),
                quad.NativeOf(tags["membershipID"]).(string),
                quad.NativeOf(tags["key"]).(string),
                quad.NativeOf(tags["value"]).(string),
            }
        }
    })
    if err != nil {
        d.log.Critical(err)
    }
    return tx
}

// ReturnAll returns every transactions
func (d *DAG) ReturnAll() []Transaction {
    p := cayley.StartPath(d.DB).Tag("hash").
        Has(quad.String("timestamp")).
        Save("parentHash0", "parentHash0").
        Save("parentHash1", "parentHash1").
        Save("timestamp", "timestamp").
        Save("membershipID", "membershipID").
        Save("key", "key").
        Save("value", "value")

    txs := []Transaction{}
    err := p.Iterate(nil).TagValues(nil, func(tags map[string]quad.Value) {
        txs = append(txs, Transaction{
            quad.NativeOf(tags["hash"]).(string),
            quad.NativeOf(tags["parentHash0"]).(string),
            quad.NativeOf(tags["parentHash1"]).(string),
            quad.NativeOf(tags["timestamp"]).(int64),
            quad.NativeOf(tags["membershipID"]).(string),
            quad.NativeOf(tags["key"]).(string),
            quad.NativeOf(tags["value"]).(string),
        })
    })
    if err != nil {
        d.log.Critical(err)
    }
    return txs
}

func (d *DAG) createAlphaTx(
    prevChain0 string,
    prevChain1 string,
    newChainID string,
    alphaTxTime int64,
) {
    if prevChain0 == "" {
        prevChain0 = "98fc19db"
    }
    if alphaTxTime == 0 {
        alphaTxTime = 1608123456 + 1
    }
    parent0, err := d.GetTip(prevChain0)
    if err != nil {
        parent0 = "errorTx"
        d.log.Error(err)
    }
    d.log.Debugf("createalpha p0: %s\n", parent0)
    parent1, err := d.GetTip(prevChain1)
    if err != nil {
        parent1 = "errorTx"
        d.log.Error(err)
    }
    d.log.Debugf("createalpha p1: %s\n", parent1)

    alpha := Transaction{
        Hash:           "",
        ParentHash0:    parent0,
        ParentHash1:    parent1,
        MembershipID:   newChainID,
        Timestamp:      alphaTxTime,
        Key:            "alphatx",
        Value:          "alphatx",
    }
    alpha.setHash()
    headTxMembershipID = newChainID
    headTx = alpha.Hash
    d.InsertTx(&alpha)
}

func (d *DAG) CompileTransactions(chainIDs []string) []Transaction {
    dbLock.Lock()
    defer dbLock.Unlock()
    var txs []Transaction
    for _, chain := range chainIDs {
        p := cayley.StartPath(d.DB).Tag("hash").
            Has(quad.String("membershipID"), quad.String(chain)).
            Save("parentHash0", "parentHash0").
            Save("parentHash1", "parentHash1").
            Save("timestamp", "timestamp").
            Save("membershipID", "membershipID").
            Save("key", "key").
            Save("value", "value")

        p.Iterate(nil).TagValues(nil, func(tags map[string]quad.Value) {
                tx := Transaction {
                    quad.NativeOf(tags["hash"]).(string),
                    quad.NativeOf(tags["parentHash0"]).(string),
                    quad.NativeOf(tags["parentHash1"]).(string),
                    quad.NativeOf(tags["timestamp"]).(int64),
                    quad.NativeOf(tags["membershipID"]).(string),
                    quad.NativeOf(tags["key"]).(string),
                    quad.NativeOf(tags["value"]).(string),
                }
                txs = append(txs, tx)
        })
    }

    return txs
}
