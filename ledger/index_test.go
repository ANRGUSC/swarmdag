package ledger

import (
	"testing"
	"reflect"
)

func TestInsertChainID(t *testing.T) {
	targetTxHashes := []string{"a", "aa", "ab", "abc", "d", "de", "def", "m", "z"}
	targetChainIDs := []string{"1", "a", "ab", "abc", "d", "def", "l", "m", "n", "z"}

	ledgerMap := map[string][]string {
		"a": []string{"a", "ab", "abc", "d", "def", "m", "z"},
		"ab": []string{"ab"},
		"abc": []string{"abc"},
		"d": []string{"d"},
		"def": []string{"def"},
		"m": []string{"m"},
		"z": []string{"z"},
	}

	idx :=  index{
		LedgerMap: ledgerMap,
		SortedChainIDs: []string{"a", "ab", "abc", "d", "def", "m", "z"},
		txCount: 1,
	}

	idx.insertChainID("1")
	idx.insertChainID("l")
	idx.insertChainID("n")

	idx.InsertTxHash("a", "aa")
	idx.InsertTxHash("a", "de")

	if !reflect.DeepEqual(idx.LedgerMap["a"], targetTxHashes) {
		t.Error("tx hash insertions failed")
	}

	if !reflect.DeepEqual(idx.SortedChainIDs, targetChainIDs) {
		t.Error("chain ID insertions failed")
	}
}