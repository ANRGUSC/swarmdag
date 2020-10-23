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
    DB *cayley.Handle
    log *logging.Logger
}

var (
    headTx string
    headTxLock = &sync.Mutex{}
    headTxMembershipID string
)

func NewDAG(log *logging.Logger, host host.Host, psub *pubsub.PubSub) *DAG {
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
        DB: db,
        log: log,
    }

    genesis := Transaction{
        Hash:           "",
        ParentHash:     "Genesis",
        Timestamp:      time.Now().Unix(),
        MembershipID:   "0d54c6bcdfad49ec071ba01601d44df398fc19db", 
        Key:            "Genesis",
        Value:          "Genesis",
    }

    genesis.setHash()
    headTx = genesis.Hash
    d.insertTx(&genesis)

    LatestTransaction = (&genesis)
    genesis.Print()

    return d
}

func (d *DAG) insertTx (tx *Transaction) {
    headTxLock.Lock()
    if tx.ParentHash == "" {
        // typical append to DAG (not a reconciling insert)
        if (tx.Key != "alphatx") && (headTxMembershipID != tx.MembershipID) {
            d.log.Fatal("appending to tx with different membership ID!")
        }
        if tx.Hash != "" {
            d.log.Fatal("tx hash should not be set on a typical append!")
        }
        tx.ParentHash = headTx
        tx.setHash()
        headTx = tx.Hash 
        headTxMembershipID = tx.MembershipID
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
    headTxLock.Unlock()
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

// Get hash of tip of membership chain
func (d *DAG) GetTip(membershipID string) string {
    // Grab a tx with this membershipID and start iterating from there
    p := cayley.StartPath(d.DB, quad.String(membershipID)).
        In(quad.String("membershipID"))
    tx, _ := p.Iterate(nil).FirstValue(nil)
    txHash := strings.Trim(tx.String(), "\"")

    p = cayley.StartPath(d.DB, quad.String(txHash)).
        FollowRecursive(path.StartMorphism().In("parentHash"), 0, nil).
        HasReverse(quad.String("membershipID"), quad.String(membershipID))
    it, _ := p.BuildIterator().Optimize()
    defer it.Close()


    ctx := context.TODO()
    for it.Next(ctx) {}  // iterate until we get the head Tx
    hash := strings.Trim(d.DB.NameOf(it.Result()).String(), "\"")
    return hash
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
    d.insertTx(&alpha)
}

