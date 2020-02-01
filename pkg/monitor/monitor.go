package monitor

import (
	"sync"
	sdk "github.com/grafana-tools/sdk"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"strings"
	"context"
	"net"
	"net/url"
	"time"
	"strconv"
	client "github.com/influxdata/influxdb1-client"
	pb "github.com/ANRGUSC/swarmdag/pkg/monitor/types"
)

type shortID int

type partInfo struct {
    members []int
    membershipID string
}

type partitionTable struct {
    nodeTable map[int]shortID   //maps node to partition short ID
    entry map[shortID]partInfo  
    freeShortIDs []shortID
    totalShortIDs int
}

var pTable partitionTable

var pTableLock sync.Mutex 

var s *grpc.Server

type reporter struct{}

func (s *reporter) ReportEvent(ctx context.Context, r *pb.ReportEventReq) (*pb.ReportEventReply, error) {
    pTableLock.Lock()
    nID, _ := strconv.Atoi(r.NodeID)
    sID := updatePartitionTable(nID, r.MembershipID)
    reportMembership(r.InfluxdbURL, r.NodeID, int(sID), r.MembershipID)
    pTableLock.Unlock()
    return &pb.ReportEventReply{Success: true},nil 
}

func getMembers(n int) []int {
    if sID, exist := pTable.nodeTable[n]; !exist {
        return []int{}
    } else {
        return pTable.entry[sID].members
    }
}

func freeShortID(sID shortID) {
    delete(pTable.entry, sID)
    pTable.freeShortIDs = append(pTable.freeShortIDs, sID)
}

func getNewShortID() shortID {
    if len(pTable.freeShortIDs) == 0 {
        pTable.totalShortIDs++
        return shortID(pTable.totalShortIDs - 1)
    } 

    var sID shortID
    sID, pTable.freeShortIDs = pTable.freeShortIDs[0], pTable.freeShortIDs[1:]
    return sID
}

func CreateDashboard(url string) error {
    board := sdk.NewBoard("Sample dashboard title")
    board.ID = 1
    row1 := board.AddRow("Sample row title")
    row1.Add(sdk.NewGraph("Sample graph"))
    graphWithDs := sdk.NewGraph("Sample graph 2")
    target := sdk.Target{
        RefID:      "A",
        Datasource: "Sample Source 1",
        Expr:       "sample request 1"}
    graphWithDs.AddTarget(&target)
    row1.Add(graphWithDs)
    // data, _ := json.MarshalIndent(board, "", "    ")
    // log.Infof("%s", data)

    c := sdk.NewClient(url, "admin:admin", sdk.DefaultHTTPClient)
    response, err := c.SetDashboard(*board, false)
    log.Errorf("string is %+v", response)
    if err != nil {
        if strings.Contains(*response.Message, "with the same name") {
            log.Warn(*response.Message)
            return nil
        } else {
            log.Errorf("error on uploading dashboard %s", board.Title)
            return err
        }
    } else {
        log.Infof("dashboard URL: %v", url+*response.URL)
        return nil
    }
}

func updatePartitionTable(nodeID int, membershipID string) shortID {
    needNew := true



    //if node is completely new to the network, add to table
    if _, exists := pTable.nodeTable[nodeID]; !exists {
        log.Debugf("Node %d added to network", nodeID)
        pTable.nodeTable[nodeID] = -1
    } else if len(getMembers(nodeID)) == 1 {
        // if node is last member of the partition, delete the partition
        log.Debugf("cleaning up shortID %d", pTable.nodeTable[nodeID])
       //delete old membership and free up that short id 
       freeShortID(pTable.nodeTable[nodeID])
       pTable.nodeTable[nodeID] = -1
    } else {
        // remove it from previous membership
        sID := pTable.nodeTable[nodeID]
        temp := pTable.entry[sID]
        for i, _ := range temp.members {
            if temp.members[i] == nodeID {
                temp.members[i] = temp.members[len(temp.members) - 1]
                temp.members = temp.members[:len(temp.members) - 1]
                break
            }
        }
        pTable.entry[sID] = partInfo {
            members: temp.members,
            membershipID: pTable.entry[sID].membershipID,
        }
    }

    //join node to partition if it already exists, else create one
    for sID, pInfo := range pTable.entry {
        if pInfo.membershipID == membershipID {
            pTable.nodeTable[nodeID] = sID
            pTable.entry[sID] = partInfo {
                members: append(pInfo.members, nodeID),
                membershipID: pInfo.membershipID,
            }
            needNew = false
            break
        }
    }

    if needNew {
        sID := getNewShortID()
        pTable.nodeTable[nodeID] = sID
        pInfo := partInfo {
            members: make([]int, 1), 
            membershipID: membershipID,
        }
        pInfo.members[0] = nodeID
        pTable.entry[sID] = pInfo
        log.Debugf("Created new partition membershipID %v and shortID %d", 
                   membershipID, sID) 
    }

    return pTable.nodeTable[nodeID]
}

func reportMembership(influxdbURL string, nodeID string, shortID int, 
                      membershipID string) {
    host, err := url.Parse(influxdbURL)
    if err != nil {
        log.Fatal(err)
    }
    con, err := client.NewClient(client.Config{URL: *host})
    if err != nil {
        log.Fatal(err)
    }

    //TODO: extend function to allow for batched writes
    sampleSize := 1
    pts := make([]client.Point, sampleSize)

    for i := 0; i < sampleSize; i++ {
        pts[i] = client.Point{
            Measurement: "partition",
            Tags: map[string]string{
                "node": nodeID,
            },
            Fields: map[string]interface{}{
                "membershipID": membershipID,
                "shortID": shortID,
                "nodeID": nodeID,
            },
            Time:      time.Now(),
        }
    }

    bps := client.BatchPoints{
        Points:          pts,
        Database:        "mydb",
        RetentionPolicy: "autogen",
    }
    _, err = con.Write(bps)
    if err != nil {
        log.Error(err)
    }
}

func startServer() {
    ln, err := net.Listen("tcp", "0.0.0.0:32001")
    if err!=nil{
        log.Fatalf("%s\n",err)
    }
    //First Create a new gRPC Server 
    s = grpc.NewServer()
    //Reigster our new server as gRPC
    pb.RegisterReporterServer(s, &reporter{})

    go func() {
        if err := s.Serve(ln); err != nil {
            log.Errorf("failed to serve: %v", err)
        }

        log.Infof("gRPC server shut down.")
    } ()
}

func ShutdownMonitor() {
    log.Infof("stopping gRPC server...")
    s.Stop()
}

func InitMonitor() {
    pTable.nodeTable = make(map[int]shortID)
    pTable.entry = make(map[shortID]partInfo)
    pTable.freeShortIDs = make([]shortID, 1)
    pTable.freeShortIDs[0] = 0
    pTable.totalShortIDs = 1

    startServer()
}
