// +build go1.11

package main

import (
    "fmt"
    "time"
    "net"

    "github.com/hashicorp/memberlist"
)


func main() {

    /* Create the initial memberlist from a safe configuration.
       Please reference the godoc for other default config types.
       http://godoc.org/github.com/hashicorp/memberlist#Config
    */
    eventCh := make(chan memberlist.NodeEvent, 5)
    //increment i for each new terminal
    iface, err := net.InterfaceByName("eth0")

    if err != nil {
         fmt.Print(err)
         return
    }

    var i int = 0

    c := memberlist.DefaultLANConfig()
    c.ProbeInterval = 1000 * time.Millisecond
    c.ProbeTimeout = 5000 * time.Millisecond
    c.GossipInterval = 1000 * time.Millisecond
    c.PushPullInterval = 5000 * time.Millisecond
    c.SuspicionMult = 10
    
    addrs, _ := iface.Addrs()
    for _, addr := range addrs {
        if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
            if ipnet.IP.To4() != nil {
                myaddr := ipnet.IP.String()
                fmt.Printf("my ipv4 addr is: %s\n", myaddr)
                //for now determine node number (and listen port) by last ip 
                //addr field, TODO: need scalable solution for this
                i =  int(ipnet.IP.To4()[3]) - 2
                c.BindAddr = myaddr
                c.BindPort = 12345 + i
                c.Name = fmt.Sprintf("%s:%d", myaddr, 12345+i)
            } 
        } else {
            fmt.Println("error finding interface addr")
            return
        }
    }


    if i == 0 {
            c.Events = &memberlist.ChannelEventDelegate{eventCh}
    }

    m, err := memberlist.Create(c)
    if err != nil {
        fmt.Printf("unexpected err: %s", err)
    }
    defer m.Shutdown()

    //when using blockade, docker will assign IP addrs starting at x.x.x.2
    //and that node will be the bootstrapping node for initiating memberlist
    if i > 0 {
        for {
            //Known address of first node, container c0
            _, err := m.Join([]string{"172.17.0.2:12345"})
            if err != nil {
                fmt.Printf("unexpected err: %s", err)
            } else {
                break
            }

            time.Sleep(1)
        }
    }

    // Ask for members of the cluster
    for {
        for _, member := range m.Members() {
            fmt.Printf("Member: %s %s\n", member.Name, member.Addr)
        }

        time.Sleep(time.Second * 5)
    }

//     breakTimer := time.After(250 * time.Millisecond)
// WAIT:
//     for {
//         select {
//         case e := <-eventCh:
//             if e.Event == memberlist.NodeJoin {
//                 fmt.Printf("[DEBUG] Node join")
//             } else {
//                 fmt.Printf("[DEBUG] Node leave:")
//             }
//         case <-breakTimer:
//             break WAIT
//         }
//     }

    select {} //wait here

    // Continue doing whatever you need, memberlist will maintain membership
    // information in the background. Delegates can be used for receiving
    // events when members join or leave.
}
