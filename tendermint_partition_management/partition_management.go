package main

import (
	"gorpc" //need to clone this repository https://github.com/valyala/gorpc.git
	"log"
)


func main() {
	c := &gorpc.Client{
	    // TCP address of the server.
	    Addr: "127.0.0.1:12345",
	}
	c.Start()
	// All client methods issuing RPCs are thread-safe and goroutine-safe,
	// i.e. it is safe to call them from multiple concurrently running goroutines.
	resp, err := c.Call("partition")
	if err != nil {
	    log.Fatalf("Error when sending request to server: %s", err)
	}
	log.Printf("Response is %+v",resp);
	if resp.(string) != "ok" {
	   log.Fatalf("Unexpected response from the server: %+v", resp)
	}
}


