package ledger

import (
    "time"
	"context"
	"strings"
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
)

type Transaction struct {
    Hash         string `json:"hash" quad:"hash"`
    ParentHash   string `json:"parentHash" quad:"parentHash"`
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
}

var (
    headTx string
    headTxLock = &sync.Mutex{}
    headTxMembershipID string
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
    }

    genesis := Transaction{
        Hash:           "",
        ParentHash:     "Genesis",
        Timestamp:      time.Now().Unix(),
        // 0d54c6bcdfad49ec071ba01601d44df398fc19db
        MembershipID:   "fc19db",
        Key:            "Genesis",
        Value:          "Genesis",
    }

    genesis.setHash()
    headTx = genesis.Hash
    d.InsertTx(&genesis)

    LatestTransaction = (&genesis)
    genesis.Print()

    return d
}

func (d *DAG) InsertTx (tx *Transaction) bool {
    headTxLock.Lock()
    defer headTxLock.Unlock()
    if tx.ParentHash == "" {
        // typical append to DAG (not a reconciling insert)
        if (tx.Key != "alphatx") && (headTxMembershipID != tx.MembershipID) {
            d.log.Fatal("Appending to tx with different membership ID!" +
                "If a new membership is created, start with CreateAlpha()")
        }
        if tx.Hash != "" {
            d.log.Fatal("tx hash should not be set on a typical append!")
        }
        tx.ParentHash = headTx
        tx.setHash()
        headTx = tx.Hash
        headTxMembershipID = tx.MembershipID
    }
    if d.TxExists(tx.Hash) {
        d.log.Debugf("Tx %s already exists", tx.Hash)
        return false
    }
    t := cayley.NewTransaction()
    t.AddQuad(quad.Make(tx.Hash, "parentHash", tx.ParentHash , nil))
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

func (d *DAG) recursiveSearch(membershipID string, direction int) string {
    // Grab a tx with this membershipID and start iterating from there
    p := cayley.StartPath(d.DB, quad.String(membershipID)).
        In(quad.String("membershipID"))
    tx, _ := p.Iterate(nil).FirstValue(nil)
    txHash := strings.Trim(tx.String(), "\"")

    if direction == 1 {
        p = cayley.StartPath(d.DB, quad.String(txHash)).
            FollowRecursive(path.StartMorphism().Out("parentHash"), -1, nil).
            HasReverse(quad.String("membershipID"), quad.String(membershipID))
    } else if direction == -1 {
        p = cayley.StartPath(d.DB, quad.String(txHash)).
            FollowRecursive(path.StartMorphism().In("parentHash"), -1, nil).
            HasReverse(quad.String("membershipID"), quad.String(membershipID))
    } else {
        panic("recursiveSearch: invalid direction")
    }


    it, _ := p.BuildIterator().Optimize()
    defer it.Close()

    ctx := context.TODO()
    for it.Next(ctx) {}  // iterate until we get the alpha Tx
    hash := strings.Trim(d.DB.NameOf(it.Result()).String(), "\"")
    return hash
}

func (d *DAG) getAlpha(membershipID string) string {
    return d.recursiveSearch(membershipID, 1)
}

// Get hash of tip of membership chain
func (d *DAG) GetTip(membershipID string) string {
    return d.recursiveSearch(membershipID, -1)
}

func (d *DAG) TxExists(hash string) bool {
    p := cayley.StartPath(d.DB, quad.String(hash))
    tx, _ := p.Iterate(nil).FirstValue(nil)
    if tx != nil {
        return true
    }
    return false
}

// Search returns nil if not found
func (d *DAG) Search(hash string) *Transaction {
    var tx *Transaction = nil
    p := cayley.StartPath(d.DB).Tag("hash").
        Has(quad.String("timestamp")).
        Save("parentHash", "parentHash").
        Save("timestamp", "timestamp").
        Save("membershipID", "membershipID").
        Save("key", "key").
        Save("value", "value")
    err := p.Iterate(nil).TagValues(nil, func(tags map[string]quad.Value) {
        if strings.Compare(quad.NativeOf(tags["hash"]).(string), hash) == 0 {
            tx = &Transaction {
                quad.NativeOf(tags["hash"]).(string),
                quad.NativeOf(tags["parentHash"]).(string),
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
        Save("parentHash", "parentHash").
        Save("timestamp", "timestamp").
        Save("membershipID", "membershipID").
        Save("key", "key").
        Save("value", "value")

    txs := []Transaction{}
    err := p.Iterate(nil).TagValues(nil, func(tags map[string]quad.Value) {
        txs = append(txs, Transaction{
            quad.NativeOf(tags["hash"]).(string),
            quad.NativeOf(tags["parentHash"]).(string),
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

func (d *DAG) CreateAlphaTx(membershipID string) {
    alpha := Transaction{
        Hash:           "",
        ParentHash:     "",
        MembershipID:   membershipID,
        Timestamp:      time.Now().Unix(),
        Key:            "alphatx",
        Value:          "alphatx",
    }
    d.InsertTx(&alpha)
}

func (d *DAG) CompileTransactions(chainIDs []string) []Transaction {
    var txs []Transaction
    for _, chain := range chainIDs {
        p := cayley.StartPath(d.DB).Tag("hash").
            Has(quad.String("membershipID"), quad.String(chain)).
            Save("parentHash", "parentHash").
            Save("timestamp", "timestamp").
            Save("membershipID", "membershipID").
            Save("key", "key").
            Save("value", "value")

        p.Iterate(nil).TagValues(nil, func(tags map[string]quad.Value) {
                tx := Transaction {
                    quad.NativeOf(tags["hash"]).(string),
                    quad.NativeOf(tags["parentHash"]).(string),
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
