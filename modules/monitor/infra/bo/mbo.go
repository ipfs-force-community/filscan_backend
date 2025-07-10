package mbo

type MinerTag struct {
	miners    string `gorm:"column:miners"`
	minerTags string `gorm:"column:miners_tags"`
}

type MinerDetail struct {
	Epoch    int64                `json:"epoch"`
	Accounts []AccountAddrBalance `json:"accounts"`
}

type MinerInfo struct {
	MinerID   string `json:"miner_id"`
	MinerTag  string `json:"miner_tag"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
}

type AccountAddrBalance struct {
	Type    string `json:"type,omitempty"`
	Addr    string `json:"address,omitempty"`
	Balance string `json:"balance,omitempty"`
}
