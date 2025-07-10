package po

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"time"
)

type MinerLocation struct {
	Miner      string
	Country    string
	Region     *string
	City       *string
	Latitude   *float64
	Longitude  *float64
	Ip         string
	MultiAddrs types.StringArray
	UpdatedAt  *time.Time
}

func (MinerLocation) TableName() string {
	return "chain.miner_locations"
}
