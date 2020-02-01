package monitor

import (
	"encoding/hex"
	"testing"
	"os"
	"math/rand"
	"strconv"
	"time"
	"crypto/sha1"
	// "github.com/stretchr/testify/assert"
	log "github.com/sirupsen/logrus"
)



func TestMain(t *testing.T) {
    log.SetOutput(os.Stdout)
    log.SetLevel(log.DebugLevel)
}

func TestClientDashboardCreation(t *testing.T) {
    // grafanaURL := "http://0.0.0.0:3000"
    // err := CreateDashboard(grafanaURL) 
    // assert.NoError(t, err, "expected no error on CreateDashboard")
}

func TestReport(t *testing.T) {
    log.SetOutput(os.Stdout)
    log.SetLevel(log.DebugLevel)

    InitMonitor()

    time.Sleep(2000 * time.Second)
}

// assumes Grafana running locally on port 8086
func TestMembershipReport(t *testing.T) {
    rand.Seed(time.Now().UnixNano())
    // partitionID := 0 

    InitMonitor()

    time.Sleep(1 * time.Second)
    rand.Seed(time.Now().UnixNano())
    b := make([]byte, 10)

    for i := 0; i < 8; i++ {
        rand.Read(b)
        h := sha1.New()
        h.Write(b)
        mID := h.Sum(nil)
        ReportEventRequest("", "http://0.0.0.0:8086", strconv.Itoa(i), hex.EncodeToString(mID))
    }

    ShutdownMonitor()
}

func TestUpdatePartitionTable(t *testing.T) {
    totalNodes := 8
    numPartitions := 3

    rand.Seed(time.Now().UnixNano())
    InitMonitor()

    time.Sleep(1 * time.Second)
    b := make([]byte, 10)

    for i := 0; i < totalNodes; i++ {
        rand.Read(b)
        h := sha1.New()
        h.Write(b)
        mID := h.Sum(nil)
        updatePartitionTable(i, hex.EncodeToString(mID))
    }

    log.Debugf("nodetable \n %v", pTable.nodeTable)
    log.Debugf("pTable \n %v", pTable.entry)

    log.Debugf("Splitting into %d partitions...", numPartitions)
    for j := 0; j < numPartitions; j++ {
        rand.Read(b)
        h := sha1.New()
        h.Write(b)
        mID := h.Sum(nil)
        for i := 0; i < totalNodes; i++ {
            if i % numPartitions == j {
                updatePartitionTable(i, hex.EncodeToString(mID))
            }
        }
    }

    log.Debugf("nodetable \n %v", pTable.nodeTable)
    log.Debugf("pTable \n %v", pTable.entry)

    log.Debug("Merging into a single partition...")
    rand.Read(b)
    h := sha1.New()
    h.Write(b)
    mID := h.Sum(nil)
    for i := 0; i < totalNodes; i++ {
        updatePartitionTable(i, hex.EncodeToString(mID))
    }

    log.Debugf("nodetable \n %v", pTable.nodeTable)
    log.Debugf("pTable \n %v", pTable.entry)

    ShutdownMonitor()
}
