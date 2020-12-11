package node

import (
	"encoding/json"
	"io/ioutil"
	"fmt"
	"os"
	"github.com/libp2p/go-libp2p-core/peer"
)

// Libp2p keys
type Key struct {
	Name	  string    `json:"name"`
	Priv	  string	`json:"privKey"`
	Pub		  string	`json:"pubKey"`
    Libp2pID  string    `json:"libp2pID"`
}

func getPrivKey(path string, nodeID int) (string, error) {
    if _, err := os.Stat(path); os.IsNotExist(err) {
        return "", err
    }
    var keys []Key
    data, _ := ioutil.ReadFile(path)
    _ = json.Unmarshal(data, &keys)
    k := keys[nodeID]

    if k.Name != fmt.Sprintf("node%d", nodeID) {
        panic("Key file not ordered")
    }

    return k.Priv, nil
}

func readLibp2pIDs(path string) (map[peer.ID]int, error) {
    if _, err := os.Stat(path); os.IsNotExist(err) {
        return nil, err
    }
    var keys []Key
    ids := make(map[peer.ID]int)
    data, _ := ioutil.ReadFile(path)
    _ = json.Unmarshal(data, &keys)

    for i, k := range keys {
        id, _ := peer.IDB58Decode(k.Libp2pID)
        ids[id] = i
    }

    return ids, nil
}