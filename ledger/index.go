package ledger

import (
	"sync"
	"sort"
	"github.com/mitchellh/hashstructure/v2"
)

type Index interface {
	InsertChainID(id string)
	InsertTxHash(chainID string, txHash string)
	HashSortedChainIDs() uint64
	ChainCount() int
	CompileChainInfo() map[string]txInfo
	HashLedger() uint64
	ChainIDs() []string
}

type index struct {
	ledgerMap map[string][]string // chain ID to sorted transactions
	sortedChainIDs []string
	txCount int
	lock sync.Mutex
}

func NewIndex() Index {
    idx := &index {
        ledgerMap: make(map[string][]string),
        txCount: 0,
    }

    return idx
}

func (idx *index) ChainIDs() []string {
	idx.lock.Lock()
	defer idx.lock.Unlock()
	var chainIDs []string
	copy(chainIDs, idx.sortedChainIDs)
	return chainIDs
}

func (idx *index) InsertChainID(id string) {
	idx.lock.Lock()
	defer idx.lock.Unlock()
	if _, exists := idx.ledgerMap[id]; exists {
		return
	}
	idx.ledgerMap[id] = []string{}
	i := sort.SearchStrings(idx.sortedChainIDs, id)
	idx.sortedChainIDs = append(idx.sortedChainIDs, "")
	if i < len(idx.sortedChainIDs) - 1 {
		copy(idx.sortedChainIDs[i+1:], idx.sortedChainIDs[i:])
	}
	idx.sortedChainIDs[i] = id
}

// Returns false if transaction already exists.
func (idx *index) InsertTxHash(chainID, txHash string) {
	idx.lock.Lock()
	defer idx.lock.Unlock()
	if _, exists := idx.ledgerMap[chainID]; !exists {
		idx.ledgerMap[chainID] = []string{}
	}
	i := sort.SearchStrings(idx.ledgerMap[chainID], txHash)
	if i < len(idx.ledgerMap[chainID]) && idx.ledgerMap[chainID][i] == txHash {
		return
	}
	idx.ledgerMap[chainID] = append(idx.ledgerMap[chainID], "")
	if i < len(idx.ledgerMap[chainID]) - 1 {
		copy(idx.ledgerMap[chainID][i+1:], idx.ledgerMap[chainID][i:])
	}
	idx.ledgerMap[chainID][i] = txHash
	idx.txCount++
}

func (idx *index) HashSortedChainIDs() uint64 {
	idx.lock.Lock()
	defer idx.lock.Unlock()
	h, err := hashstructure.Hash(idx.sortedChainIDs, hashstructure.FormatV2, nil)
	if err != nil {
		panic(err)
	}
	return h
}

func (idx *index) ChainCount() int {
	idx.lock.Lock()
	defer idx.lock.Unlock()
	return len(idx.sortedChainIDs)
}

func (idx *index) CompileChainInfo() map[string]txInfo {
	idx.lock.Lock()
	defer idx.lock.Unlock()

	chainInfo := make(map[string]txInfo, len(idx.sortedChainIDs))
	for chainID, txs := range idx.ledgerMap {
		h, err := hashstructure.Hash(txs, hashstructure.FormatV2, nil)
		if err != nil {
			panic(err)
		}
		chainInfo[chainID] = txInfo{
			SortedTxHash: h,
			TxCount: len(txs),
		}
	}
	return chainInfo
}

func (idx *index) HashLedger() uint64 {
	idx.lock.Lock()
	defer idx.lock.Unlock()
	h, err := hashstructure.Hash(idx.ledgerMap, hashstructure.FormatV2, nil)
	if err != nil {
		panic(err)
	}
	return h
}


