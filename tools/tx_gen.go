package main

import (
	"encoding/json"
	"fmt"
	"time"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/ANRGUSC/swarmdag/ledger"
)

// send transactions for mock_abci.go
func main() {
	c, _ := rpchttp.New("tcp://127.0.0.1:26657", "")

	for i := 0; i < 20; i++ {
		tx := ledger.Transaction {
				Hash: "",
	            ParentHash: "",
	            Timestamp: time.Now().Unix(),
	            MembershipID: "mock_abci",
	            Key: fmt.Sprintf("k%d", i),
	            Value: fmt.Sprintf("v%d", i),
	        }
		txBytes, _ := json.Marshal(tx)
		bres, err := c.BroadcastTxCommit(txBytes)
		if err != nil {
			fmt.Println(err)
		}
		resp, _ := json.MarshalIndent(bres, "", "  ")
		fmt.Println(string(resp))
		time.Sleep(500 * time.Millisecond)
	}

	cmd := []byte("returnAll")
	qres, _ := c.ABCIQuery("", cmd)
	fmt.Println("all transactions in ledger:", string(qres.Response.Value))
}