package main

import (
	"path"
	"io/ioutil"
	"encoding/json"
	"fmt"
	"flag"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/ANRGUSC/swarmdag/node"
)

var numKeys int
var output string

func init() {
    flag.IntVar(&numKeys, "n", 4, "Number of keys")
    flag.StringVar(&output, "o", ".", "Output dir of keys.json")
}

// Creates keys for Libp2p client
func main() {
	flag.Parse()

	keys := make([]node.Key, numKeys)
	for i := 0; i < numKeys; i++ {
		priv, pub, _ := crypto.GenerateKeyPair(crypto.Ed25519, -1)
		privBytes, _ := priv.Bytes()
		pubBytes, _ := pub.Bytes()
		id, _ := peer.IDFromPublicKey(pub)

		key := node.Key {
			Name: fmt.Sprintf("node%d", i),
			Priv: crypto.ConfigEncodeKey(privBytes),
			Pub: crypto.ConfigEncodeKey(pubBytes),
			Libp2pID: id.String(),
		}

		keys[i] = key
	}

	file, _ := json.MarshalIndent(keys, "", "    ")
	_ = ioutil.WriteFile(path.Join(output, "keys.json"), file, 0755)
	fmt.Println("created", path.Join(output, "keys.json"))
}
