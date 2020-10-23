package membership

import (
	"strconv"
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
	"github.com/ANRGUSC/swarmdag/pkg/monitor"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	logging "github.com/op/go-logging"
    tmrand "github.com/tendermint/tendermint/libs/rand"
    "github.com/ANRGUSC/swarmdag/partition"
)

var isLeader bool = false

type nodeState int
 
const (
    FOLLOW_ANY nodeState = 0
    LEAD_PROPOSAL nodeState = 1
    FOLLOW_PROPOSER nodeState = 2
)

var nmlistLock sync.RWMutex

type Manager interface {
    OnStart()
    GetMembershipID() string
    ConnectHandler(network.Conn)
}

type Config struct {
    NodeID                      int
    MaxConnections              int
    BroadcastPeriod   time.Duration
    ProposeHeartbeatInterval    int

    ProposeTimerMin             int
    ProposeTimerMax             int   

    PeerTimeout                 int
    LeaderTimeout               int    
    FollowerTimeout             int  
    MajorityRatio               float32
}

type manager struct {
    conf                *Config

    ctx                 context.Context
    psub                *pubsub.PubSub
    host                host.Host
    notif               *network.NotifyBundle

    membershipID        string
    numConnections      int 
    neighborList        map[string]peer.ID
    membershipList      map[string]int64
    nextMembershipList  map[string]int64
    viewID              int64

    membershipBroadcastCh   chan string     
    proposeRoutineCh        chan string
    state                   nodeState
    log                     *logging.Logger
}

func NewManager(
    conf *Config, 
    ctx  context.Context, 
    host host.Host,
    psub *pubsub.PubSub, 
    log  *logging.Logger,
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

    mm := &manager {
        conf:                       conf,

        ctx:                        ctx,
        psub:                       psub,
        host:                       host,
        notif:                      &network.NotifyBundle{},

        numConnections:             0,
        neighborList:               make(map[string]peer.ID),
        membershipList:             make(map[string]int64),
        nextMembershipList:         make(map[string]int64),
        viewID:                     1, //must start with view ID 1

        membershipBroadcastCh:      make(chan string),
        proposeRoutineCh:           make(chan string, 2),
        state:                      FOLLOW_ANY,
        log: log,
    } 

    return mm
}

func (mm *manager) GetMembershipID() string {
    // TODO: also integrated into grpc
    // return mm.membershipID
    return tmrand.Str(6)
}

func (mm *manager) GetNodeID() int {
    // TODO: also integrated into grpc
    return 1
}

func (mm *manager) ConnectHandler(c network.Conn) {
    mm.log.Debug("rcvd remote conn. request from", c.RemotePeer())

    //TODO: this assumes that this node will accept all connection requests from
    //remote nodes. further optimization can be to deny connections to reduce
    //the degree of the network while still maintaining a neighbor list
    _, neighbor := mm.neighborList[c.RemotePeer().String()]

    if !neighbor {
        mm.neighborList[c.RemotePeer().String()] = c.RemotePeer()
    }
}

func (mm *manager) OnStart() {
    mm.host.Network().SetConnHandler(mm.ConnectHandler)
    peerChan := initMDNS(mm.ctx, mm.host, "TODO: changme", 5 * time.Second) 

    //TODO: fast initialization at start (forgot what this means?)
    mm.membershipList[mm.host.ID().String()] = time.Now().Unix()
    mm.membershipList["VIEW_ID"] = mm.viewID 
    mm.nextMembershipList["VIEW_ID"] = mm.viewID  + 1

    mm.notif.ConnectedF = func(n network.Network, c network.Conn) {
            // fmt.Println("NOTIFIEE CONNECTED")
    }

    mm.notif.DisconnectedF = func(n network.Network, c network.Conn) {
        mm.log.Debug("disconnected from peer:", c.RemotePeer())
    }

    mm.host.Network().Notify(mm.notif)

    go mm.membershipBroadcast()
    go mm.membershipBroadcastHandler()
    go mm.proposeRoutine()
    go mm.mdnsQueryHandler(peerChan)
}

func validAddr(addr string) bool {
    return (addr != "INSTALL_VIEW" && addr != "CONFIRMED_LEADER" && 
            addr != "PROPOSE" && addr != "VIEW_ID" && addr != "VOTE_TABLE")
}

func (mm *manager) mdnsQueryHandler(peerChan chan peer.AddrInfo) {
    for {
        p := <-peerChan // block until a mdns query is received
        // mm.log.Debug("mDNS query from:", p)

        _, neighbor := mm.neighborList[p.ID.String()]
        nmlistLock.Lock()
        _, member := mm.membershipList[p.ID.String()]

        if member {
            mm.membershipList[p.ID.String()] = time.Now().Unix()
        } 

        mm.nextMembershipList[p.ID.String()] = time.Now().Unix()

        nmlistLock.Unlock()

        //TODO use peer store to determine num connections
        if mm.numConnections < mm.conf.MaxConnections {
            //connect will only actually dial if a connection does not exist
            if err := mm.host.Connect(mm.ctx, p); err != nil {
                mm.log.Warning("Connection failed:", err)
            } 
            // mm.numConnections++ //TODO: implement counter
        }

        if !neighbor {
            mm.neighborList[p.ID.String()] = p.ID
        }
    }
}

func (mm *manager) checkPeerTimeouts() bool {
    changed := false

    nmlistLock.Lock()
    defer nmlistLock.Unlock()

    mm.nextMembershipList[mm.host.ID().String()] = time.Now().Unix()
    for addr, timestamp := range mm.nextMembershipList {
        if validAddr(addr) {
            if time.Now().Unix() - timestamp > int64(mm.conf.PeerTimeout) {
                delete(mm.nextMembershipList, addr)
                changed = true
            }
        }
    }


    return changed
}

func (mm *manager) membershipBroadcast() {
    broadcastTicker := time.NewTicker(mm.conf.BroadcastPeriod)
    for {
        select {
        case <-broadcastTicker.C:
            mm.checkPeerTimeouts()

            nmlistLock.RLock()
            nmlist, _ := json.Marshal(mm.nextMembershipList)
            nmlistLock.RUnlock()

            mm.psub.Publish("next_membership", nmlist)
        case cmd := <-mm.membershipBroadcastCh:
            switch cmd {
            case "START_BROADCAST":
                mm.log.Debug("start membership broadcast")
                broadcastTicker = time.NewTicker(mm.conf.BroadcastPeriod) 
            case "STOP_BROADCAST":
                mm.log.Debug("stop membership broadcast")
                broadcastTicker.Stop()
            }
        }
    }
}

func (mm *manager) voteTableBroadcast(voteTable *map[string]int, 
                                                voteTableBroadcastCh chan string) {
    //use the same period as membership broadcast
    ticker := time.NewTicker(mm.conf.BroadcastPeriod)
    mm.log.Debug("start vote table broadcasts")
    for {
        select {
        case <-ticker.C:
            vt, _ := json.Marshal(*voteTable)
            mm.psub.Publish("membership_propose", vt)
        case <-voteTableBroadcastCh:
            mm.log.Debug("stopping vote table broadcasts")
            //stop broadcasts
            return
        }
    }
}

// In voteTable, "1" means yes, "0" means no response, and "-1" means a NACK
func (mm *manager) leadProposal() {
    var src peer.ID
    var numNACKs int
    numVotes := 1 
    var msg *pubsub.Message
    newMsg := make(chan bool)
    done := make(chan bool)
    voteTable := make(map[string]int, len(mm.nextMembershipList))
    voteTableBroadcastCh := make(chan string)
    leadProposalTimeout := time.NewTicker(time.Duration(mm.conf.LeaderTimeout) * time.Second)

    //initiate empty table
    nmlistLock.RLock()
    for addr, _ := range mm.nextMembershipList {
        if validAddr(addr) {
            voteTable[addr] = 0
        }
    }
    voteTable["VOTE_TABLE"] = 1
    nmlistLock.RUnlock()

    voteTable[mm.host.ID().String()] = 1
    go mm.voteTableBroadcast(&voteTable, voteTableBroadcastCh)

    //follower nodes respond on this channel
    sub, _ := mm.psub.Subscribe(mm.host.ID().String()) 

    for {
        // the -2 accounts for the "PROPOSE" and "VIEW_ID" keys
        nmlistLock.RLock()
        nmlistLen := len(mm.nextMembershipList)
        nmlistLock.RUnlock()

        if (float32(numVotes) / float32(nmlistLen - 2)) > mm.conf.MajorityRatio {
            //let nodes know that votes confirm you are the leader
            mm.proposeRoutineCh<-"CONFIRMED_LEADER"

            if numVotes == (nmlistLen - 2) {
                voteTableBroadcastCh<-"STOP_VTABLE_BCAST"
                nmlistLock.Lock()
                delete(mm.nextMembershipList, "CONFIRMED_LEADER")
                delete(mm.nextMembershipList, "PROPOSE")
                mm.nextMembershipList["INSTALL_VIEW"] = 1
                nmlistLock.Unlock()
                //broadcasting continues in proposeRoutine()
                mm.launchMemberlist(mm.nextMembershipList)
                //TODO: when you return, memberlist should be populated
                mm.installMembershipView(mm.nextMembershipList, mm.host.ID().String())
                mm.proposeRoutineCh<-"RESET"
                return
            } 
        }

        go func(){
            msg, _ = sub.Next(mm.ctx)
            newMsg <- true
            <-done
        }()

        select {
        case <-leadProposalTimeout.C:
            mm.log.Debug("LEADER TIMEOUT")
            mm.proposeRoutineCh<-"RESET"
            voteTableBroadcastCh<-"STOP_VTABLE_BCAST"
            done <- true
            return
        case <-newMsg:
            done <- true
        }

        mlist := make(map[string]int64)
        json.Unmarshal(msg.Data, &mlist)
        src, _ = peer.IDFromBytes(msg.From)

        //check if response indicates a leader already exists. must check 
        //before NACK 
        if mlist["LEADER_EXISTS"] == 1 {
            mm.proposeRoutineCh<-"RESET"
            voteTableBroadcastCh<-"STOP_VTABLE_BCAST"
            return
        }

        //check if NACK message. if so, increment NACK count. Decrement ACK 
        //count if necessary
        if mlist["NACK"] == 1 {
            mm.log.Debug("received NACK")
            if voteTable[src.String()] == 1 {
                numVotes -= 1
            }
            voteTable[src.String()] = -1
            numNACKs += 1

            nmlistLock.RLock()
            nmlistLen := len(mm.nextMembershipList)
            nmlistLock.RUnlock()
            // the -1 accounts for the "PROPOSE" key
            if (float32(numNACKs) / float32(nmlistLen - 2)) > 
                    (1.0 - mm.conf.MajorityRatio) {
                mm.proposeRoutineCh<-"RESET"
                return
            }
            continue
        }
        
        //must be a propose membership ACK response
        nmlistLock.Lock()
        for addr, timestamp := range mlist {
            if validAddr(addr) {
                if mm.nextMembershipList[addr] < mlist[addr] {
                    mm.nextMembershipList[addr] = timestamp 
                }
            }
        }
        nmlistLock.Unlock()

        if voteTable[src.String()] == -1 {
            numNACKs -= 1
            voteTable[src.String()] = 1
        }

        numVotes += 1
    }
}

func (mm *manager) checkMembershipChange() bool {
    mm.checkPeerTimeouts()
    nmlistLock.Lock()
    defer nmlistLock.Unlock()

    nmlen := len(mm.nextMembershipList)
    mlen := len(mm.membershipList)
    invalid := []string{"INSTALL_VIEW", "CONFIRMED_LEADER", "PROPOSE", "VIEW_ID", "VOTE_TABLE"}

    for k := range invalid {
        if mm.nextMembershipList[invalid[k]] != 0 {
            nmlen = nmlen - 1
        }
        if mm.membershipList[invalid[k]] != 0 {
            mlen = mlen - 1
        }
    }

    if len(mm.nextMembershipList) == len(mm.membershipList) {
        for addr, _ := range mm.nextMembershipList {
            if validAddr(addr) {
                if mm.membershipList[addr] == 0 {
                    return true
                }
            }
        }
    } else {
        return true
    }

    return false
}

func (mm *manager) proposeRoutine() {
    rand.Seed(time.Now().UnixNano())
    proposeSub, _ := mm.psub.Subscribe("membership_propose") 
    rcvdMsg := make(chan bool)
    nextMsg := make(chan bool)
    var src peer.ID
    var leader string
    mlist := make(map[string]int64)
    nack := make(map[string]int64)
    nack["NACK"] = 1
    var followTimeo time.Ticker

    //sleep min time plus a random value up to the max at 10ms resolution
    proposeTicker := time.NewTicker(time.Duration(mm.conf.ProposeTimerMin * 1000 + 
        rand.Intn((mm.conf.ProposeTimerMax - mm.conf.ProposeTimerMin) * 100) * 10) * 
        time.Millisecond)

    proposeHeartbeat := time.NewTicker(10 * time.Second)
    proposeHeartbeat.Stop()

    go func(){
        for {
            got, _ := proposeSub.Next(mm.ctx)
            mlist = make(map[string]int64)
            json.Unmarshal(got.Data, &mlist)
            src, _ = peer.IDFromBytes(got.From)

            if src.String() != mm.host.ID().String() {
                rcvdMsg <- true
                <-nextMsg
            }
        }
    }()

    for {
        select {
        case <-proposeTicker.C:

            if mm.state == FOLLOW_ANY {
                if mm.checkMembershipChange() {
                    mm.log.Debugf("proposing membership update to view ID %d\n", mm.viewID + 1)
                    mm.state = LEAD_PROPOSAL
                    leader = mm.host.ID().String() //for creating membership ID
                    mm.log.Debug("state LEAD_PROPOSAL")
                    nmlistLock.Lock()
                    mm.nextMembershipList["PROPOSE"] = 1
                    nmlistLock.Unlock()
                    go mm.leadProposal()
                    proposeTicker.Stop()
                    proposeHeartbeat = time.NewTicker(time.Duration(mm.conf.ProposeHeartbeatInterval))
                }
            } else {
                mm.log.Debug("no changes detected, not proposing membership update")
            }

        case <-proposeHeartbeat.C:
            mm.checkPeerTimeouts()
            nmlistLock.RLock()
            nmlist, _ := json.Marshal(mm.nextMembershipList)
            mm.psub.Publish("membership_propose", nmlist)
            nmlistLock.RUnlock()

        case <-rcvdMsg:
            switch mm.state {
            case LEAD_PROPOSAL:
                if mlist["CONFIRMED_LEADER"] == 1 {
                    mm.proposeRoutineCh <- "RESET"
                }
                //already responded to a leader, NACK any other proposals
                if mlist["PROPOSE"] == 1 {
                    nackResp, _ := json.Marshal(nack)
                    mm.psub.Publish(src.String(), nackResp) 
                }
                break
            case FOLLOW_ANY:
                if mlist["PROPOSE"] == 1 && mlist["VIEW_ID"] > mm.membershipList["VIEW_ID"] {
                    mm.log.Debug("ACKing proposal for membership update to", src.String())

                    mm.updateMembershipList(mlist)
                    nmlistLock.RLock()
                    ackResp, _ := json.Marshal(mm.nextMembershipList)
                    nmlistLock.RUnlock()
                    mm.psub.Publish(src.String(), ackResp)
                    leader = src.String()
                    mm.state = FOLLOW_PROPOSER
                    proposeTicker.Stop()
                    followTimeo = *time.NewTicker(time.Duration(mm.conf.FollowerTimeout) * time.Second)
                    mm.log.Debug("state FOLLOW_PROPOSER")
                    mm.membershipBroadcastCh<-"STOP_BROADCAST"
                } 

            case FOLLOW_PROPOSER:
RESPOND_TO_PROPOSER:
                if src.String() == leader {
                    //only respond to vote tables
                    if mlist["VOTE_TABLE"] == 1 {
                        if mlist[mm.host.ID().String()] < 1 {
                            nmlistLock.RLock()
                            ackResp, _ := json.Marshal(mm.nextMembershipList)
                            mm.psub.Publish(src.String(), ackResp) 
                            nmlistLock.RUnlock()
                        }
                    }
                    if mlist["INSTALL_VIEW"] == 1  && mlist["VIEW_ID"] > mm.viewID {
                        mm.log.Debug("state FOLLOW_ANY")
                        mm.state = FOLLOW_ANY
                        followTimeo.Stop()
                        mm.installMembershipView(mlist, leader)
                        mm.proposeRoutineCh <- "RESET"
                        break
                    }
                    if mlist["PROPOSE"] == 1 {
                        if mm.updateMembershipList(mlist) {
                            mm.log.Debug("ACKing proposal for membership update to ", src.String())
                            nmlistLock.RLock()
                            ackResp, _ := json.Marshal(mm.nextMembershipList)
                            mm.psub.Publish(src.String(), ackResp) 
                            nmlistLock.RUnlock()
                        }
                    }
                } else {
                    if mlist["CONFIRMED_LEADER"] == 1 && mlist["VIEW_ID"] > mm.viewID {
                        leader = src.String()
                        goto RESPOND_TO_PROPOSER
                    }
                    //already responded to a leader, NACK any other proposals
                    if mlist["PROPOSE"] == 1 {
                        nackResp, _ := json.Marshal(nack)
                        mm.psub.Publish(src.String(), nackResp) 
                    }
                }
            } // switch

            nextMsg <- true

        case <-followTimeo.C:
            mm.log.Debug("FOLLOW_PROPOSER timeout!")
            mm.state = FOLLOW_ANY
            followTimeo.Stop()
            mm.proposeRoutineCh <- "RESET"

        case status := <-mm.proposeRoutineCh:
            switch status {
            case "RESET":
                mm.log.Debug("Resetting proposeRoutine()...")

                mm.state = FOLLOW_ANY
                mm.log.Debug("state FOLLOW_ANY")
                proposeHeartbeat.Stop()
                nmlistLock.Lock()
                delete(mm.nextMembershipList, "CONFIRMED_LEADER") //just in case
                delete(mm.nextMembershipList, "PROPOSE")
                delete(mm.nextMembershipList, "INSTALL_VIEW")
                nmlistLock.Unlock()

                proposeTicker = time.NewTicker(time.Duration(mm.conf.ProposeTimerMin * 1000 + 
                    rand.Intn((mm.conf.ProposeTimerMax - mm.conf.ProposeTimerMin) * 100) * 10) * 
                    time.Millisecond)
                mm.membershipBroadcastCh<-"START_BROADCAST"
            case "CONFIRMED_LEADER":
                mm.log.Debug("confirmed leader node")
                //include leader confirmation in membership broadcast
                nmlistLock.Lock()
                if mm.nextMembershipList["CONFIRMED_LEADER"] == 0 {
                    mm.nextMembershipList["CONFIRMED_LEADER"] = 1
                }
                nmlistLock.Unlock()
            } // switch
        } // select
    } // for
}

func (mm *manager) launchMemberlist(mlist map[string]int64) {
    nmlistLock.RLock()
    mm.log.Debugf("***waiting to install new membership view %d***", mlist["VIEW_ID"])
    nmlistLock.RUnlock()
    time.Sleep(1 * time.Second)
}

func (mm *manager) updateMembershipList(mlist map[string]int64) (haveMore bool) {
    haveMore = false
    mm.checkPeerTimeouts()

    nmlistLock.Lock()
    defer nmlistLock.Unlock()
    for addr, _ := range mm.nextMembershipList {
        if validAddr(addr) && mlist[addr] == 0 {
            //have at least one node in nextMembershipList not in leader's list
            haveMore = true
            break
        }
    }

    for addr, timestamp := range mlist {
        if validAddr(addr) {
            if mlist[addr] > mm.nextMembershipList[addr] {
                mm.nextMembershipList[addr] = timestamp 
            }
        }
    }

    return haveMore
}

func (mm *manager) membershipBroadcastHandler() {
   sub, _ := mm.psub.Subscribe("next_membership") 

    for {
        got, _ := sub.Next(mm.ctx)

        mlist := make(map[string]int64)
        json.Unmarshal(got.Data, &mlist)
        src, _ := peer.IDFromBytes(got.From)

        if src.String() != mm.host.ID().String() {
            mm.checkPeerTimeouts()

            nmlistLock.Lock()
            for addr, timestamp := range mlist {
                if validAddr(addr) {
                    if mlist[addr] > mm.nextMembershipList[addr] {
                        mm.nextMembershipList[addr] = timestamp 
                    }
                }
            }
            nmlistLock.Unlock()
        }
    }
}

func (mm *manager) installMembershipView(mlist map[string]int64, leader string) {
    nmlistLock.Lock()
    defer nmlistLock.Unlock()

    viewID := mlist["VIEW_ID"]
    mm.membershipList = make(map[string]int64)
    mm.nextMembershipList = make(map[string]int64)

    mm.viewID = viewID
    mm.membershipList["VIEW_ID"] = viewID
    mm.nextMembershipList["VIEW_ID"] = viewID + 1

    for addr, _ := range mlist {
        if validAddr(addr) {
            mm.membershipList[addr] = time.Now().Unix() 
            mm.nextMembershipList[addr] = time.Now().Unix() 
        }
    }

    mm.log.Debugf("***INSTALLED VIEW %d***", viewID)
    mm.log.Debug("membership: ", mlist)

    h := sha1.New()
    h.Write([]byte(leader + string(mm.viewID)))
    mm.membershipID = hex.EncodeToString(h.Sum(nil))

    info := partition.MembershipInfo {
        ID: mm.membershipID,
        ViewID: viewID, //TODO
        NodeIDs: {0},
        AmLeader: true,
    }

    // TODO: take specific notes on the packet characteristics.
    partition.NewNetwork(info)

    monitor.ReportEventRequest("172.16.0.254:32001", "http://0.0.0.0:8086",
        strconv.Itoa(mm.conf.NodeID), mm.membershipID)
    mID := []byte("replace me")
    mm.log.Debugf("reporting membership...%v", hex.EncodeToString(mID))
    mm.log.Debugf("my node ID is %d", mm.conf.NodeID)

    //TODO: make mID a variable
    //TODO: cross reference membershipList with neighbor list. join on ONLY a neighbor.
    //TODO: start new blockchain
}
