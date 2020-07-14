package node

import (
	"github.com/ANRGUSC/swarmdag/partition"
	"github.com/ANRGUSC/swarmdag/membership"
	logging "github.com/op/go-logging"
)



type Node struct {
    membershipManager membership.Manager
    partitionManager partition.Manager
}

func (n *Node) Init() {

}