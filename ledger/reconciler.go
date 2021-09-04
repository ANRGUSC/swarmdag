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
	AlphaTxTime int64					`json:"AlphaTxTime,omitempty"`
	Destination string					`json:"Destination,omitempty"`
	Nonce	int							`json:"Nonce,omitempty"`
}

type reconciler struct {
	topic *pubsub.Topic
	amLeader bool
	dag *DAG
	peerRepeats	map[peer.ID]int
	log *logging.Logger
	prevChainIDs map[string]struct{}
	alphaTxTime int64
	wg sync.WaitGroup
	bcastLock sync.Mutex
	timeo *time.Timer
	finalHash uint64
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
	nonce := 0
	bcastTicker := time.NewTicker(r.dag.reconcileBcastInterval)
	for {
		select {
		case <-bcastTicker.C:
			r.bcastLock.Lock()
			m := createMsg(r.dag.ChainID(), r.dag.Idx)
			m.Nonce = nonce
			rMsg, _ := json.Marshal(m)
			nonce++
			r.bcastLock.Unlock()
			err := r.topic.Publish(ctx, rMsg)
			if err != nil && err != pubsub.ErrTopicClosed {
				panic(err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *reconciler) sendPullReq(srcID peer.ID,	rMsg reconcileMsg, doneCh chan bool, ctx context.Context) {
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

	pullReq, _ := json.Marshal(
		reconcileMsg {
			MsgType: "pullReq",
			NeededChains: neededChains,
			Destination: srcID.String(),
	})
	pullReqTicker := time.NewTicker(pullReqInterval)
	for {
		select {
		case <-pullReqTicker.C:
			r.bcastLock.Lock()
			err := r.topic.Publish(ctx, pullReq)
			if err != nil && err != pubsub.ErrTopicClosed {
				panic(err)
			}
			r.bcastLock.Unlock()
		case <-doneCh:
			return
		}
	}
}

func (r *reconciler) sendFinMsg(alphaTxTime int64, ledgerHash uint64, ctx context.Context) {
	fin := createMsg(r.dag.ChainID(), r.dag.Idx)
	fin.MsgType = "finished"
	fin.AlphaTxTime = alphaTxTime
	fin.LedgerHash = ledgerHash
	f, _ := json.Marshal(fin)
	err := r.topic.Publish(ctx, f)
	if err != nil && err != pubsub.ErrTopicClosed {
		panic(err)
	}
	fmt.Println("sent fin msg")
}

func (r *reconciler) messageHandler(ctx context.Context) {
	defer r.log.Debug("stop reconciler broadcasting")
	sub, _ := r.topic.Subscribe()
	defer sub.Cancel()
	var lock sync.Mutex
	pendingPullsSent := make(map[peer.ID]chan bool)
	reconcileInProg := make(map[peer.ID]bool)
	var pendingLock sync.Mutex

	go func() {
		<-ctx.Done()
		pendingLock.Lock()
		for _, done := range pendingPullsSent {
			select {
			case done <- true:
			default:
			}
		}
		pendingLock.Unlock()
	} ()

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
			r.alphaTxTime = rMsg.AlphaTxTime
			r.sendFinMsg(r.alphaTxTime, rMsg.LedgerHash, ctx) // re-propagate message
			r.finalHash = rMsg.LedgerHash
			return
		case "pullReq":
			if rMsg.Destination == r.dag.host.ID().String() {
				r.log.Warningf("rcvd pull request from %s", m.GetFrom().String()[len(m.GetFrom())-6:])
				txs := r.dag.CompileTransactions(rMsg.NeededChains)
				pullResp, _ := json.Marshal(reconcileMsg {
					MsgType: "pullResp",
					Transactions: txs,
					LedgerHash: r.dag.Idx.HashLedger(),
					Destination: srcID.String(),
				})
				err := r.topic.Publish(ctx, pullResp)
				if err != nil && err != pubsub.ErrTopicClosed {
					panic(err)
				}
			}
			continue
		case "pullResp":
			if rMsg.Destination != r.dag.host.ID().String() {
				continue
			}
			var txCopy []Transaction
			for _, t := range rMsg.Transactions {
				txCopy = append(txCopy, t)
			}
			go func(src peer.ID, txList []Transaction, lh uint64) {
				r.wg.Add(1)
				pendingLock.Lock()
				_, exists := pendingPullsSent[src]
				if inProg, _ := reconcileInProg[src]; inProg {
					exists = false
				}
				if exists {
					reconcileInProg[src] = true
				}
				pendingLock.Unlock()

				if exists {
					pendingLock.Lock()
					select {
					case pendingPullsSent[src] <- true:
						//shuts down sendPullReq routine
					default:
						r.log.Errorf("missing pendingpullsent entry?")
					}
					pendingLock.Unlock()

					r.bcastLock.Lock()
					r.log.Warningf("PROCESSING %s", srcID.String()[len(srcID.String())-6:])
					if lh != r.dag.Idx.HashLedger() {
						r.timeo.Reset(reconcilerTimeout)
						for _, tx := range txList {
							r.dag.InsertTx(&tx)
						}
					}
					r.log.Info("DONE with", srcID.String())

					pendingLock.Lock()
					delete(reconcileInProg, src)
					delete(pendingPullsSent, src)
					r.log.Warningf("pending pulls: %+v", len(reconcileInProg))
					pendingLock.Unlock()

					r.bcastLock.Unlock()

				}
				r.wg.Done()
			} (m.GetFrom(), txCopy, rMsg.LedgerHash)
			continue
		case "bcast":
			// pass
		default:
			continue // unrecognized message
		}

		// for source, repeats := range r.peerRepeats {
		// 	if repeats < 2 {
		// 		if srcID == source {
		// 			fmt.Println("finally got msg from" + srcID.String())
		// 		}
		// 	}
		// }

		if (rMsg.LedgerHash == r.dag.Idx.HashLedger()) {
			r.peerRepeats[srcID]++
			if r.amLeader && r.peerRepeats[srcID] == 2 {
				r.log.Warningf("LEADER MISSING: ")
				for p, repeats := range r.peerRepeats {
					if repeats < 2 {
						fmt.Printf("%s ", p.String()[len(p.String())-6:])
					}
				}
				fmt.Printf("\n")
			} else if r.peerRepeats[srcID] == 2 {
				r.log.Warningf("FOLLOWER MISSING: ")
				for p, repeats := range r.peerRepeats {
					if repeats < 2 {
						fmt.Printf("%s, ", p.String()[len(p.String())-6:])
					}
				}
				fmt.Printf("\n")
			}


		} else {
			pendingLock.Lock()
			if rMsg.LedgerHash == r.dag.Idx.HashLedger() {
				//invalidated by a processed pullResp while waiting for lock
				continue
			}
			r.peerRepeats[srcID] = 0
			_, sent := pendingPullsSent[srcID]
			if len(pendingPullsSent) < maxPullReqs && !sent {
					doneCh := make(chan bool, 1)
					pendingPullsSent[srcID] = doneCh
					var rm reconcileMsg
					json.Unmarshal(m.Data, &rm)
					go func(id peer.ID, rm reconcileMsg, dCh chan bool) {
						r.wg.Add(1)
						r.sendPullReq(id, rm, dCh, ctx)
						r.wg.Done()
					}(srcID, rm, doneCh)
			}
			pendingLock.Unlock()
		}

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
			r.alphaTxTime = time.Now().Unix()
			r.finalHash = r.dag.Idx.HashLedger()
			r.sendFinMsg(r.alphaTxTime, r.finalHash, ctx)
			time.Sleep(200 * time.Millisecond)
			r.sendFinMsg(r.alphaTxTime, r.finalHash, ctx)
			return
		}
	}
}

func (r *reconciler) timeout(ctx context.Context, cancel context.CancelFunc) {
	select {
	case <-r.timeo.C:
		fmt.Println("reconciler timeout")
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
		prevChainIDs: make(map[string]struct{}, 2),
		timeo: time.NewTimer(reconcilerTimeout),
		finalHash: 0,
	}
	r.prevChainIDs[r.dag.ChainID()] = struct{}{}

	ctx, cancel := context.WithCancel(dag.ctx)

	timeoCtx, timeoCancel := context.WithCancel(ctx)
	go r.ledgerBroadcast(timeoCtx)
	go r.timeout(ctx, timeoCancel)
	r.messageHandler(timeoCtx)

	cancel()
	log.Info("Waiting to cancel...................")
	r.wg.Wait()
	if r.dag.Idx.HashLedger() != r.finalHash && r.finalHash != 0 {
		r.log.Error("reconciler finished with wrong hash")
	}
	err = topic.Close()
	if err != nil && err != pubsub.ErrTopicClosed {
		panic(err)
	}

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
