package ledger

import (
	"time"
	"context"
	"sort"
	"encoding/json"
	logging "github.com/op/go-logging"
	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	topic = "reconcile"
	repeatsRequired = 2
	pullReqInterval = 300 * time.Millisecond
	reconcilerTimeout = 8 * time.Second
)

type txInfo struct {
	SortedTxHash uint64	`json:"SortedTxHash"`
	TxCount 	 int	`json:"TxCount"`
}

type reconcileMsg struct {
	MsgType string						`json:"MsgType"`
	PrevChainID	string			  		`json:"PrevChainID,omitempty"`
	LedgerHash uint64					`json:"LedgerHash,omitempty"`
	ChainsHash uint64					`json:"ChainsHash,omitempty"`
	ChainCount int						`json:"ChainCount,omitempty"`
	ChainInfo  map[string]txInfo		`json:"ChainInfo,omitempty"`
	Transactions []Transaction			`json:"Transactions,omitempty"`
	NeededChains []string				`json:"NeededChains,omitempty"`
	AlphaTxTime int64				`json:"AlphaTxTime,omitempty"`
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
	prevChainIDs map[string]struct{}
	alphaTxTime int64
}

func createMsg(prevChainID string, index Index) reconcileMsg {
	chainInfo := index.CompileChainInfo()
	rMsg := reconcileMsg{
		MsgType: "bcast",
		PrevChainID: prevChainID,
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
			rMsg, _ := json.Marshal(createMsg(r.dag.ChainID(), r.dag.Idx))
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
					select {
					case pending[m.GetFrom()] <- true:
					default:
					}
					delete(pending, m.GetFrom())
				}
			case "pullReq":
				r.log.Warning("rcvd pull request")
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
			sub.Cancel()
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

	r.log.Infof("sending pull req to %s\n", srcID.String())
	r.log.Infof("my hash: %d, their hash: %d\n", r.dag.Idx.HashLedger(), rMsg.LedgerHash)

	p := pendingPull{srcID, make(chan bool, 1)}
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
			r.log.Info("done with pull req")
			return
		}
	}
}

func (r *reconciler) broadcastMsgHandler(ctx context.Context) {
	sub, _ := r.dag.psub.Subscribe(topic)

	var rMsg reconcileMsg
	for {
		m, err := sub.Next(ctx)
		if err != nil {
			return
		}
		srcID := m.ReceivedFrom
		json.Unmarshal(m.Data, &rMsg)
		if rMsg.MsgType == "finished" {
			// received finished from leader
			r.alphaTxTime = rMsg.AlphaTxTime
			return
		} else if rMsg.MsgType != "bcast" {
			continue
		}

		r.prevChainIDs[rMsg.PrevChainID] = struct{}{}

		if (rMsg.LedgerHash == r.dag.Idx.HashLedger()) {
			r.peerRepeats[srcID]++
		} else {
			r.sendPullReq(srcID, rMsg)
			r.peerRepeats[srcID] = 0
		}

		// stop reconciliation if same msg rcvd received enough times
		if r.amLeader {
			notFinished := false
			for _, repeats := range r.peerRepeats {
				if repeats < repeatsRequired {
					notFinished = true
				}
			}
			if notFinished {
				continue
			}
			fin := createMsg(r.dag.ChainID(), r.dag.Idx)
			fin.MsgType = "finished"
			r.alphaTxTime = time.Now().Unix()
			fin.AlphaTxTime = r.alphaTxTime
			f, _ := json.Marshal(fin)
			r.dag.psub.Publish(topic, f)
			r.dag.psub.Publish(topic, f)
			return
		}
	}
}

func timeout(ctx context.Context, cancel context.CancelFunc) {
	select {
	case <-time.After(reconcilerTimeout):
		cancel()
	case <-ctx.Done():
	}
}

// Reconciler assumes network events only include a simple split or a merger of
// only two partitions.
func Reconcile(
	libp2pIDs []peer.ID,
	amLeader bool,
	dag *DAG,
	log *logging.Logger,
) (string, string, int64) {
	// TODO: need to take care of case of singular membership. is this already done?
	log.Infof("Begin reconciler process, leader: %t\n", amLeader)
	peerRepeats := make(map[peer.ID]int, len(libp2pIDs))
	for _, id := range libp2pIDs {
		peerRepeats[id] = 0
	}

	r := reconciler {
		amLeader: amLeader,
		peerRepeats: peerRepeats,
		dag: dag,
		log: log,
		newPullCh: make(chan pendingPull),
		prevChainIDs: make(map[string]struct{}, 2),
	}
	r.prevChainIDs[r.dag.ChainID()] = struct{}{}

	ctx, cancel := context.WithCancel(dag.ctx)
	defer cancel()

	timeoCtx, timeoCancel := context.WithCancel(ctx)
	go r.ledgerBroadcast(ctx)
	go r.pullMsgHandler(ctx)
	go timeout(timeoCtx, timeoCancel)
	r.broadcastMsgHandler(timeoCtx)

	log.Infof("Sanity check -- my hash: %d\n", dag.Idx.HashLedger())

	prevChainIDs := []string{"", ""}
	i := 0
	for chainID := range r.prevChainIDs {
		prevChainIDs[i] = chainID
		i++
	}
	sort.Strings(prevChainIDs)

	if r.alphaTxTime > 0 {
		// prevChainIDs[1] is empty during clean network split
		return prevChainIDs[0], prevChainIDs[1], r.alphaTxTime
	} else {
		return "", "", 0
	}
}
