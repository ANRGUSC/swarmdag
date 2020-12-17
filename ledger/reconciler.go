package ledger

import (
	"fmt"
	"time"
	"context"
	"encoding/json"
	logging "github.com/op/go-logging"
	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	topic = "reconcile"
	repeatsRequired = 3
	pullReqInterval = 200 * time.Millisecond
)

type txInfo struct {
	SortedTxHash uint64	`json:"SortedTxHash"`
	TxCount 	 int	`json:"TxCount"`
}

type reconcileMsg struct {
	MsgType string					`json:"MsgType"`
	LedgerHash uint64				`json:"LedgerHash,omitempty"`
	ChainsHash uint64				`json:"ChainsHash,omitempty"`
	ChainCount int					`json:"ChainCount,omitempty"`
	ChainInfo  map[string]txInfo	`json:"ChainInfo,omitempty"`
	Transactions []Transaction		`json:"Transactions,omitempty"`
	NeededChains []string			`json:"NeededChains,omitempty"`
}

type pendingPull struct {
	peer	peer.ID
	doneCh	chan bool
}

type reconciler struct {
	amLeader bool
	dag *DAG
	peerRepeats	map[peer.ID]int
	log *logging.Logger
	newPullCh chan pendingPull
}

func createMsg(index Index) reconcileMsg {
	chainInfo := index.CompileChainInfo()
	rMsg := reconcileMsg{
		MsgType: "bcast",
		LedgerHash: index.HashLedger(),
		ChainsHash: index.HashSortedChainIDs(),
		ChainCount: len(chainInfo),
		ChainInfo: chainInfo,
	}

	return rMsg
}

func (r *reconciler) ledgerBroadcast(ctx context.Context) {
	bcastTicker := time.NewTicker(r.dag.reconcileBcastInterval)
	for {
		select {
		case <-bcastTicker.C:
			rMsg, _ := json.Marshal(createMsg(r.dag.Idx))
			r.dag.psub.Publish(topic, rMsg)
		case <-ctx.Done():
			return
		}
	}
}

func (r *reconciler) pullMsgHandler(ctx context.Context) {
	var rMsg reconcileMsg
	msgCh := make(chan *pubsub.Message)
	sub, _ := r.dag.psub.Subscribe("pull-" + r.dag.host.ID().String())
	pending := make(map[peer.ID]chan bool)

	go func() {
		for {
			m, err := sub.Next(ctx)
			if err != nil {
				return
			}
			msgCh <- m
		}
	}()

	for {
		select {
		case m := <-msgCh:
			json.Unmarshal(m.Data, &rMsg)

			switch rMsg.MsgType {
			case "pullResp":
				if _, exists := pending[m.GetFrom()]; exists {
					for _, tx := range rMsg.Transactions {
						r.dag.InsertTx(&tx)
					}
					pending[m.GetFrom()] <- true
					delete(pending, m.GetFrom())
				}
			case "pullReq":
				txs := r.dag.CompileTransactions(rMsg.NeededChains)
				pullResp, _ := json.Marshal(reconcileMsg {
					MsgType: "pullResp",
					Transactions: txs,
				})
				r.dag.psub.Publish("pull-" + m.GetFrom().String(), pullResp)
			}
		case p := <-r.newPullCh:
			pending[p.peer] = p.doneCh
		case <-ctx.Done():
			return
		}
	}
}

func (r *reconciler) sendPullReq(srcID peer.ID,	rMsg reconcileMsg) {
	var neededChains []string
	myChainInfo := r.dag.Idx.CompileChainInfo()

	for theirChainID, theirInfo := range rMsg.ChainInfo {
		myInfo, exists := myChainInfo[theirChainID]

		if !exists {
			neededChains = append(neededChains, theirChainID)
		} else if myInfo.SortedTxHash != theirInfo.SortedTxHash {
			if myInfo.TxCount <= theirInfo.TxCount {
				neededChains = append(neededChains, theirChainID)
			}
		}
	}

	if len(neededChains) == 0 {
		return // src should pull ledger from me
	}

	p := pendingPull{srcID, make(chan bool)}
	r.newPullCh <- p
	pullReq, _ := json.Marshal(
		reconcileMsg {
			MsgType: "pullReq",
			NeededChains: neededChains,
	})
	pullReqTicker := time.NewTicker(pullReqInterval)
	for {
		select {
		case <-pullReqTicker.C:
			r.dag.psub.Publish("pull-" + srcID.String(), pullReq)
		case <-p.doneCh:
			return
		}
	}
}

func (r *reconciler) broadcastMsgHandler(ctx context.Context) {
	sub, _ := r.dag.psub.Subscribe(topic)

	var rMsg reconcileMsg
	for {
		m, _ := sub.Next(ctx)
		srcID := m.ReceivedFrom
		json.Unmarshal(m.Data, &rMsg)
		if rMsg.MsgType == "finished" {
			// received finished from leader
			return
		} else if rMsg.MsgType != "bcast" {
			continue
		}
		if (rMsg.LedgerHash == r.dag.Idx.HashLedger()) {
			r.peerRepeats[srcID]++
		} else {
			fmt.Println("send pull req")
			r.sendPullReq(srcID, rMsg)
			r.peerRepeats[srcID] = 0
		}

		// stop reconciliation if same msg rcvd received enough times
		if r.amLeader {
			var notFinished bool
			for s, repeats := range r.peerRepeats {
				if repeats < repeatsRequired {
					notFinished = true
				}
			}
			if notFinished {
				continue
			}
			fin := createMsg(r.dag.Idx)
			fin.MsgType = "finished"
			f, _ := json.Marshal(fin)
			r.dag.psub.Publish(topic, f)
			return
		}
	}
}

func Reconcile(
	libp2pIDs []peer.ID,
	amLeader bool,
	dag *DAG,
	log *logging.Logger,
) error {
	// TODO: need to take care of case of singular membership
	log.Info("Begin reconciler process")
	peerRepeats := make(map[peer.ID]int, len(libp2pIDs))
	for _, id := range libp2pIDs {
		peerRepeats[id] = 0
	}

	fmt.Printf("%+v\n", peerRepeats)

	r := reconciler {
		amLeader: amLeader,
		peerRepeats: peerRepeats,
		dag: dag,
		log: log,
		newPullCh: make(chan pendingPull),
	}

	ctx, cancel := context.WithCancel(dag.ctx)
	defer cancel()

	go r.ledgerBroadcast(ctx)
	go r.pullMsgHandler(ctx)
	r.broadcastMsgHandler(ctx)

	return nil
}
