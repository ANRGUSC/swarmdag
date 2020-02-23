module github.com/ANRGUSC/swarmdag/cayley

go 1.13

require (
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/cayleygraph/cayley"
	_ "github.com/cayleygraph/cayley/graph/kv/bolt"
	"github.com/cayleygraph/quad"
)