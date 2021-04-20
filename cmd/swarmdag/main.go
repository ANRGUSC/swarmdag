package main

import (
	"os"
    "flag"
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
    txInterval = 200 * time.Millisecond
)

var orchestrator string

func init() {
    flag.StringVar(&orchestrator, "orchestrator", "core",
                   "Container orchestrator (\"core\" or \"docker\")")
}

func main() {
    // Undo Alpine default umask 0022 when launching in docker
    if orchestrator == "docker" {
        syscall.Umask(0000)
    }

    os.Chdir("/home/jasonatran/go/src/github.com/ANRGUSC/swarmdag/build")

    cfg := &node.Config{
        Membership: membership.Config{
            NodeID: -1, // TODO: wht to do with this?
            BroadcastPeriod: 300 * time.Millisecond,
            ProposeHeartbeatInterval: 200 * time.Millisecond,
            ProposeTimerMin: 1, //todo
            ProposeTimerMax: 3,
            PeerTimeout: 2000 * time.Millisecond,
            LeaderTimeout: 5 * time.Second,
            FollowerTimeout: 2 * time.Second, // needs to be greater than ProposeHeartbeatInterval
            MajorityRatio: 0.51,
        },
        ReconcileBcastInterval: 400 * time.Millisecond,
        Orchestrator: orchestrator,
    }

	node.NewNode(cfg, 8001, "./templates/keys.json")

    c, _ := rpchttp.New("tcp://0.0.0.0:30000", "")
    i := 0
    for {
        tx := ledger.Transaction {
                Hash: "",
                ParentHash0: "",
                ParentHash1: "",
                Timestamp: time.Now().Unix(),
                MembershipID: "",
                Key: fmt.Sprintf("k%d", i),
                Value: fmt.Sprintf("v%d", i),
            }
        txBytes, _ := json.Marshal(tx)
        _, err := c.BroadcastTxCommit(txBytes)
        if err != nil {
            // fmt.Println(err)
        }
        time.Sleep(txInterval)
        i++
    }
}