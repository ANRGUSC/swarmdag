package main

import (
	"syscall"
	"time"
	"github.com/ANRGUSC/swarmdag/node"
    "github.com/ANRGUSC/swarmdag/membership"
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
	select {}
}