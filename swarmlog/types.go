package swarmlog

type InsertTx struct {
    Type        string         `json:"Type"`
    Hash        string         `json:"Hash"`
    UnixTime    int64          `json:"UnixTime"`
}

type MembershipStart struct {
    Type        string         `json:"Type"`
    NodeID      int            `json:"NodeID"`
    UnixTime    int64          `json:"UnixTime"`
}

type InstallView struct {
    Type            string         `json:"Type"`
    NodeID          int            `json:"NodeID"`
    UnixTime        int64          `json:"UnixTime"`
    ViewID          int            `json:"ViewID"`
    Members         []int          `json:"Members"`
    AmLeader        bool           `json:"AmLeader"`
    ProposeTime     int64          `json:"ProposeTime"`
    StartLead       int64          `json:"StartLead"`
}
