package chain

import (
	"fmt"
	"github.com/shopspring/decimal"
)

func NewReward(miner SmartAddress, epoch Epoch, reward AttoFil) Reward {
	return Reward{miner: miner, epoch: epoch, reward: reward}
}

type Reward struct {
	miner     SmartAddress
	epoch     Epoch
	reward    AttoFil
	day       int
	isRelease bool // 标识该奖励时释放奖励
}

func (r Reward) Day() int {
	return r.day
}

func (r Reward) Miner() SmartAddress {
	return r.miner
}

func (r Reward) Epoch() Epoch {
	return r.epoch
}

func (r Reward) Reward() AttoFil {
	return r.reward
}

func (r Reward) Valid() error {
	if r.epoch <= 0 {
		return fmt.Errorf("invalid reward epoch: %d", r.epoch.Int64())
	}
	if r.reward.Decimal().LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("invalid reward: %s", r.reward.String())
	}
	return nil
}

func (r Reward) Releases() (releases RewardReleases, err error) {
	
	if err = r.Valid(); err != nil {
		return
	}
	
	if r.isRelease {
		err = fmt.Errorf("release reward has no releases")
		return
	}
	
	// 产生奖励高度立即释放 25%
	released := r.reward.Decimal().Mul(decimal.NewFromFloat(0.25))
	releases.list = append(releases.list, Reward{
		epoch:     r.epoch,
		reward:    NewNanoFil(released),
		isRelease: true,
	})
	
	epoch := r.epoch
	
	// 释放 180 天
	for i := 0; i < 180; i++ {
		epoch += 2880 // 按天累加
		if i == 179 {
			releases.list = append(releases.list, Reward{
				epoch:     epoch,
				reward:    NewNanoFil(r.reward.Decimal().Sub(released)), // 最后一天全部将除以不尽的数全部释放
				day:       i + 1,
				isRelease: true,
			})
		} else {
			v := r.reward.Decimal().Mul(decimal.NewFromFloat(0.75).Div(decimal.NewFromInt(180)))
			released = released.Add(v)
			releases.list = append(releases.list, Reward{
				epoch:     epoch,
				reward:    NewNanoFil(v),
				day:       i + 1,
				isRelease: true,
			})
		}
	}
	
	releases.mapping = map[string]Reward{}
	for _, v := range releases.list {
		if v.Epoch() < r.epoch {
			err = fmt.Errorf("invalid reward: %s(%d) release epoch: %d", r.reward.String(), r.epoch.Int64(), v.Epoch().Int64())
			return
		}
		releases.mapping[v.Epoch().Date()] = v
	}
	
	return
}

type RewardReleases struct {
	list    []Reward
	mapping map[string]Reward
}

func (r RewardReleases) Rewards() []Reward {
	return r.list
}

// TotalReleasesBeforeEpoch  获取给定高度以前的产生的释放奖励总额
// < epoch
func (r RewardReleases) TotalReleasesBeforeEpoch(epoch Epoch) AttoFil {
	total := decimal.Decimal{}
	for _, v := range r.list {
		if v.Epoch() < epoch {
			total = total.Add(v.reward.Decimal())
		}
	}
	return NewNanoFil(total)
}

// TotalReleasesAfterEpoch 获取给定高度以后的产生的释放奖励总额
// > epoch
func (r RewardReleases) TotalReleasesAfterEpoch(epoch Epoch) AttoFil {
	total := decimal.Decimal{}
	for _, v := range r.list {
		if v.Epoch() > epoch {
			total = total.Add(v.reward.Decimal())
		}
	}
	return NewNanoFil(total)
}

func (r RewardReleases) DayRelease(date string) (reward Reward, ok bool) {
	if r.mapping == nil {
		return
	}
	reward, ok = r.mapping[date]
	if ok {
		return
	}
	return
}
