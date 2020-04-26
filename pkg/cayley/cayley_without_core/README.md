## Cayley Tendermint

### Installation

This guide assumes a working tendermint instance and go 1.13.
GO111MODULE=on needs to be set.

Build the cayley app:
```
go build
```

Start tendermint and execute the created executable using:
```
tendermint node --proxy_app=127.0.0.1:26658
./cayley
```


### Troubleshooting

A restart of tendermint might fail. A reset using the following command
fixes this. All data in the ledger will be lost.
```
tendermint unsafe_reset_all
```

### Usage

Info:
Values need to be surrounded with **< >**  

Add a new transaction with a key value pair
```
curl -s 'localhost:26657/broadcast_tx_commit?tx="<key>=<value>"'
```

Adding a new values using JSON. The JSON String **MUST** be base64 encoded beforehand.
```
curl --data-binary '{"jsonrpc":"2.0","id":"","method":"broadcast_tx_commit","params": {"tx": "(REPLACE WITH BASE64 JSON STRING)"}}' -H 'content-type:application/json;' http://localhost:26657
```

All queries will return all data semicolon-seperated encoded in **base64** as the value in response.
Returns all the stored data as a JSON:
```
curl -s 'localhost:26657/abci_query?data="returnAll"'
```
Returns Transaction matching the Hash (also base64, exactly like in the JSON returned by the *returnAll* call)
```
curl -s 'localhost:26657abci_query?path="search"&data="(HASH TO SEARCH FOR)"'
```
Return the hash of all transactions
```
curl -s 'localhost:26657/abci_query?path="returnHash"'
```


Enabling incmoing transactions (enabled by default on start):
```
curl -s 'localhost:26657/abci_query?path="enableTx"'
```

Disable incoming transactions:
```
curl -s 'localhost:26657/abci_query?path="disableTx"'
```

### DOCKER

```
docker build -t cayley_container .
```

```
docker run cayley_container
```

### DOCKER-COMPOSE

```
docker-compose up
```

After ctrl-c, run to delete the virtual network
```
docker-compose down
```
