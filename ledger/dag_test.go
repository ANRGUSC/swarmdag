package ledger

import (
	"encoding/json"
	"testing"
	"time"
	"context"
	logging "github.com/op/go-logging"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

var log = logging.MustGetLogger("dag_test")
var format = logging.MustStringFormatter(
    `%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
)

func TestDAGLedger(t *testing.T) {
    ctx := context.Background()
  	host, _ := libp2p.New(ctx)
    psub, _ := pubsub.NewGossipSub(ctx, host)
	dag := NewDAG(log, 100 * time.Millisecond, psub, host, ctx)

	dag.createAlphaTx("", "", "chain0", 0)

	tx := &Transaction{
		Hash: "",
		ParentHash0: "",
		ParentHash1: "",
		MembershipID: "chain0",
		Timestamp: time.Now().Unix(),
		Key: "count",
		Value: "0",
	}
	dag.InsertTx(tx)
	h := tx.Hash

	tx = &Transaction{
		Hash: "",
		ParentHash0: "",
		ParentHash1: "",
		MembershipID: "chain0",
		Timestamp: time.Now().Unix() + 10, // force higher timestamp
		Key: "count",
		Value: "1",
	}
	dag.InsertTx(tx)
	dag.InsertTx(tx) // generates print stating Tx already exists

	dag.createAlphaTx("chain0", "", "chain1", time.Now().Unix())

	txs, _ := json.MarshalIndent(dag.ReturnAll(), "", "    ")
	t.Logf("All Transactions: %s\n", txs)

	c := dag.CompileTransactions([]string{"chain0"})
	cStr, _ := json.MarshalIndent(c, "", "    ")
	t.Log("Transactions of membershipID chain0")
	t.Logf("%s\n", cStr)

	if dag.Search(h) == nil {
		t.Errorf("Tx %s should exist", h)
	}
	if dag.Search("not_a_tx") != nil {
		t.Error("hash 'not_a_tx' should not exist")
	}

	h, _ = dag.GetTip("chain0")
	t.Logf("chain0 tip: %s\n", h)
	h, _ = dag.getAlpha("chain0")
	t.Logf("chain0 alpha: %s\n",h)
	h1, _ := dag.GetTip("chain1")
	h2, _ := dag.GetTip("chain1")
	t.Logf("chain1 tip and alpha (should be the same): %s %s\n", h1, h2)
	h, err := dag.GetTip("ShouldNotExist")
	t.Log(err)
}
