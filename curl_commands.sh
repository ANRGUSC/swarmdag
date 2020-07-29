curl -s '192.168.10.2:3000/broadcast_tx_commit?tx="<k1>=<v1>"'
curl -s '192.168.10.4:3000/broadcast_tx_commit?tx="<k2>=<v2>"'
curl -s '192.168.10.3:3000/abci_query?data="returnAll"'
curl -s '192.168.10.2:3001/broadcast_tx_commit?tx="<k3>=<v3>"'
curl -s '192.168.10.4:3001/broadcast_tx_commit?tx="<k4>=<v4>"'
curl -s '192.168.10.3:3001/abci_query?data="returnAll"'
curl -s '192.168.10.5:3001/abci_query?data="returnAll"'

curl -X POST --data '{
    "jsonrpc": "2.0",
    "method": "platform.sign",
    "params": {
        "tx":"1112SGbsdMYKfF3Qfx6EvUct4FRaainLkLtWUeE14Pvx37xueb6fw5bUv53L22VRnTmfRkJ2CrqBQSimcCGRxtwYcfq5Dcw5T3uBen6g1emNVRtuBakmMYTwRBJuao8M5HjK5X44SdZTZLQevpEseAti78n9fy8o88KwvG18GToMHPdFoVLugLSd3W25L2S1768GNmkpWQz7t3Gy",
        "signer":"6Y3kysjF9jnHnYkdS9yGAuoHyae2eNmeV",
        "username":"jason",
        "password":"jas0n_Tran#&@"
    },
    "id": 2
}' -H 'content-type:application/json;' 10.0.7.254:9650/ext/P

MYTEST=`echo '{
            "hash": "thisishash",
            "parentHash": "1634fc5298057fd76ca2345f2d3466812f749109657d87d5430c4b7bdb449a32",
            "timestamp": '$(date +%s)',
            "key": "k7",
            "value": "v6"
        }' | base64 -w 0`

curl --data-binary '{
    "jsonrpc":"2.0",
    "id":"anything",
    "method":"broadcast_tx_commit",
    "params": {
        "tx": "ewogICAgICAgICAgICAiaGFzaCI6ICJ0aGlzaXNoYXNoIiwKICAgICAgICAgICAgInBhcmVudEhhc2giOiAiNjIxNmVhMzdjMGIwZTIyNzY3N2ViODU1MjlhZTY5NzRkZDkyNTU4NmExMDI2ODY1YjRjOGRhMzE2NjM5ZDM5NCIsCiAgICAgICAgICAgICJ0aW1lc3RhbXAiOiAxNTk2MDExNTA1LAogICAgICAgICAgICAia2V5IjogIms3IiwKICAgICAgICAgICAgInZhbHVlIjogInY2IgogICAgICAgIH0K"
    }
}' -H 'content-type:text/plain;' http://192.168.10.3:3000

ewogICAgICAgICAgICAiaGFzaCI6ICJ0aGlzaXNoYXNoIiwKICAgICAgICAgICAgInByZXZIYXNoIjogInRoZSBsYXN0IGhhc2giLAogICAgICAgICAgICAidGltZXN0YW1wIjogMTU5NTg5NzI0OSwKICAgICAgICAgICAgImtleSI6ICJrNiIsCiAgICAgICAgICAgICJ2YWx1ZSI6ICJ2NiIKICAgICAgICB9Cg==

1634fc5298057fd76ca2345f2d3466812f749109657d87d5430c4b7bdb449a32