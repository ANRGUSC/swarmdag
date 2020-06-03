## SwarmDAG: A Partition Tolerant Distributed Ledger Protocol for Swarm Robotics

TODO: setup instructions via Vagrant

### Requirements:

### Installation:

Pre-reqs: Install Docker 18.09 and Go 1.13+ and setup the following environment 
variables:

    export PATH=$PATH:/usr/local/go/bin
    export GOPATH=$HOME/go
    export PATH=$PATH:$GOPATH/bin

Install swarmdag via `go get`. 

    go get git@github.com:ANRGUSC/swarmdag.git

If the repository is private, you need to force github to clone via SSH instead
of HTTP by default. In your `~/.gitconfig` file, append these two lines.


```
[url "git@github.com:"]
    insteadOf = https://github.com/
```

Because the repository is private, you may run into some issues with go modules.
Add the following environment variable to tell Go that you have a private repo.
Add this to your .bashrc to make it permanent.

    export GOPRIVATE="github.com/ANRGUSC"

You may also run into issues with go-libp2p versioning because of the new global
proxy for Go modules (goproxy). You can disable this by setting the following
environment variable.

    export GOPROXY=direct

### Developing using Docker Containers

The following is a workflow for using Docker for development. The main way to
run SwarmDAG is to use COREEMU.

First, build the docker container if you have changed it. The Dockerfile should
be kept simple by mounting all application files into the container.

    make build-docker

Then, build and run via docker-compose by invoking

    make all 

There are additional make targets in the top level Makefile for use in 
development (read the targets to understand the build flow. To remove all build 
files, run

    make clean

### InfluxDB Logging and Grafana Visualization

Launch InfluxDB locally via docker.

    docker run -p 8086:8086 -v $PWD:/var/lib/influxdb influxdb

Then, create a database simply called `mydb` (yes very creative) using curl in 
a terminal.

    curl -XPOST 'http://localhost:8086/query' --data-urlencode 'q=CREATE DATABASE "mydb"'

Launch Grafana locally via docker. The default username:password is `admin:admin`

    docker run -d --name=grafana -p 3000:3000 grafana/grafana

Go to `grafana-tools/` for instructions on how to import template dashboard and
datasources.

The next steps must be done exactly to match the dashboard config saved in this
repository. Log into the grafana UI at `localhost:3000` and click on the gear 
icon to go to Configuration->Data Sources. Add an InfluxDB datasource and set 
the following properties.

 * Name: InfluxDB
 * URL: http://localhost:8086
 * Access: Browser
 * Basic auth and With Credentials: OFF
 * Database: mydb
 * HTTP Method: GET

Hit "Save & Test" to see if it's working. Then load the exported dashboard
configuration file `./tm_app/monitor/partition_test_dashboard.json` by clicking
the "+" icon and going to "Import".

Next, launch the "monitor" which is currently done inside a test. Go to 
`tm_app/monitor/` and run

    go test -run TestReport

Lastly, run the CORE script to launch a partitoning network.

    cd tm_app/coreemulator
    sudo python3.6 partition_net.py

Next, go to the ./monitor/ directory and compile report.proto by installing 
protoc-gen-go (make sure to get `protoc` version 3+) and executing in terminal:
    
    protoc --go_out=plugins=grpc:. report.proto 
