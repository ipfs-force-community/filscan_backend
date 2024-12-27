package monitortest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gozelle/fs"
	"github.com/gozelle/spew"
	"github.com/gozelle/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	mapi "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/api"
	mbiz "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/service"
	mbo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

func TestAddALLPowerRule(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)
	client, err := injector.NewAliMsgClient(conf)
	require.NoError(t, err)
	clientClient, err := injector.NewAliCallClient(conf)
	require.NoError(t, err)
	mailClient, err := injector.NewMailClient(conf)
	require.NoError(t, err)
	_ = injector.NewNotify(conf, client, clientClient, mailClient)

	db, _, err := injector.NewGormDB(conf)

	require.NoError(t, err)
	redisRedis := redis.NewRedis(conf)
	globalBiz := injector.NewGlobalBiz(conf, db, adapter, redisRedis)
	biz := mbiz.NewRuleBiz(db, adapter)
	var ids []int64
	err = globalBiz.DB.Raw(`SELECT id FROM pro.users ORDER BY "id"`).Scan(&ids).Error
	if err != nil {
		return
	}
	fmt.Println(ids)
	for _, id := range ids {
		group, err := biz.GroupRepo.SelectActiveGroupsByUserID(context.Background(), id)
		if err != nil {
			return
		}
		var reqList []*mapi.SaveUserRulesReq
		for _, p := range group {
			reqList = append(reqList, &mapi.SaveUserRulesReq{
				MType:        "PowerMonitor",
				GroupIDOrAll: p.Id,
				MinerOrAll:   "",
			})
		}
		request := mapi.SaveUserRulesRequest{Items: reqList}
		resp, err := biz.SaveUserRules(context.Background(), request)
		if err != nil {
			return
		}
		fmt.Println(resp)
	}

}

func TestName(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)
	client, err := injector.NewAliMsgClient(conf)
	require.NoError(t, err)
	clientClient, err := injector.NewAliCallClient(conf)
	require.NoError(t, err)
	mailClient, err := injector.NewMailClient(conf)
	require.NoError(t, err)
	_ = injector.NewNotify(conf, client, clientClient, mailClient)

	db, _, err := injector.NewGormDB(conf)

	require.NoError(t, err)
	redisRedis := redis.NewRedis(conf)
	globalBiz := injector.NewGlobalBiz(conf, db, adapter, redisRedis)
	userMiners, err := globalBiz.UserMinerRepo.SelectMinersByUserID(context.Background(), 11) //获得加入组里的miner
	if err != nil {
		return
	}
	groups, err := globalBiz.GroupRepo.SelectActiveGroupsByUserID(context.Background(), 11)
	if err != nil {
		return
	}
	groupIDToNameMap := make(map[int64]string)
	for _, group := range groups {
		groupIDToNameMap[group.Id] = group.GroupName
	}
	for _, userMiner := range userMiners {
		if userMiner.MinerID.Address() == "f01909429" {
			groupID := int64(0)
			if userMiner.GroupID != nil {
				groupID = *userMiner.GroupID
			}
			var a = &mbo.MinerInfo{
				MinerID:   "f01909429",
				MinerTag:  userMiner.MinerTag,
				GroupID:   groupID,
				GroupName: groupIDToNameMap[groupID],
			}
			fmt.Println(a)
		}
	}
}

func TestRuleWatcherBiz(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)
	client, err := injector.NewAliMsgClient(conf)
	require.NoError(t, err)
	clientClient, err := injector.NewAliCallClient(conf)
	require.NoError(t, err)
	mailClient, err := injector.NewMailClient(conf)
	require.NoError(t, err)
	_ = injector.NewNotify(conf, client, clientClient, mailClient)

	db, _, err := injector.NewGormDB(conf)

	require.NoError(t, err)

	redisRedis := redis.NewRedis(conf)
	_ = injector.NewGlobalBiz(conf, db, adapter, redisRedis)
	biz := injector.NewWatcherBiz(db, adapter, redisRedis)
	fmt.Println("start ...")
	biz.RuleWatch(context.Background())
	fmt.Println("end ...")
	select {}
}

func TestVIP(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)
	client, err := injector.NewAliMsgClient(conf)
	require.NoError(t, err)
	clientClient, err := injector.NewAliCallClient(conf)
	require.NoError(t, err)
	mailClient, err := injector.NewMailClient(conf)
	require.NoError(t, err)
	_ = injector.NewNotify(conf, client, clientClient, mailClient)

	db, _, err := injector.NewGormDB(conf)

	require.NoError(t, err)

	redisRedis := redis.NewRedis(conf)
	_ = injector.NewGlobalBiz(conf, db, adapter, redisRedis)
	biz := injector.NewWatcherBiz(db, adapter, redisRedis)
	fmt.Println(time.Now().Format("2006-01-02"))
	biz.RemoveOutdatedRule(context.Background())
	//biz.SendVipExpirationNotification(context.Background())
	//biz.CheckVipExpiration(context.Background())
	biz.VipWatch(context.Background())
}

func TestDistrubutedLock(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)
	client, err := injector.NewAliMsgClient(conf)
	require.NoError(t, err)
	clientClient, err := injector.NewAliCallClient(conf)
	require.NoError(t, err)
	mailClient, err := injector.NewMailClient(conf)
	require.NoError(t, err)
	_ = injector.NewNotify(conf, client, clientClient, mailClient)

	db, _, err := injector.NewGormDB(conf)

	require.NoError(t, err)

	redisRedis := redis.NewRedis(conf)
	_ = injector.NewGlobalBiz(conf, db, adapter, redisRedis)
	//biz := injector.NewWatcherBiz(db, adapter, redisRedis)
	lock := redis.NewRedisLock("hahah", redisRedis, redis.WithBlock(), redis.WithBlockWaitingSeconds(2))
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		fmt.Println("666666666")
		if err := lock.Lock(ctx); err != nil {
			t.Error(err)
			return
		}
		fmt.Println(err, 33)
		time.Sleep(4 * time.Second)
		err := lock.Unlock(ctx)
		if err != nil {
			fmt.Println(err)
		}

	}()

	go func() {
		time.Sleep(time.Second)
		defer wg.Done()
		if err := lock.Lock(ctx); err != nil {
			t.Error(err)
			return
		}
		fmt.Println(666666)
		err = lock.Unlock(ctx)
		if err != nil {
			fmt.Println(err)
		}
	}()
	fmt.Println("hahahahahah")
	wg.Wait()

	t.Log("success")
}
