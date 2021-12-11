package swarmlog

type InsertTx struct {
    Type        string         `json:"Type"`
    Hash        string         `json:"Hash"`
    UnixTime    int64          `json:"UnixTime"`
}

type MembershipStart struct {
    Type        string         `json:"Type"`
    NodeID      int64          `json:"NodeID"`
    UnixTime    int64          `json:"UnixTime"`
}

type InstallView struct {
    Type            string         `json:"Type"`
    NodeID          int64          `json:"NodeID"`
    UnixTime        int64          `json:"UnixTime"`
    Members         []int          `json:"Members"`
    AmLeader        bool           `json:"AmLeader"`
    ProposeTime     uint64         `json:"ProposeTime"`
}
