## kvstore - simple distributed BFT key-value store

Kvstore is a simple key-value store with the following features:
 * Add new key-value pairs to the ledger
 * Query all transactions stored in the ledger
 * Enable/Disable new incoming key-value 

### Installation:

This guide assumes a working tendermint instance and go 1.13.
GO111MODULE=on need to be set.

Build the kvstore app:
```
go build
```

Start tendermint and execute the created executable using:
```
TMHOME="/tmp/example" tendermint init
./kvstore -config "/tmp/example/config/config.toml"
```


### Troubleshooting

A restart of tendermint might fail. A reset using the following commands
fixes this. All data in the ledger will be lost.
```
TMHOME="/tmp/example" tendermint unsafe_reset_all
rm -rf /tmp/example
```

## Usage

Adding a new key-value pair to the ledger:
```
curl -s 'localhost:26657/broadcast_tx_commit?tx="city=losangeles"'
```

Returning all key-value paris stored in the ledger:
```
curl -s 'localhost:26657/abci_query?data="returnAll"'
```

Enabling incmoing transactions (enabled by default on start):
```
curl -s 'localhost:26657/abci_query?data="enableTx"'
```

Disable incoming transactions:
```
curl -s 'localhost:26657/abci_query?data="disableTx"'
```
