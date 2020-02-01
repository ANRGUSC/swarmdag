## Warning: partition_manager and tendermint is not ready for use.
## Requirements
install gorpc library to $GOPATH/src folder

https://github.com/valyala/gorpc.git


## run tendermint_main
go run tendermint_main.go

starts tendermint process and a server to listen for events from partition_management program


## run partition_management
client that sends "partition" string to the server once
the server than kills the existing tendermint process and starts a new tendermint process


## command to kill tendermint process
killall -9 tendermint
