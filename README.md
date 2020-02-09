## SwarmDAG: A Partition Tolerant Distributed Ledger Protocol for Swarm Robotics

TODO: setup instructions via Vagrant

### Requirements:

### Installation:

Pre-reqs: Install Docker 18.09 and Go 1.13+ and setup the following environment 
variables:

    export PATH=$PATH:/usr/local/go/bin
    export GOPATH=$HOME/go
    export PATH=$PATH:$GOPATH/bin

Follow the [Tendermint's procedures](https://tendermint.com/docs/introduction/install.html#from-binary)
for installing/building from source. Then, install swarmdag via `go get`. 

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

**WARNING: notes from here to InfluxDB are deprecated.**

`make build` will create a binary in the tendermint/build/ directory. This
will be revisited later.

Then, if this is your first compilation or you've made changes to the Tendermint 
Core:

    make build_tendermint

Then, build the SwarmDAG TM app and build the container

    make build-all

Alternatively, run `make build` to just rebuild the Docker image.

Pre-generate tendermint configs for nodes by running this locally. This will 
create a build/ folder with subfolders for each node. Modify the starting IP 
address in the Makefile in this directory depending on your docker's bridge IP.

    make reset-testnet


### InfluxDB Logging and Grafana Visualization

Launch InfluxDB locally via docker.

    docker run -p 8086:8086 -v $PWD:/var/lib/influxdb influxdb

Then, create a database simply called `mydb` (yes very creative) using curl in 
a terminal.

    curl -XPOST 'http://localhost:8086/query' --data-urlencode 'q=CREATE DATABASE "mydb"'

Launch Grafana locally via docker. The default username:password is `admin:admin`

    docker run -d --name=grafana -p 3000:3000 grafana/grafana

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