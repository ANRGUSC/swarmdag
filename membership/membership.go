package membership

import (
	"encoding/hex"
	"crypto/sha1"
	"context"
	"time"
	"encoding/json"
	"math/rand"
	"sync"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	logging "github.com/op/go-logging"
    "github.com/ANRGUSC/swarmdag/partition"
)

var isLeader bool = false

type nodeState int

const (
    FOLLOW_ANY nodeState = 0
    LEAD_PROPOSAL nodeState = 1
    FOLLOW_PROPOSER nodeState = 2
)

var nextMembershipListLock sync.RWMutex

type Manager interface {
    Start()
    ConnectHandler(c network.Conn)
}

type Config struct {
    NodeID                      int
    MaxConnections              int
    BroadcastPeriod             time.Duration
    ProposeHeartbeatInterval    int

    ProposeTimerMin             int
    ProposeTimerMax             int

    PeerTimeout                 int
    LeaderTimeout               int
    FollowerTimeout             int
    MajorityRatio               float32
}

type manager struct {
    conf                Config
    partManager         partition.Manager

    ctx                 context.Context
    psub                *pubsub.PubSub
    host                host.Host
    notif               *network.NotifyBundle

    membershipID        string
    numConnections      int
    neighborList        map[string]peer.ID
    membershipList      map[string]int64
    nextMembershipList  map[string]int64
    viewID              int
    libp2pIDs           map[peer.ID]int
    swarmSize           int
    initializing        bool

    membershipBroadcastCh   chan string
    proposeRoutineCh        chan string
    state                   nodeState
    log                     *logging.Logger

    dmLeadCh                chan *pubsub.Message
    leadingProposalCh       chan bool
}

type directMsg struct {
    MsgType string      `json:"MsgType"`
}

func NewManager(
    conf Config,
    partManager partition.Manager,
    ctx  context.Context,
    host host.Host,
    psub *pubsub.PubSub,
    log  *logging.Logger,
    libp2pIDs map[peer.ID]int,
) Manager {

    if conf.MaxConnections == 0 {
        conf.MaxConnections = 9999
    }

    if conf.BroadcastPeriod == 0 {
        conf.BroadcastPeriod = 200 * time.Millisecond
    }

    if conf.ProposeHeartbeatInterval == 0 {
        conf.ProposeHeartbeatInterval = 1
    }

    if conf.ProposeTimerMin == 0 {
       conf.ProposeTimerMin = 3
    }

    if conf.ProposeTimerMax == 0 {
       conf.ProposeTimerMax = 5
    }

    if conf.PeerTimeout == 0 {
        conf.PeerTimeout = 1 //seconds
    }

    if conf.LeaderTimeout == 0 {
        conf.LeaderTimeout = 5
    }

    if conf.FollowerTimeout == 0 {
        conf.FollowerTimeout = 1
    }

    if conf.MajorityRatio == 0 {
        conf.MajorityRatio = 0.51
    }

    m := &manager {
        conf:                       conf,
        partManager:                partManager,

        ctx:                        ctx,
        psub:                       psub,
        host:                       host,
        notif:                      &network.NotifyBundle{},

        numConnections:             0,
        neighborList:               make(map[string]peer.ID),
        membershipList:             make(map[string]int64),
        nextMembershipList:         make(map[string]int64),
        viewID:                     0, // first actual viewID will be 1
        libp2pIDs:                  libp2pIDs,
        swarmSize:                  len(libp2pIDs),
        initializing:               true,

        membershipBroadcastCh:      make(chan string),
        proposeRoutineCh:           make(chan string, 2),
        state:                      FOLLOW_ANY,
        log: log,

        dmLeadCh:                   make(chan *pubsub.Message),
        leadingProposalCh:          make(chan bool),
    }

    return m
}

func (m *manager) ConnectHandler(c network.Conn) {
    m.log.Debug("rcvd remote conn. request from", m.libp2pIDs[c.RemotePeer()])

    // OPT: this assumes that this node will accept all connection requests from
    //remote nodes. further optimization can be to deny connections to reduce
    //the degree of the network while still maintaining a neighbor list
    _, neighbor := m.neighborList[c.RemotePeer().String()]

    if !neighbor {
        m.neighborList[c.RemotePeer().String()] = c.RemotePeer()
    }
}

func (m *manager) Start() {
    m.host.Network().SetConnHandler(m.ConnectHandler)
    peerChan := initMDNS(m.ctx, m.host, "rendezvousStr", 5 * time.Second)
    m.membershipList[m.host.ID().String()] = time.Now().Unix()
    m.membershipList["VIEW_ID"] = int64(m.viewID)
    m.nextMembershipList["VIEW_ID"] = int64(m.viewID + 1)
    m.notif.ConnectedF = func(n network.Network, c network.Conn) {
            // m.log.Debug("NOTIFIEE CONNECTED")
    }
    m.notif.DisconnectedF = func(n network.Network, c network.Conn) {
        m.log.Debug("disconnected from node", m.libp2pIDs[c.RemotePeer()])
    }
    m.host.Network().Notify(m.notif)

    go m.membershipBroadcast()
    go m.membershipBroadcastHandler()
    go m.proposeRoutine()
    go m.mdnsQueryHandler(peerChan)
    go m.directMsgDispatcher()
}

func isAddr(addr string) bool {
    return (addr != "INSTALL_VIEW" && addr != "CONFIRMED_LEADER" &&
            addr != "PROPOSE" && addr != "VIEW_ID" && addr != "VOTE_TABLE")
}

func (m *manager) mdnsQueryHandler(peerChan chan peer.AddrInfo) {
    for {
        p := <-peerChan // block until a mdns query is received

        _, neighbor := m.neighborList[p.ID.String()]
        nextMembershipListLock.Lock()
        _, member := m.membershipList[p.ID.String()]

        if member {
            m.membershipList[p.ID.String()] = time.Now().Unix()
        }

        m.nextMembershipList[p.ID.String()] = time.Now().Unix()

        nextMembershipListLock.Unlock()

        //TODO use peer store to determine num connections
        if m.numConnections < m.conf.MaxConnections {
            //connect will only actually dial if a connection does not exist
            if err := m.host.Connect(m.ctx, p); err != nil {
                m.log.Warning("Connection failed:", err)
            }
            // m.numConnections++ //TODO: implement counter
        }

        if !neighbor {
            m.neighborList[p.ID.String()] = p.ID
        }
    }
}

func (m *manager) checkPeerTimeouts() bool {
    changed := false

    nextMembershipListLock.Lock()
    defer nextMembershipListLock.Unlock()

    m.nextMembershipList[m.host.ID().String()] = time.Now().Unix()
    for addr, timestamp := range m.nextMembershipList {
        if isAddr(addr) {
            if time.Now().Unix() - timestamp > int64(m.conf.PeerTimeout) {
                delete(m.nextMembershipList, addr)
                changed = true
            }
        }
    }

    return changed
}

func (m *manager) membershipBroadcast() {
    broadcastTicker := time.NewTicker(m.conf.BroadcastPeriod)
    for {
        select {
        case <-broadcastTicker.C:
            m.checkPeerTimeouts()

            nextMembershipListLock.RLock()
            nmlist, _ := json.Marshal(m.nextMembershipList)
            nextMembershipListLock.RUnlock()

            m.psub.Publish("next_membership", nmlist)
        case cmd := <-m.membershipBroadcastCh:
            switch cmd {
            case "START_BROADCAST":
                m.log.Debug("start membership broadcast")
                broadcastTicker = time.NewTicker(m.conf.BroadcastPeriod)
            case "STOP_BROADCAST":
                m.log.Debug("stop membership broadcast")
                broadcastTicker.Stop()
            }
        }
    }
}

func (m *manager) voteTableBroadcast(
    voteTable *map[string]int,
    vtLock *sync.Mutex,
    ctx context.Context,
) {
    //use the same period as membership broadcast
    ticker := time.NewTicker(m.conf.BroadcastPeriod)
    m.log.Debug("start vote table broadcasts")
    for {
        select {
        case <-ticker.C:
            vtLock.Lock()
            vt, _ := json.Marshal(*voteTable)
            vtLock.Unlock()
            m.psub.Publish("membership_propose", vt)
        case <-ctx.Done():
            m.log.Debug("stopping vote table broadcasts")
            return
        }
    }
}

func (m *manager) directMsgDispatcher() {
    var leading bool
    msgCh := make(chan *pubsub.Message)
    sub, _ := m.psub.Subscribe(m.host.ID().String())

    var dm directMsg

    go func() {
        for {
            msg, err := sub.Next(m.ctx)
            if err != nil {
                panic(err)
            }
            msgCh <- msg
        }
    }()

    for {
        select {
        case msg := <-msgCh:
            if leading {
                json.Unmarshal(msg.Data, dm)
                switch dm.MsgType {
                case "voteAck":
                    m.dmLeadCh <- msg
                case "voteNack":
                    m.dmLeadCh <- msg
                }
            }
        case leading = <-m.leadingProposalCh:
        case <-m.ctx.Done():
            return
        }
    }

}

// In voteTable, "1" means yes, "0" means no response, and "-1" means a NACK
func (m *manager) leadProposal() {
    var src peer.ID
    var numNACKs int
    numVotes := 1
    voteTable := make(map[string]int, len(m.nextMembershipList))
    var vtLock sync.Mutex
    voteCtx, cancelVote := context.WithCancel(context.Background())
    leadProposalTimeout := time.NewTicker(time.Duration(m.conf.LeaderTimeout) *
                                          time.Second)

    //initiate empty table
    nextMembershipListLock.RLock()
    for addr, _ := range m.nextMembershipList {
        if isAddr(addr) {
            voteTable[addr] = 0
        }
    }
    voteTable["VOTE_TABLE"] = 1
    nextMembershipListLock.RUnlock()

    voteTable[m.host.ID().String()] = 1
    go m.voteTableBroadcast(&voteTable, &vtLock, voteCtx)

    for {
        // the -2 accounts for the "PROPOSE" and "VIEW_ID" keys
        nextMembershipListLock.RLock()
        nextMembershipLen := len(m.nextMembershipList) - 2
        nextMembershipListLock.RUnlock()

        if (float32(numVotes) / float32(nextMembershipLen)) > m.conf.MajorityRatio {
            //let nodes know that votes confirm you are the leader
            // fix: this occurs for each vote message
            m.proposeRoutineCh<-"CONFIRMED_LEADER"

            if numVotes == (nextMembershipLen) {
                cancelVote()
                nextMembershipListLock.Lock()
                delete(m.nextMembershipList, "CONFIRMED_LEADER")
                delete(m.nextMembershipList, "PROPOSE")
                m.nextMembershipList["INSTALL_VIEW"] = 1
                nextMembershipListLock.Unlock()

                // Let two broadcasts with "INSTALL_VIEW" before starting TM
                // libp2p gossipsub should ensure 100% reliability
                time.Sleep(2 * m.conf.BroadcastPeriod)

                m.installMembershipView(m.nextMembershipList, m.host.ID().String())
                m.proposeRoutineCh<-"RESET"
                m.leadingProposalCh<-false
                return
            }
        }

        select {
        case <-leadProposalTimeout.C:
            m.log.Debug("LEADER TIMEOUT")
            m.proposeRoutineCh<-"RESET"
            cancelVote()
            m.leadingProposalCh<-false
            return
        case dm := <-m.dmLeadCh:
            proposeResp := make(map[string]int64)
            json.Unmarshal(dm.Data, &proposeResp)
            src, _ = peer.IDFromBytes(dm.From)

            //check if response indicates a leader already exists. must check
            //before NACK
            if proposeResp["LEADER_EXISTS"] == 1 {
                m.proposeRoutineCh<-"RESET"
                cancelVote()
                m.leadingProposalCh<-false
                return
            }

            //check if NACK message. if so, increment NACK count. Decrement ACK
            //count if necessary
            if proposeResp["NACK"] == 1 {
                m.log.Debug("received NACK")
                vtLock.Lock()
                if voteTable[src.String()] == 1 {
                    numVotes -= 1
                }
                voteTable[src.String()] = -1
                vtLock.Unlock()
                numNACKs += 1

                nextMembershipListLock.RLock()
                nextMembershipLen = len(m.nextMembershipList) - 2
                nextMembershipListLock.RUnlock()
                // the -1 accounts for the "PROPOSE" key
                if (float32(numNACKs) / float32(nextMembershipLen)) >
                        (1.0 - m.conf.MajorityRatio) {
                    m.proposeRoutineCh<-"RESET"
                    m.leadingProposalCh<-false
                    return
                }

                continue
            }

            //must be a propose membership ACK response
            nextMembershipListLock.Lock()
            for addr, timestamp := range proposeResp {
                if isAddr(addr) {
                    if m.nextMembershipList[addr] < proposeResp[addr] {
                        m.nextMembershipList[addr] = timestamp
                    }
                }
            }
            nextMembershipListLock.Unlock()

            vtLock.Lock()
            if voteTable[src.String()] == -1 {
                numNACKs -= 1
                voteTable[src.String()] = 1
            }
            vtLock.Unlock()

            numVotes += 1
        }


    }

    m.leadingProposalCh<-false
}

func (m *manager) checkMembershipChange() bool {
    m.checkPeerTimeouts()
    nextMembershipListLock.Lock()
    defer nextMembershipListLock.Unlock()

    nmlen := len(m.nextMembershipList)
    mlen := len(m.membershipList)
    invalid := []string{"INSTALL_VIEW", "CONFIRMED_LEADER", "PROPOSE", "VIEW_ID", "VOTE_TABLE"}

    for k := range invalid {
        if m.nextMembershipList[invalid[k]] != 0 {
            nmlen = nmlen - 1
        }
        if m.membershipList[invalid[k]] != 0 {
            mlen = mlen - 1
        }
    }

    if len(m.nextMembershipList) == len(m.membershipList) {
        for addr, _ := range m.nextMembershipList {
            if isAddr(addr) {
                if m.membershipList[addr] == 0 {
                    return true
                }
            }
        }
    } else {
        return true
    }

    return false
}

func createAck(mlist map[string]int64) map[string]interface{} {
    ack := make(map[string]interface{}, len(mlist) + 1)
    ack["MsgType"] = "ackResp"
    for k, v := range mlist {
        ack[k] = v
    }
    return ack
}

func (m *manager) proposeRoutine() {
    rand.Seed(time.Now().UnixNano())
    proposeSub, _ := m.psub.Subscribe("membership_propose")
    rcvdMsg := make(chan bool)
    nextMsg := make(chan bool)
    var src peer.ID
    var leader string
    mlist := make(map[string]int64)
    // TODO: make nack a simple struct msg
    nack := directMsg{MsgType: "nackResp"}

    //sleep min time plus a random value up to the max at 10ms resolution
    proposeTicker := time.NewTicker(time.Duration(m.conf.ProposeTimerMin * 1000 +
        rand.Intn((m.conf.ProposeTimerMax - m.conf.ProposeTimerMin) * 100) * 10) *
        time.Millisecond)

    // init timers
    proposeHeartbeat := time.NewTicker(10 * time.Second)
    followTimeo := time.NewTicker(1)
    proposeHeartbeat.Stop()
    followTimeo.Stop()


    go func(){
        for {
            got, _ := proposeSub.Next(m.ctx)
            mlist = make(map[string]int64)
            json.Unmarshal(got.Data, &mlist)
            src, _ = peer.IDFromBytes(got.From)

            if src.String() != m.host.ID().String() {
                rcvdMsg <- true
                <-nextMsg
            }
        }
    }()

    for {
        select {
        case <-proposeTicker.C:

            if m.state == FOLLOW_ANY {
                if m.checkMembershipChange() {
                    m.log.Debugf("proposing membership update to view ID %d\n", m.viewID + 1)
                    m.state = LEAD_PROPOSAL
                    leader = m.host.ID().String() //for creating membership ID
                    m.log.Debug("state LEAD_PROPOSAL")
                    nextMembershipListLock.Lock()
                    m.nextMembershipList["PROPOSE"] = 1
                    nextMembershipListLock.Unlock()
                    go m.leadProposal()
                    proposeTicker.Stop()
                    proposeHeartbeat = time.NewTicker(time.Duration(m.conf.ProposeHeartbeatInterval))
                }
            } else {
                m.log.Debug("no changes detected, not proposing membership update")
            }

        case <-proposeHeartbeat.C:
            m.checkPeerTimeouts()
            nextMembershipListLock.RLock()
            nmlist, _ := json.Marshal(m.nextMembershipList)
            m.psub.Publish("membership_propose", nmlist)
            nextMembershipListLock.RUnlock()

        case <-rcvdMsg:
            switch m.state {
            case LEAD_PROPOSAL:
                if mlist["CONFIRMED_LEADER"] == 1 {
                    m.proposeRoutineCh <- "RESET"
                }

                // another leader asking to install view
                if mlist["INSTALL_VIEW"] == 1  && mlist["VIEW_ID"] > int64(m.viewID) {
                    m.log.Debug("state FOLLOW_ANY")
                    m.state = FOLLOW_ANY
                    m.installMembershipView(mlist, src.String())
                    m.proposeRoutineCh <- "RESET"
                    break
                }

                // I'm leading, nack anyone else
                if mlist["PROPOSE"] == 1 {
                    nackResp, _ := json.Marshal(nack)
                    m.psub.Publish(src.String(), nackResp)
                }
            case FOLLOW_ANY:
                if mlist["PROPOSE"] == 1 && int64(mlist["VIEW_ID"]) > m.membershipList["VIEW_ID"] {
                    m.log.Debug("ACKing proposal for membership update to node", m.libp2pIDs[src])
                    m.updateMembershipList(mlist)
                    nextMembershipListLock.RLock()
                    ackResp, _ := json.Marshal(createAck(m.nextMembershipList))
                    nextMembershipListLock.RUnlock()
                    m.psub.Publish(src.String(), ackResp)
                    leader = src.String()
                    m.state = FOLLOW_PROPOSER
                    proposeTicker.Stop()
                    followTimeo = time.NewTicker(time.Duration(m.conf.FollowerTimeout) * time.Second)
                    m.log.Debug("state FOLLOW_PROPOSER")
                    m.membershipBroadcastCh<-"STOP_BROADCAST"
                }
                // fixme: respond to out of sync node? (trying to propose lower viewID, etc)
            case FOLLOW_PROPOSER:
RESPOND_TO_PROPOSER:
                if src.String() == leader {
                    //only respond to vote tables
                    if mlist["VOTE_TABLE"] == 1 {
                        if mlist[m.host.ID().String()] < 1 {
                            nextMembershipListLock.RLock()
                            ackResp, _ := json.Marshal(createAck(m.nextMembershipList))
                            nextMembershipListLock.RUnlock()
                            m.psub.Publish(src.String(), ackResp)
                        }
                    }
                    if mlist["INSTALL_VIEW"] == 1  && mlist["VIEW_ID"] > int64(m.viewID) {
                        m.log.Debug("state FOLLOW_ANY")
                        m.state = FOLLOW_ANY
                        followTimeo.Stop()
                        m.installMembershipView(mlist, leader)
                        m.proposeRoutineCh <- "RESET"
                        break
                    }
                    if mlist["PROPOSE"] == 1 {
                        if m.updateMembershipList(mlist) {
                            nextMembershipListLock.RLock()
                            ackResp, _ := json.Marshal(createAck(m.nextMembershipList))
                            nextMembershipListLock.RUnlock()
                            m.psub.Publish(src.String(), ackResp)
                        }
                    }
                } else {
                    if mlist["CONFIRMED_LEADER"] == 1 && mlist["VIEW_ID"] > int64(m.viewID) {
                        leader = src.String()
                        goto RESPOND_TO_PROPOSER
                    }
                    //already responded to a leader, NACK any other proposals
                    if mlist["PROPOSE"] == 1 {
                        nackResp, _ := json.Marshal(nack)
                        m.psub.Publish(src.String(), nackResp)
                    }
                }
            } // switch

            nextMsg <- true

        case <-followTimeo.C:
            m.log.Debug("FOLLOW_PROPOSER timeout!")
            m.state = FOLLOW_ANY
            followTimeo.Stop()
            m.proposeRoutineCh <- "RESET"

        case status := <-m.proposeRoutineCh:
            switch status {
            case "RESET":
                m.log.Debug("Resetting proposeRoutine()...")

                m.state = FOLLOW_ANY
                m.log.Debug("state FOLLOW_ANY")
                proposeHeartbeat.Stop()
                nextMembershipListLock.Lock()
                delete(m.nextMembershipList, "CONFIRMED_LEADER") //just in case
                delete(m.nextMembershipList, "PROPOSE")
                delete(m.nextMembershipList, "INSTALL_VIEW")
                nextMembershipListLock.Unlock()

                proposeTicker = time.NewTicker(time.Duration(m.conf.ProposeTimerMin * 1000 +
                    rand.Intn((m.conf.ProposeTimerMax - m.conf.ProposeTimerMin) * 100) * 10) *
                    time.Millisecond)
                m.membershipBroadcastCh<-"START_BROADCAST"
            case "CONFIRMED_LEADER":
                m.log.Debug("confirmed leader node")
                //include leader confirmation in membership broadcast
                nextMembershipListLock.Lock()
                if m.nextMembershipList["CONFIRMED_LEADER"] == 0 {
                    m.nextMembershipList["CONFIRMED_LEADER"] = 1
                }
                nextMembershipListLock.Unlock()
            } // switch
        } // select
    } // for
}

func (m *manager) updateMembershipList(mlist map[string]int64) (haveMore bool) {
    haveMore = false
    m.checkPeerTimeouts()

    nextMembershipListLock.Lock()
    defer nextMembershipListLock.Unlock()
    for addr, _ := range m.nextMembershipList {
        if isAddr(addr) && mlist[addr] == 0 {
            //have at least one node in nextMembershipList not in leader's list
            haveMore = true
            break
        }
    }

    for addr, timestamp := range mlist {
        if isAddr(addr) {
            if mlist[addr] > m.nextMembershipList[addr] {
                m.nextMembershipList[addr] = timestamp
            }
        }
    }

    return haveMore
}

func (m *manager) membershipBroadcastHandler() {
   sub, _ := m.psub.Subscribe("next_membership")

    for {
        got, _ := sub.Next(m.ctx)

        mlist := make(map[string]int64)
        json.Unmarshal(got.Data, &mlist)
        src, _ := peer.IDFromBytes(got.From)

        if src.String() != m.host.ID().String() {
            m.checkPeerTimeouts()

            nextMembershipListLock.Lock()
            for addr, timestamp := range mlist {
                if isAddr(addr) {
                    if mlist[addr] > m.nextMembershipList[addr] {
                        m.nextMembershipList[addr] = timestamp
                    }
                }
            }
            nextMembershipListLock.Unlock()
        }
    }
}

func countAddrs(mlist map[string]int64) int {
    cnt := 0
    for addr, _ := range mlist {
        if isAddr(addr) {
            cnt++
        }
    }
    return cnt
}

func (m *manager) reconcileNeeded(currMembership, nextMembership map[string]int64) bool {
    if currMembership["VIEW_ID"] == 0 {
        // swarm is initializing
        return false
    }

    // curr has "VIEW_ID" key and next has "VIEW_ID" and "INSTALL_VIEW" keys
    if countAddrs(currMembership) > countAddrs(nextMembership) {
        for addr, _ := range nextMembership {
            if isAddr(addr) {
                if _, exists := currMembership[addr]; !exists {
                    return true
                }
            }
        }

        // clean network split, no reconciliation needed
        // return false
        return true // TODO: fix this. right now, forcing force reconcile to see
                    // if needed. for merge event, reconcile provides both parent transactions
    }

    return true
}

func (m *manager) installMembershipView(mlist map[string]int64, leader string) {
    nextMembershipListLock.Lock()
    defer nextMembershipListLock.Unlock()

    var memberNodeIDs []int
    var memberLibp2pIDs []peer.ID
    reconcile := m.reconcileNeeded(m.membershipList, mlist)
    viewID := int(mlist["VIEW_ID"])
    m.membershipList = make(map[string]int64)
    m.nextMembershipList = make(map[string]int64)
    m.viewID = viewID
    m.membershipList["VIEW_ID"] = int64(viewID)
    m.nextMembershipList["VIEW_ID"] = int64(viewID + 1)

    for addr, _ := range mlist {
        if isAddr(addr) {
            m.membershipList[addr] = time.Now().Unix()
            m.nextMembershipList[addr] = time.Now().Unix()
            id, _ := peer.IDB58Decode(addr)
            memberNodeIDs = append(memberNodeIDs, m.libp2pIDs[id])
            memberLibp2pIDs = append(memberLibp2pIDs, id)
        }
    }

    // stopgap: only start new network when all nodes have booted
    if m.initializing {
        if len(memberNodeIDs) == m.swarmSize {
             m.initializing = false
             viewID = 1
             m.membershipList["VIEW_ID"] = 1
             m.nextMembershipList["VIEW_ID"] = 2
             m.log.Debug("Starting initial network")
        } else {
            return
        }
    }

    m.log.Debugf("***INSTALLED VIEW %d***", viewID)
    m.log.Debugf("member node IDs: %+v\n", memberNodeIDs)

    h := sha1.New()
    h.Write([]byte(leader + string(m.viewID)))
    m.membershipID = hex.EncodeToString(h.Sum(nil))[:6] // increase len if needed...

    var amLeader bool
    if m.host.ID().String() == leader {
        amLeader = true
    } else {
        amLeader = false
    }

    info := partition.NetworkInfo {
        ViewID: viewID,
        ChainID: m.membershipID,
        MemberNodeIDs: memberNodeIDs,
        Libp2pIDs: memberLibp2pIDs,
        AmLeader: amLeader,
        ReconcileNeeded: reconcile,
    }

    m.partManager.NewNetwork(info)
    // monitor.ReportEventRequest("172.16.0.254:32001", "http://0.0.0.0:8086",
    //     strconv.Itoa(m.conf.NodeID), m.membershipID)
}
