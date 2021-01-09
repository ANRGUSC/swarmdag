package ledger

import (
	"sync"
	"sort"
	"github.com/mitchellh/hashstructure/v2"
)

type Index interface {
	InsertTxHash(chainID string, txHash string)
	HashSortedChainIDs() uint64
	ChainCount() int
	CompileChainInfo() map[string]txInfo
	HashLedger() uint64
	ChainIDs() []string
	GetLedgerMap() map[string][]string
}

type index struct {
	LedgerMap map[string][]string // chain ID to sorted transactions
	SortedChainIDs []string
	txCount int
	lock sync.Mutex
}

type hashableLedger struct {
	SortedChainIDs []string
	SortedTxHashes []uint64
}

func NewIndex() Index {
    idx := &index {
        LedgerMap: make(map[string][]string),
        txCount: 0,
    }

    return idx
}

func (idx *index) GetLedgerMap() map[string][]string {
	return idx.LedgerMap
}

func (idx *index) ChainIDs() []string {
	idx.lock.Lock()
	defer idx.lock.Unlock()
	var chainIDs []string
	copy(chainIDs, idx.SortedChainIDs)
	return chainIDs
}

func (idx *index) insertChainID(id string) {
	if _, exists := idx.LedgerMap[id]; exists {
		return
	}
	idx.LedgerMap[id] = []string{}
	i := sort.SearchStrings(idx.SortedChainIDs, id)
	idx.SortedChainIDs = append(idx.SortedChainIDs, "")
	if i < len(idx.SortedChainIDs) - 1 {
		copy(idx.SortedChainIDs[i+1:], idx.SortedChainIDs[i:])
	}
	idx.SortedChainIDs[i] = id
}

// Returns false if transaction already exists.
func (idx *index) InsertTxHash(chainID, txHash string) {
	idx.lock.Lock()
	defer idx.lock.Unlock()
	idx.insertChainID(chainID)
	i := sort.SearchStrings(idx.LedgerMap[chainID], txHash)
	if i < len(idx.LedgerMap[chainID]) && idx.LedgerMap[chainID][i] == txHash {
		return
	}
	idx.LedgerMap[chainID] = append(idx.LedgerMap[chainID], "")
	if i < len(idx.LedgerMap[chainID]) - 1 {
		copy(idx.LedgerMap[chainID][i+1:], idx.LedgerMap[chainID][i:])
	}
	idx.LedgerMap[chainID][i] = txHash
	idx.txCount++
}

func (idx *index) HashSortedChainIDs() uint64 {
	idx.lock.Lock()
	defer idx.lock.Unlock()
	h, err := hashstructure.Hash(idx.SortedChainIDs, hashstructure.FormatV2, nil)
	if err != nil {
		panic(err)
	}
	return h
}

func (idx *index) ChainCount() int {
	idx.lock.Lock()
	defer idx.lock.Unlock()
	return len(idx.SortedChainIDs)
}

func (idx *index) CompileChainInfo() map[string]txInfo {
	idx.lock.Lock()
	defer idx.lock.Unlock()

	chainInfo := make(map[string]txInfo, len(idx.SortedChainIDs))
	for chainID, txs := range idx.LedgerMap {
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
	var l hashableLedger
	l.SortedChainIDs = idx.SortedChainIDs
	l.SortedTxHashes = make([]uint64, len(idx.SortedChainIDs))
	for i, chainID := range l.SortedChainIDs {
		h, err := hashstructure.Hash(idx.LedgerMap[chainID], hashstructure.FormatV2, nil)
		if err != nil {
			panic(err)
		}
		l.SortedTxHashes[i] = h
	}

	// j, err := json.MarshalIndent(l, "", "  ")
	// fmt.Println(string(j))

	ledgerHash, err := hashstructure.Hash(l, hashstructure.FormatV2, nil)
	if err != nil {
		panic(err)
	}
	return ledgerHash
}
