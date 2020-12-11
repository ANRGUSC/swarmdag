package ledger

import (
	"testing"
	"encoding/json"
	"reflect"
)

func TestCreateMessage(t *testing.T) {
	index := NewIndex()

	index.InsertChainID("abcde")
	index.InsertTxHash("abcde", "123456")
	index.InsertTxHash("abcde", "789012")
	index.InsertTxHash("abcde", "345678")

	index.InsertChainID("hello0")
	index.InsertTxHash("hello0", "transaction1")
	index.InsertTxHash("hello0", "transaction2")
	index.InsertTxHash("hello0", "transaction3")

	m := createMsg(index)

	msg, err := json.Marshal(m)
	if err != nil {
		t.Error(err)
	}
	t.Log(string(msg))

	var rMsg reconcileMsg
	if err = json.Unmarshal(msg, &rMsg); err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(rMsg, m) {
		t.Error("error creating reconcileMsg")
	}
}
