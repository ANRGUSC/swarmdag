package monitor

import (
    "context"
    "google.golang.org/grpc"
    log "github.com/sirupsen/logrus"
    pb "github.com/ANRGUSC/swarmdag/pkg/monitor/types"
)

func ReportEventRequest(monitorAddr string, influxdbURL string, nodeID string, 
                        membershipID string) {
    var serv string
    if monitorAddr == "" {
        serv = "0.0.0.0:32001"
    } else {
        serv = monitorAddr //should be 172.16.0.254:32001
    }
    conn, err := grpc.Dial(serv, grpc.WithInsecure())

    if err != nil{
        log.Fatalf("%s\n", err)
    }
    defer conn.Close()
    c:= pb.NewReporterClient(conn)

    r, err:= c.ReportEvent(context.Background(), &pb.ReportEventReq{
            InfluxdbURL: influxdbURL,
            NodeID: nodeID,
            MembershipID: membershipID,
        })

    log.Info(r)

    if err!=nil{
        log.Fatalf("%s\n",err)
    }
}