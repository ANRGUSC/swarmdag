package main

import (
    "gorpc" //need to clone this repository https://github.com/valyala/gorpc.git
    "fmt"
    "os/exec"
    "syscall"
    "os"
    "log"
)

func new_tendermint_process() *exec.Cmd {
    cmd := exec.Command("tendermint", "node")
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
    cmd.Stdin = os.Stdin;
    cmd.Stdout = os.Stdout;
    cmd.Stderr = os.Stderr;
    err := cmd.Start()
    if err != nil {
        fmt.Printf("%v\n", err)
    }
    return cmd
}

func main() {
    cmd:= new_tendermint_process()    
    s := &gorpc.Server{
        // Accept clients on this TCP address.
        Addr: ":12345",

        // Echo handler - just return back the message we received from the client
        Handler: func(clientAddr string, request interface{}) interface{} {
            log.Printf("Obtained request %+v from the client %s\n", request, clientAddr)
             
            if(request.(string) == "partition") { 
                syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
                cmd.Wait()
                fmt.Print("tendermint process killed \n")
                fmt.Print("Extracting commited blocks from database \n")
                fmt.Print("launching new tendermint process\n")
                cmd = new_tendermint_process()
            }
            return "ok"
        },
    }
    if err := s.Serve(); err != nil {
        log.Fatalf("Cannot start rpc server: %s", err)
    }
}


