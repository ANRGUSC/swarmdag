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

	// Start new membership with ID "chain0"
	dag.CreateAlphaTx("chain0")

	tx := &Transaction{
		Hash: "",
		ParentHash: "",
		MembershipID: "chain0",
		Timestamp: time.Now().Unix(),
		Key: "count",
		Value: "0",
	}
	dag.InsertTx(tx)
	h := tx.Hash

	tx = &Transaction{
		Hash: "",
		ParentHash: "",
		MembershipID: "chain0",
		Timestamp: time.Now().Unix() + 10, // force higher timestamp
		Key: "count",
		Value: "1",
	}
	dag.InsertTx(tx)
	dag.InsertTx(tx) // generates print stating Tx already exists

	txs, _ := json.MarshalIndent(dag.ReturnAll(), "", "    ")
	t.Logf("All Transactions: %s\n", txs)

	c := dag.CompileTransactions([]string{"chain0"})
	cStr, _ := json.MarshalIndent(c, "", "    ")
	t.Log("Transactions of membershipID chain0")
	t.Logf("%s\n", cStr)

	if !dag.TxExists(h) {
		t.Errorf("Tx %s should exist", h)
	}
	if dag.TxExists("not_a_tx") {
		t.Error("hash 'not_a_tx' should not exist")
	}
}
