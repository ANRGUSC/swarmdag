package membership

import (
	"encoding/hex"
	"crypto/sha1"
	"context"
	"time"
	"encoding/json"
	"math/rand"
	"sync"
    "sort"

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
    ProposeHeartbeatInterval    time.Duration

    ProposeTimerMin             int
    ProposeTimerMax             int

    PeerTimeout                 time.Duration
    LeaderTimeout               time.Duration
    FollowerTimeout             time.Duration
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

    leadingProposalCh       chan chan *pubsub.Message
}

type directMsg struct {
    MsgType string          `json:"MsgType"`
    InstallView int64       `json:"INSTALL_VIEW"`
    ConfirmedLeader int64   `json:"CONFIRMED_LEADER"`
    Propose int64           `json:"PROPOSE"`
    ViewID  int64           `json:"VIEW_ID"`
    List map[string]int64
}

func parseDirectMsg(in []byte) directMsg {
    var dm directMsg
    json.Unmarshal(in, &dm)
    json.Unmarshal(in, &dm.List)
    delete(dm.List, "INSTALL_VIEW")
    delete(dm.List, "CONFIRMED_LEADER")
    delete(dm.List, "PROPOSE")
    delete(dm.List, "VIEW_ID")
    return dm
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
        conf.ProposeHeartbeatInterval = 300 * time.Millisecond
    }

    if conf.ProposeTimerMax == 0 {
       conf.ProposeTimerMax = 4
    }

    if conf.PeerTimeout == 0 {
        conf.PeerTimeout = 1000 * time.Millisecond
    }

    if conf.LeaderTimeout == 0 {
        conf.LeaderTimeout = 5 * time.Second
    }

    if conf.FollowerTimeout == 0 {
        conf.FollowerTimeout = 2 * time.Millisecond
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

        leadingProposalCh:          make(chan chan *pubsub.Message),
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
    peerChan := initMDNS(m.ctx, m.host, "rendezvousStr", 1000 * time.Millisecond)
    m.membershipList[m.host.ID().String()] = time.Now().UnixNano()
    m.membershipList["VIEW_ID"] = int64(m.viewID)
    m.nextMembershipList["VIEW_ID"] = int64(m.viewID + 1)
    m.notif.ConnectedF = func(n network.Network, c network.Conn) {
            m.log.Debug("NOTIFIEE CONNECTED", c.RemoteMultiaddr().String())
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
    switch addr {
    case "INSTALL_VIEW":
        break
    case "CONFIRMED_LEADER":
        break
    case "PROPOSE":
        break
    case "VIEW_ID":
        break
    case "VOTE_TABLE":
        break
    default:
        return true
    }
    return false
}

func memberCount(mlist map[string]int64) int {
    cnt := 0
    for addr, _ := range mlist {
        if isAddr(addr) {
            cnt++
        }
    }
    return cnt
}

func (m *manager) mdnsQueryHandler(peerChan chan peer.AddrInfo) {
    for {
        p := <-peerChan // block until a mdns query is received
        _, neighbor := m.neighborList[p.ID.String()]
        nextMembershipListLock.Lock()
        m.nextMembershipList[p.ID.String()] = time.Now().UnixNano()
        nextMembershipListLock.Unlock()

        //connect will only actually dial if a connection does not exist
        if m.host.Network().Connectedness(p.ID) == network.NotConnected {
            if err := m.host.Connect(m.ctx, p); err != nil {
                m.log.Warning("Connection failed:", err)
            }
        }
        if !neighbor {
            m.neighborList[p.ID.String()] = p.ID
        }
    }
}

func (m *manager) checkPeerTimeouts() bool {
    nextMembershipListLock.Lock()
    defer nextMembershipListLock.Unlock()
    m.nextMembershipList[m.host.ID().String()] = time.Now().UnixNano()
    changed := false
    for addr, timestamp := range m.nextMembershipList {
        if isAddr(addr) {
            if time.Now().UnixNano() - timestamp > m.conf.PeerTimeout.Nanoseconds() {
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
            c := []byte("{\"PROPOSE\": -1}")
            m.psub.Publish("membership_propoose", c) // FIXME: put me in a better place?
            return
        }
    }
}

func (m *manager) directMsgDispatcher() {
    var leadCh chan *pubsub.Message
    msgCh := make(chan *pubsub.Message)
    sub, _ := m.psub.Subscribe(m.host.ID().String())

    go func() {
        for {
            msg, err := sub.Next(m.ctx)
            if err != nil {
                close(msgCh)
                return
            }
            msgCh <- msg
        }
    }()

    for {
        select {
        case msg := <-msgCh:
            var ack directMsg
            json.Unmarshal(msg.Data, &ack)
            if leadCh != nil {
                select {
                case leadCh <- msg:
                default:
                }
            }
        case leadCh = <-m.leadingProposalCh:
        case <-m.ctx.Done():
            for range msgCh {}
            return
        }
    }

}

// In voteTable, "1" means yes, "0" means no response, and "-1" means a NACK
func (m *manager) leadProposal(ctx context.Context) {
    var numNACKs int
    numVotes := 1
    voteTable := make(map[string]int, len(m.nextMembershipList))
    dmCh := make(chan *pubsub.Message, 16)
    var vtLock sync.Mutex
    voteCtx, cancelVote := context.WithCancel(ctx)
    leadProposalTimeout := time.NewTicker(m.conf.LeaderTimeout)
    m.log.Debug("leader timeout is ", m.conf.LeaderTimeout)
    m.leadingProposalCh <- dmCh
    defer func() {
        cancelVote()
        m.proposeRoutineCh <- "RESET"
        m.leadingProposalCh <- nil
    } ()

    //initiate empty table
    nextMembershipListLock.RLock()
    for addr, _ := range m.nextMembershipList {
        if isAddr(addr) {
            voteTable[addr] = 0
        }
    }
    voteTable["VOTE_TABLE"] = 1
    voteTable["VIEW_ID"] = int(m.nextMembershipList["VIEW_ID"])
    nextMembershipListLock.RUnlock()

    voteTable[m.host.ID().String()] = 1
    go m.voteTableBroadcast(&voteTable, &vtLock, voteCtx)

    for {
        nextMembershipListLock.RLock()
        vtLock.Lock()
        memberCnt := memberCount(m.nextMembershipList)
        for addr, _ := range m.nextMembershipList {
            if isAddr(addr) {
                if _, exist := voteTable[addr]; !exist {
                    voteTable[addr] = 0
                }
            }
        }
        for addr, _ := range voteTable {
            if isAddr(addr) {
                if _, exist := m.nextMembershipList[addr]; !exist {
                    delete(voteTable, addr)
                }
            }
        }
        vtLock.Unlock()
        nextMembershipListLock.RUnlock()

        if (float32(numVotes) / float32(memberCnt)) > m.conf.MajorityRatio {
            //let nodes know that votes confirm you are the leader
            // fix: this occurs for each vote message
            m.proposeRoutineCh<-"CONFIRMED_LEADER"
            if numVotes == memberCnt {
                installList := make(map[string]int64)
                nextMembershipListLock.RLock()
                for k, v := range m.nextMembershipList {
                    installList[k] = v
                }
                nextMembershipListLock.RUnlock()
                installList["INSTALL_VIEW"] = 1
                installMsg, _ := json.Marshal(installList)

                // stop-gap for "membership_propose" topic to propagate
                m.psub.Publish("membership_propose", installMsg)
                time.Sleep(250 * time.Millisecond)
                m.psub.Publish("membership_propose", installMsg)
                m.log.Debug("installList: ", string(installMsg))
                m.installMembershipView(installList, m.host.ID().String())
                return
            }
        }

        select {
        case <-ctx.Done():
            return
        case <-leadProposalTimeout.C:
            m.log.Debug("LEADER TIMEOUT")
            return
        case dm := <-dmCh:
            msg := parseDirectMsg(dm.Data)
            src, _ := peer.IDFromBytes(dm.From)

            switch msg.MsgType {
            case "nackResp":
                vtLock.Lock()
                switch voteTable[src.String()] {
                case -1:
                    // already got NACK from this node
                case 0:
                    numNACKs += 1
                    voteTable[src.String()] = -1
                    m.log.Debug("received NACK")
                case 1:
                    numVotes -= 1
                    voteTable[src.String()] = -1
                    numNACKs += 1
                    m.log.Debug("received NACK")
                }
                vtLock.Unlock()

                nextMembershipListLock.RLock()
                // the -2 accounts for the "PROPOSE" and "VIEW_ID" keys
                memberCnt := memberCount(m.nextMembershipList)
                nextMembershipListLock.RUnlock()
                if (float32(numNACKs) / float32(memberCnt)) >
                        (1.0 - m.conf.MajorityRatio) {
                    m.log.Info("cancelling leadProposal()")
                    return
                }
                continue
            case "ackResp":
                //must be a propose membership ACK response
                nextMembershipListLock.Lock()
                for addr, timestamp := range msg.List {
                    if m.nextMembershipList[addr] < msg.List[addr] {
                        m.nextMembershipList[addr] = timestamp
                    }
                }
                nextMembershipListLock.Unlock()

                vtLock.Lock()
                switch voteTable[src.String()] {
                case -1:
                    numNACKs -= 1
                    voteTable[src.String()] = 1
                    m.log.Debugf("ACK from node %d \n", m.libp2pIDs[src])
                    numVotes += 1
                    m.log.Debugf("member count %d\n", memberCnt)
                case 0:
                    voteTable[src.String()] = 1
                    m.log.Debugf("ACK from node %d\n", m.libp2pIDs[src])
                    m.log.Debugf("member count %d\n", memberCnt)
                    numVotes += 1
                case 1:
                    // do nothing
                }
                vtLock.Unlock()
            } // switch msg.Type
        } // select
    }

}

func (m *manager) membershipChanged() bool {
    m.checkPeerTimeouts()
    nextMembershipListLock.Lock()
    defer nextMembershipListLock.Unlock()
    for addr, _ := range m.nextMembershipList {
        if isAddr(addr) {
            if _, exists := m.membershipList[addr]; !exists {
                return true
            }
        }
    }
    for addr, _ := range m.membershipList {
        if isAddr(addr) {
            if _, exists := m.nextMembershipList[addr]; !exists {
                return true
            }
        }
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

// This routine runs indefinitely. It decides if and when to propose a
// membership update or follow a leader that is proposing.
func (m *manager) proposeRoutine() {
    proposeSub, _ := m.psub.Subscribe("membership_propose")
    rand.Seed(time.Now().UnixNano())
    rcvdMsg := make(chan bool)
    nextMsg := make(chan bool)
    var src peer.ID
    var leader string
    mlist := make(map[string]int64)
    nack := directMsg{MsgType: "nackResp"}

    //sleep min time plus a random value up to the max at 100ms resolution
    interval := (m.conf.ProposeTimerMax - m.conf.ProposeTimerMin) * 1000 / 100
    tickerTime := time.Duration(rand.Intn(interval) * 100) * time.Millisecond +
                  time.Duration(m.conf.ProposeTimerMin) * time.Second
    proposeTicker := time.NewTicker(tickerTime)

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

            if src != m.host.ID() {
                rcvdMsg <- true
                <-nextMsg
            }
        }
    }()

    var leadCtx context.Context
    var leadCancel context.CancelFunc

    for {
        select {
        case <-proposeTicker.C:
            if m.membershipChanged() {
                proposeTicker.Stop()
                m.log.Debugf("proposing membership update to view ID %d\n", m.viewID + 1)
                m.state = LEAD_PROPOSAL
                leader = m.host.ID().String() //for creating membership ID
                m.log.Debug("state LEAD_PROPOSAL")
                nextMembershipListLock.Lock()
                m.nextMembershipList["PROPOSE"] = 1
                nextMembershipListLock.Unlock()
                leadCtx, leadCancel = context.WithCancel(m.ctx)
                go m.leadProposal(leadCtx)
                proposeHeartbeat = time.NewTicker(m.conf.ProposeHeartbeatInterval)
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
                    leadCancel()
                }

                // another leader asking to install view
                if mlist["INSTALL_VIEW"] == 1 && mlist["VIEW_ID"] > int64(m.viewID) {
                    if mlist[m.host.ID().String()] > 0 {
                        if m.updateMembershipList(mlist) {
                            m.log.Error("ERR: leader said to install but I have more nodes!")
                        }
                        m.log.Debug("state FOLLOW_ANY")
                        m.state = FOLLOW_ANY
                        m.installMembershipView(mlist, src.String())
                    }
                    leadCancel()
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
                    proposeTicker.Stop()
                    m.log.Debug("ACKing proposal for membership update to node", m.libp2pIDs[src])
                    m.updateMembershipList(mlist)
                    nextMembershipListLock.RLock()
                    ackResp, _ := json.Marshal(createAck(m.nextMembershipList))
                    nextMembershipListLock.RUnlock()
                    m.psub.Publish(src.String(), ackResp)
                    leader = src.String()
                    m.state = FOLLOW_PROPOSER
                    followTimeo = time.NewTicker(m.conf.FollowerTimeout)
                    m.log.Debugf("state FOLLOW_PROPOSER, leader: node %d\n", m.libp2pIDs[src])
                }
                // fixme: respond to out of sync node? (trying to propose lower viewID, etc)
            case FOLLOW_PROPOSER:
RESPOND_TO_PROPOSER:
                if src.String() == leader && mlist["VIEW_ID"] > int64(m.viewID) {
                    if mlist["VOTE_TABLE"] == 1 && mlist[m.host.ID().String()] < 1 {
                        nextMembershipListLock.RLock()
                        ackResp, _ := json.Marshal(createAck(m.nextMembershipList))
                        nextMembershipListLock.RUnlock()
                        m.psub.Publish(src.String(), ackResp)
                        m.log.Warningf("ACKING LEADER VOTE TABLE\n")
                    }
                    if mlist["INSTALL_VIEW"] == 1 {
                        if _, exists := mlist[m.host.ID().String()]; exists {
                            if m.updateMembershipList(mlist) {
                                m.log.Error("ERR: have more in mlist than leader")
                            }
                            m.log.Debug("state FOLLOW_ANY")
                            m.state = FOLLOW_ANY
                            followTimeo.Stop()
                            m.installMembershipView(mlist, leader)
                            m.proposeRoutineCh <- "RESET"
                            break
                        }
                    }
                    if mlist["PROPOSE"] == 1 {
                        if m.updateMembershipList(mlist) {
                            nextMembershipListLock.RLock()
                            ackResp, _ := json.Marshal(createAck(m.nextMembershipList))
                            nextMembershipListLock.RUnlock()
                            m.psub.Publish(src.String(), ackResp)
                        }
                    } else if mlist["PROPOSE"] == -1 {
                        m.log.Debug("leader cancelled")
                        m.state = FOLLOW_ANY
                        followTimeo.Stop()
                        m.proposeRoutineCh <- "RESET"
                        break
                    }
                    followTimeo = time.NewTicker(m.conf.FollowerTimeout)
                } else {
                    if mlist["CONFIRMED_LEADER"] == 1 && mlist["VIEW_ID"] > int64(m.viewID) {
                        m.log.Info("Switched leader to node", m.libp2pIDs[src])
                        leader = src.String()
                        goto RESPOND_TO_PROPOSER
                    }
                    //already responded to a leader, NACK any other proposals
                    if mlist["PROPOSE"] == 1 {
                        m.log.Warning("sent nack, already following someone")
                        nackResp, _ := json.Marshal(nack)
                        m.psub.Publish(src.String(), nackResp)
                    }
                }
            } // switch, <-rcvdMsg
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
                m.log.Debug("state FOLLOW_ANY")
                m.state = FOLLOW_ANY
                proposeHeartbeat.Stop()
                nextMembershipListLock.Lock()
                delete(m.nextMembershipList, "CONFIRMED_LEADER") //just in case
                delete(m.nextMembershipList, "PROPOSE")
                delete(m.nextMembershipList, "INSTALL_VIEW")
                nextMembershipListLock.Unlock()
                interval = (m.conf.ProposeTimerMax - m.conf.ProposeTimerMin) * 1000 / 100
                tickerTime = time.Duration(rand.Intn(interval) * 100) * time.Millisecond +
                              time.Duration(m.conf.ProposeTimerMin) * time.Second
                proposeTicker = time.NewTicker(tickerTime)
                m.log.Debug("proposeTicker: ", tickerTime)
                leader = ""
            case "CONFIRMED_LEADER":
                //include leader confirmation in membership broadcast
                nextMembershipListLock.Lock()
                if m.nextMembershipList["CONFIRMED_LEADER"] == 0 {
                    m.log.Debug("confirmed leader node")
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

    for addr, _ := range m.membershipList {
        if isAddr(addr) {
            if _, exists := m.nextMembershipList[addr]; !exists {
                p, _ := peer.IDB58Decode(addr)
                m.host.Network().ClosePeer(p)
            }
        }
    }

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
            m.membershipList[addr] = time.Now().UnixNano()
            m.nextMembershipList[addr] = time.Now().UnixNano()
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

    h := sha1.New()
    h.Write([]byte(leader + string(m.viewID)))
    m.membershipID = hex.EncodeToString(h.Sum(nil))[:8] // increase len if needed...

    var amLeader bool
    if m.host.ID().String() == leader {
        amLeader = true
    } else {
        amLeader = false
    }

    nextMembershipListLock.Unlock()

    info := partition.NetworkInfo {
        ViewID: viewID,
        ChainID: m.membershipID,
        MemberNodeIDs: memberNodeIDs,
        Libp2pIDs: memberLibp2pIDs,
        AmLeader: amLeader,
        ReconcileNeeded: reconcile,
    }

    err := m.partManager.NewNetwork(info)
    if err != nil {
        nextMembershipListLock.Lock()
        // prepare internal state to redo membership update
        m.membershipList = make(map[string]int64)
        m.membershipList["VIEW_ID"] = int64(viewID - 1)
        m.viewID = viewID - 1
        m.nextMembershipList["VIEW_ID"] = int64(viewID)
        m.log.Info("Membership installation failed")
        nextMembershipListLock.Unlock()
        return
    }
    sort.IntSlice(memberNodeIDs).Sort()
    m.log.Debugf("************** INSTALLED VIEW %d **************", viewID)
    m.log.Debugf("member node IDs: %+v\n", memberNodeIDs)
    // monitor.ReportEventRequest("172.16.0.254:32001", "http://0.0.0.0:8086",
    //     strconv.Itoa(m.conf.NodeID), m.membershipID)
}
