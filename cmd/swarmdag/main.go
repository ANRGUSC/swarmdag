package main

import (
	"syscall"
	"time"
    "fmt"
    "encoding/json"
	"github.com/ANRGUSC/swarmdag/node"
    "github.com/ANRGUSC/swarmdag/membership"
    rpchttp "github.com/tendermint/tendermint/rpc/client/http"
    "github.com/ANRGUSC/swarmdag/ledger"
)

const (
    txInterval = 5 * time.Second
)

func main() {
    // Undo Alpine default umask 0022
    syscall.Umask(0000)

    cfg := &node.Config{
        Membership: membership.Config{
            NodeID: -1, // will be automatically filled
            BroadcastPeriod: 200 * time.Millisecond,
            ProposeHeartbeatInterval: 1,
            ProposeTimerMin: 3, //todo
            ProposeTimerMax: 5,
            PeerTimeout: 5, // seconds
            LeaderTimeout: 10, // seconds
            FollowerTimeout: 10, // sec
            MajorityRatio: 0.51,
        },
        ReconcileBcastInterval: 500 * time.Millisecond,
    }

	node.NewNode(cfg, 8001, "./templates/keys.json")

    c, _ := rpchttp.New("tcp://0.0.0.0:30000", "")
    i := 0
    for {
        tx := ledger.Transaction {
                Hash: "",
                ParentHash: "",
                Timestamp: time.Now().Unix(),
                MembershipID: "",
                Key: fmt.Sprintf("k%d", i),
                Value: fmt.Sprintf("v%d", i),
            }
        txBytes, _ := json.Marshal(tx)
        _, err := c.BroadcastTxCommit(txBytes)
        if err != nil {
            fmt.Println(err)
        }
        time.Sleep(txInterval)
        i++
    }
}