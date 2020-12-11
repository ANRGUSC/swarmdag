package node

import (
	"os"
	"encoding/json"
	"testing"
	"io/ioutil"
)

func TestGetPrivKey(t *testing.T) {
	keyfile, _ := ioutil.TempFile("", "keys.json")
	defer os.Remove(keyfile.Name())

	keys := make([]Key, 2)

	keys[0] = Key {
		Name: "node0",
		Priv: "CAESQFal1SFspPcs7REX71X2SD3I6s8q3/uochspQHdxVd34kU9wrrkWFdCL0IVP6Z4RQ9W1QwY0qiBZM+RWs2weXK0=",
		Pub: "CAESIJFPcK65FhXQi9CFT+meEUPVtUMGNKogWTPkVrNsHlyt",
	}

	keys[1] = Key {
		Name: "node1",
		Priv: "CAESQGOl+Ew9+SajBLGsylWgZT8P9JZwUbZ85nbgDO1puOh2KI0WgAXAfNbRGp4xB0Ock0fLde/hy4dARB16c5RctWc=",
		Pub: "CAESICiNFoAFwHzW0RqeMQdDnJNHy3Xv4cuHQEQdenOUXLVn",
	}

	out, _ := json.MarshalIndent(keys, "", "    ")
	keyfile.Write(out)
	keyfile.Close()

	for i := 0; i < 2; i++ {
		k, err := getPrivKey(keyfile.Name(), i)
		if err != nil {
			t.Error(err)
		}
		t.Logf("node%d privkey is %s", i, k)
	}
}
