package partition

import (
    "testing"
    logging "github.com/op/go-logging"
)

func TestPartitionManager(t *testing.T) {

    StartPartitionManager(logging.MustGetLogger("partition test"))
    Start(0)
    Start(1)

    for i := range pm.instances {
        node := pm.instances[i] 
        node.Stop()
        node.Wait()
    }
}
