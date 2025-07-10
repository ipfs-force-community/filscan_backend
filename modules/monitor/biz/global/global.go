package global

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	logging "github.com/gozelle/logger"
	goredislib "github.com/redis/go-redis/v9"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	mdal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/dal"
	mrepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/repo"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gorm.io/gorm"
)

const ContinuousNotifyDays = 8
const UUIDKeyPrefix = "UUID"

var Global *GlobalBiz
var WatcherLock *redis.RedisLock
var Lock *redsync.Mutex

var logger = logging.NewLogger("global")

type NotifyInfo struct {
	// 以下用于控制发送报警信息的频率
	IsPreAbnormal      bool      `json:"is_pre_abnormal"`
	NotifyCount        int64     `json:"notify_count"`
	NotifyTime         time.Time `json:"notify_time"`
	UpdateAt           time.Time `json:"update_at"`
	AllowDitheringTime time.Time `json:"allow_dithering_time"`
}

func NewGlobalBiz(conf *config.Config, db *gorm.DB, adapter londobell.Adapter, r *redis.Redis) *GlobalBiz {
	WatcherLock = redis.NewRedisLock("watcher_key", r, redis.WithBlock(), redis.WithBlockWaitingSeconds(2))
	client := goredislib.NewClient(&goredislib.Options{
		Addr: *conf.Redis.RedisAddress,
	})
	pool := goredis.NewPool(client)
	rs := redsync.New(pool)
	mutexname := "my-global-mutex"
	Lock = rs.NewMutex(mutexname)

	Global = &GlobalBiz{
		RuleRepo:      mdal.NewRuleDal(db),
		MinerRepo:     prodal.NewMinerInfoDal(db),
		UserMinerRepo: prodal.NewUserMinerDal(db),
		Redis:         r,
		GroupRepo:     prodal.NewGroupDal(db),
		DB:            db,
		Adapter:       adapter,
	}
	return Global
}

type GlobalBiz struct {
	RuleRepo      mrepo.RuleRepo
	MinerRepo     prorepo.MinerRepo
	UserMinerRepo prorepo.UserMinerRepo
	GroupRepo     prorepo.GroupRepo
	Redis         *redis.Redis
	DB            *gorm.DB
	Adapter       londobell.Adapter
	Mutex         sync.Mutex
}

func SetGlobalNotifyInfoMap(ctx context.Context, uuid string, info *NotifyInfo) {
	now := time.Now()
	err := Lock.Lock()
	if err != nil {
		logger.Errorf("redis set lock error: %v", err)
		return
	}
	defer func() {
		if ok, err := Lock.Unlock(); !ok || err != nil {
			logger.Errorf("redis release lock error: %v", err)
			logger.Debugln("SetGlobalNotifyInfoMap lock time:", time.Now().Sub(now), uuid)
			return
		}
		logger.Debugln("SetGlobalNotifyInfoMap lock time:", time.Now().Sub(now), uuid)
	}()
	err = Global.Redis.SetNoExpire(fmt.Sprintf("%s-%s", UUIDKeyPrefix, uuid), info)
	if err != nil {
		logger.Errorf("set uuid %s to redis error: %v", uuid, err)
	}
}

func GetGlobalNotifyInfoMap(ctx context.Context, uuid string) (info *NotifyInfo) {
	now := time.Now()
	err := Lock.Lock()
	if err != nil {
		logger.Errorf("redis set lock error: %v", err)
		return
	}
	defer func() {
		if ok, err := Lock.Unlock(); !ok || err != nil {
			logger.Errorf("redis release lock error: %v", err)
			logger.Debugln("GetGlobalNotifyInfoMap lock time:", time.Now().Sub(now), uuid)
			return
		}
		logger.Debugln("GetGlobalNotifyInfoMap lock time:", time.Now().Sub(now), uuid)
	}()
	val, err := Global.Redis.GetCacheResult(fmt.Sprintf("%s-%s", UUIDKeyPrefix, uuid))
	if err != nil {
		logger.Errorf("get uuid %s from redis error: %v", uuid, err)
		return nil
	}
	v := new(NotifyInfo)
	if val != nil {
		err = json.Unmarshal(val, v)
		if err != nil {
			logger.Errorf("unmarshal uuid %s error: %v", uuid, err)
			return nil
		}
	}
	return v
}

func SetGlobalNotifyInfoMapWithoutLock(ctx context.Context, uuid string, info *NotifyInfo) {
	err := Global.Redis.SetNoExpire(fmt.Sprintf("%s-%s", UUIDKeyPrefix, uuid), info)
	if err != nil {
		logger.Errorf("set uuid %s to redis error: %v", uuid, err)
	}
}

func GetGlobalNotifyInfoMapWithoutLock(ctx context.Context, uuid string) *NotifyInfo {
	val, err := Global.Redis.GetCacheResult(fmt.Sprintf("%s-%s", UUIDKeyPrefix, uuid))
	if err != nil {
		logger.Errorf("get uuid %s from redis error: %v", uuid, err)
		return nil
	}
	v := new(NotifyInfo)
	if val != nil {
		err = json.Unmarshal(val, v)
		if err != nil {
			logger.Errorf("unmarshal uuid %s error: %v", uuid, err)
			return nil
		}
	} else {
		return nil
	}
	return v
}
