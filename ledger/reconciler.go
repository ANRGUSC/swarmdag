package ledger

import (
	"sync"
	"time"
	"context"
	"sort"
	"fmt"
	"encoding/json"
	logging "github.com/op/go-logging"
	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	repeatsRequired = 2
	pullReqInterval = 500 * time.Millisecond
	reconcilerTimeout = 120 * time.Second
	maxPullReqs = 5
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
	topic *pubsub.Topic
	pullTopics []*pubsub.Topic
	amLeader bool
	dag *DAG
	peerRepeats	map[peer.ID]int
	log *logging.Logger
	newPullCh chan pendingPull
	prevChainIDs map[string]struct{}
	alphaTxTime int64
	wg sync.WaitGroup
	bcastLock sync.Mutex
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
			r.bcastLock.Lock()
			rMsg, _ := json.Marshal(createMsg(r.dag.ChainID(), r.dag.Idx))
			r.bcastLock.Unlock()
			err := r.topic.Publish(ctx, rMsg)
			if err != nil {
				panic(err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *reconciler) pullMsgHandler(ctx context.Context, pullMsgCh chan *pubsub.Message) {
	var rMsg reconcileMsg
	topic, _ := r.dag.psub.Join("pull-" + r.dag.host.ID().String())
	sub, _ := topic.Subscribe()
	pending := make(map[peer.ID]chan bool)

	go func() {
		for {
			m, err := sub.Next(context.Background())
			if err != nil {
				close(msgCh)
				return
			}
			msgCh <- m
		}
	}()

	for {
		// Note: design requires ctx.Done() to be above <-msgCh for thr safety
		select {
		case p := <-r.newPullCh:
			pending[p.peer] = p.doneCh
		case <-ctx.Done():
			// cleanup
			sub.Cancel()
			for range msgCh {}
			for _, done := range pending {
				select {
				case done <- true:
				default:
				}
			}
			for p := range r.newPullCh {p.doneCh <- true}
			topic.Close()
			return
		case m, _ := <-msgCh:
			json.Unmarshal(m.Data, &rMsg)

			switch rMsg.MsgType {
			case "pullResp":
				if _, exists := pending[m.GetFrom()]; exists {
					r.bcastLock.Lock()
					for _, tx := range rMsg.Transactions {
						r.dag.InsertTx(&tx)
					}
					r.bcastLock.Unlock()
					select {
					case pending[m.GetFrom()] <- true:
					default:
					}
					delete(pending, m.GetFrom())
				}
			case "pullReq":
				r.log.Warning("rcvd pull request from " + m.GetFrom()[len(m.GetFrom())-6:])
				txs := r.dag.CompileTransactions(rMsg.NeededChains)
				pullResp, _ := json.Marshal(reconcileMsg {
					MsgType: "pullResp",
					Transactions: txs,
				})
				err := r.dag.psub.Publish("pull-" + m.GetFrom().String(), pullResp)
				if err != nil {
					panic(err)
				}
			}
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
	select {
		case r.newPullCh <- p:
		default:
			return // reconciler timed out
	}
	pullReq, _ := json.Marshal(
		reconcileMsg {
			MsgType: "pullReq",
			NeededChains: neededChains,
	})
	pullReqTicker := time.NewTicker(pullReqInterval)
	for {
		select {
		case <-pullReqTicker.C:
			err := r.dag.psub.Publish("pull-" + srcID.String(), pullReq)
			if err != nil {
				panic(err)
			}
		case <-p.doneCh:
			r.log.Info("done with pull req to " + srcID.String()[len(srcID.String())-6:])
			return
		}
	}
}

func (r *reconciler) broadcastMsgHandler(ctx context.Context) {
	defer r.log.Debug("stop reconciler broadcasting")
	sub, _ := r.topic.Subscribe()
	defer sub.Cancel()
	var lock sync.Mutex
	msgCh := make(chan *pubsub.Message)
	pending := make(map[peer.ID]chan bool)

	// Record of pending pull requests by peer's ledger hash (not ID). This
	// prevents sending a pull req to more than 1 node with the same ledger and
	// sending concurrent pull requests to a single node.
	pending := make(map[uint64]bool, maxPullReqs)
	waitFor := make(map[peer.ID]bool)


	var rMsg reconcileMsg
	for {
		m, err := sub.Next(ctx)
		if err != nil {
			return
		}
		srcID := m.ReceivedFrom
		json.Unmarshal(m.Data, &rMsg)
		switch rMsg.MsgType {
		case "finished":
			// received finished from leader
			r.alphaTxTime = rMsg.AlphaTxTime
			return
		case "pullReq":
		case "bcast":
			// proceed
		default:
			continue
		}
		if rMsg.MsgType == "finished" {

		} else if rMsg.MsgType != "bcast" {
			continue
		}

		r.prevChainIDs[rMsg.PrevChainID] = struct{}{}

		lock.Lock()
		for p, r := range r.peerRepeats {
			matches := p.String()[len(p.String())-6:] == srcID.String()[len(srcID.String())-6:]
			if r < 2 && matches {
				fmt.Println("missing " + p.String()[len(p.String())-6:] + " src is " + srcID.String())
			}
		}
		if (rMsg.LedgerHash == r.dag.Idx.HashLedger()) {
			r.peerRepeats[srcID]++
			if r.amLeader && r.peerRepeats[srcID] == 2 {
				s := srcID.String()
				r.log.Infof("**matching hash**: %s\n", s[len(s)-6:])
				fmt.Printf("peer repeats: %v\n", r.peerRepeats)
			} else if r.peerRepeats[srcID] == 2 {
				s := srcID.String()
				r.log.Infof("matching: %s\n", s[len(s)-6:])
				fmt.Printf("peer repeats: %v\n", r.peerRepeats)
			}
		} else {
			if len(pending) < maxPullReqs {
				if _, exists := pending[rMsg.LedgerHash]; !exists {
					if waitFor[srcID] != true {
						waitFor[srcID] = true
						pending[rMsg.LedgerHash] = true
						go func(id peer.ID) {
							r.wg.Add(1)
							var rm reconcileMsg
							json.Unmarshal(m.Data, &rm)
							r.sendPullReq(id, rm)
							lock.Lock()
							r.peerRepeats[id] = 0
							delete(pending, rm.LedgerHash)
							delete(waitFor, id)
							lock.Unlock()
							r.wg.Done()
						}(srcID)
					}
				}

			}
		}
		lock.Unlock()

		// stop reconciliation if same msg rcvd received enough times
		if r.amLeader {
			notFinished := false
			lock.Lock()
			for _, repeats := range r.peerRepeats {
				if repeats < repeatsRequired {
					notFinished = true
				}
			}
			lock.Unlock()
			if notFinished {
				continue
			}
			fin := createMsg(r.dag.ChainID(), r.dag.Idx)
			fin.MsgType = "finished"
			r.alphaTxTime = time.Now().Unix()
			fin.AlphaTxTime = r.alphaTxTime
			f, _ := json.Marshal(fin)
			err := r.topic.Publish(ctx, f)
			if err != nil {
				panic(err)
			}
			time.Sleep(200 * time.Millisecond)
			err = r.topic.Publish(ctx, f)
			if err != nil {
				panic(err)
			}
			return
		}
	}
}

func timeout(ctx context.Context, cancel context.CancelFunc) {
	select {
	case <-time.After(reconcilerTimeout):
		fmt.Println("reconciler timeout")
		cancel()
	case <-ctx.Done():
	}
}

func (r *reconciler) enableRelay(peerIDs []peer.ID) func() {
	var rCancelFuncs []pubsub.RelayCancelFunc
	var topics []*pubsub.Topic
	for _, p := range peerIDs {
		if p.String() != r.dag.host.ID().String() {
			t, _ := r.dag.psub.Join("pull-" + p.String())
			f, _ := t.Relay()
			topics = append(topics, t)
			rCancelFuncs = append(rCancelFuncs, f)
		}
	}
	disableRelay := func() {
		for _, f := range rCancelFuncs {
			f()
		}
		for _, t := range topics {
			err := t.Close()
			if err != nil {
				panic(err)
			}
		}
	}
	return disableRelay
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

	topic, err := dag.psub.Join("reconcile")

	if err != nil {
		panic(err)
	}

	r := reconciler {
		topic: topic,
		amLeader: amLeader,
		peerRepeats: peerRepeats,
		dag: dag,
		log: log,
		newPullCh: make(chan pendingPull, maxPullReqs),
		prevChainIDs: make(map[string]struct{}, 2),
	}
	defer close(r.newPullCh)
	r.prevChainIDs[r.dag.ChainID()] = struct{}{}
	disableRelay := r.enableRelay(libp2pIDs)

	ctx, cancel := context.WithCancel(dag.ctx)

	pullMsgCh := make(chan *pubsub.Message)
	timeoCtx, timeoCancel := context.WithCancel(ctx)
	go r.ledgerBroadcast(timeoCtx)
	go r.pullMsgHandler(timeoCtx, pullMsgCh)
	go timeout(ctx, timeoCancel)
	r.broadcastMsgHandler(timeoCtx)

	cancel()
	r.wg.Wait()
	err = topic.Close()
	if err != nil {
		panic(err)
	}
	disableRelay()

	log.Infof("Reconciler finished -- my ledger hash: %d\n", dag.Idx.HashLedger())

	// TODO: if an error occurs, there may be MORE parents than 2.......
	var prevChainIDs []string
	for chainID := range r.prevChainIDs {
		prevChainIDs = append(prevChainIDs, chainID)
	}
	sort.Strings(prevChainIDs)

	if len(prevChainIDs) > 2 {
		r.log.Error(
			"Error: unexpected network merge of >2 partitions. Need to slow",
			"down network partition event rate!",
		)
	}

	if r.alphaTxTime > 0 {
		// prevChainIDs[1] is empty during clean network split
		if len(prevChainIDs) > 1 {
			return prevChainIDs[0], prevChainIDs[1], r.alphaTxTime
		} else {
			return prevChainIDs[0], "", r.alphaTxTime
		}
	} else {
		return "", "", -1
	}
}
