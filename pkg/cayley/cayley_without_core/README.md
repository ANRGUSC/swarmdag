## Caley Tendermint

### Installation

This guide assumes a working tendermint instance and go 1.13.
GO111MODULE=on needs to be set.

Build the cayley app:
```
go build
```

Start tendermint and execute the created executable using:
```
TMHOME="/tmp/example" tendermint init
./cayley -config "/tmp/example/config/config.toml"
```


### Troubleshooting

A restart of tendermint might fail. A reset using the following commands
fixes this. All data in the ledger will be lost.
```
TMHOME="/tmp/example" tendermint unsafe_reset_all
rm -rf /tmp/example
```

### Usage

Info:
Values need to be surrounded with **< >**  
Example: subject = city, predicate = is, object = losangeles  
Leaving fields empty will set them to nil  
```
curl -s 'localhost:26657broadcast_tx_commit?tx="<city>=<is>=<losangeles>=<>"'
```

Adding a new values:
```
curl -s 'localhost:26657broadcast_tx_commit?tx="<subject>=<predicate>=<object>=<tag>"'
```


Returns all the stored data:
```
curl -s 'localhost:26657/abci_query?data="returnAll"'
```
Returns all values matching the subject and predicate
```
curl -s 'localhost:26657abci_query?data="<subject>=<predicate>"'
```
All queries will return all data semicolon-seperated encoded in **base64**
as the value in response.


Enabling incmoing transactions (enabled by default on start):
```
curl -s 'localhost:26657/abci_query?data="enableTx"'
```

Disable incoming transactions:
```
curl -s 'localhost:26657/abci_query?data="disableTx"'
```
