## Cayley Usage Examples

Run this for an example of a DAG with fields/values per transaction:

    go run dag.go

You can visualize with `graphviz` (`sudo apt-get install graphviz`). First, 
compile the `cayley` binary from source or download the latest release binary
from https://github.com/cayleygraph/cayley/releases. Place it in this directory
and run:

    cayley dump --dbpath "db.boltdb" --db bolt --dump_format=graphviz -o=- | dot -Tpng -ograph.png