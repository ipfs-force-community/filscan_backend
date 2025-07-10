package capital_task

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"

	logging "github.com/gozelle/logger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

var logger = logging.NewLogger("capital-task")
var dir string
var once sync.Once

func NewCapitalTask(conf *config.Config, agg londobell.Agg, redis *redis.Redis) *CapitalTask {
	return &CapitalTask{agg: agg, conf: conf, redis: redis}
}

var _ syncer.Task = (*CapitalTask)(nil)
var Days = chain.Epoch(6)

type CapitalTask struct {
	agg        londobell.Agg
	conf       *config.Config
	redis      *redis.Redis
	initCreate bool
}

func (c CapitalTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (c CapitalTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	return
}

func (c CapitalTask) Name() string {
	return "capital-task"
}

func (c CapitalTask) Exec(ctx *syncer.Context) (err error) {
	once.Do(func() {
		logger.Info("第一次资金分析同步器启动，开始同步数据")
		// 第一次同步所有数据
		endEpoch := ctx.Epoch().CurrentDay()
		startEpoch := endEpoch - Days*2880
		err = c.GetALLTransaction(ctx.Context(), startEpoch, endEpoch)
		if err != nil {
			logger.Error(err)
			panic(fmt.Errorf("资金分析同步器第一次同步失败: %w", err))
		}
		logger.Info("资金分析同步器第一次同步成功！")
	})
	if !c.conf.TestNet {
		if ctx.Epoch()%120 != 0 {
			return
		}
	}
	// 同步1天数据，并删除过期数据
	endEpoch := ctx.Epoch().CurrentDay()
	startEpoch := endEpoch - 2880
	if ctx.Epoch()%2880 == 2280 { //同步器每天凌晨一点检查同步
		logger.Infof("开始检查及获取%d天所有转账消息数据", Days.Int64())
		startEpoch = endEpoch - Days*2880
	}

	err = c.GetALLTransaction(ctx.Context(), startEpoch, endEpoch)
	if err != nil {
		return
	}
	return
}
func (c CapitalTask) GetALLTransaction(ctx context.Context, startEpoch, endEpoch chain.Epoch) (err error) {
	keys, err := c.redis.Keys("*_transfer.json")
	if err != nil {
		logger.Errorf("get keys from redis error: %v", err)
		return
	}
	epochTransferMap := make(map[int64]struct{})
	for _, key := range keys {
		if !strings.HasSuffix(key, "transfer.json") {
			continue
		}
		split := strings.Split(key, "_")
		if len(split) != 0 {
			epoch, err := strconv.Atoi(split[0])
			if err != nil {
				return err
			}
			epochTransferMap[int64(epoch)] = struct{}{}
			if startEpoch == endEpoch-2880*Days && int64(epoch) < startEpoch.Int64() { //每天1点才会做此操作
				logger.Infof("移除早期文件-%s", key)
				//err = os.Remove(key) //移除早期的文件
				_, err = c.redis.Delete(key)
				if err != nil {
					logger.Error("移除文件失败:", err)
					return err
				}
				logger.Infof("移除早期文件-%s成功", key)
			}
		}
	}
	var wg sync.WaitGroup
	errChan := make(chan error, 365)
	getTransfer := time.Now()
	for i := startEpoch; i <= endEpoch; i = i + 2880 {
		if _, ok := epochTransferMap[i.Int64()]; !ok {
			epoch := i
			wg.Add(1)
			go func(i chain.Epoch) {
				defer wg.Done()
				err = c.GetTransferTransaction(ctx, epoch)
				if err != nil {
					errChan <- err
				}
			}(epoch)
		}
	}
	wg.Wait()
	logger.Warn("get all transfer file total time:", time.Now().Sub(getTransfer).String())
	close(errChan)
	err = <-errChan
	if err != nil {
		fmt.Println("获取数据时候发生错误:", err)
		return
	}
	return
}

func (c CapitalTask) GetTransferTransaction(ctx context.Context, endEpoch chain.Epoch) (err error) {
	now := time.Now()
	startEpoch := chain.Epoch(endEpoch.Int64() - 2880)
	result, err := c.agg.MessagesForFund(ctx, startEpoch, endEpoch)
	if err != nil {
		logger.Error("从 AGG 中获取数据发生了错误:", err)
		return
	}
	if result == nil {
		logger.Infof("从[%s] 到 [%s] AGG 中没有数据", startEpoch, endEpoch)
		return
	}

	err = c.CreatedTransferRedis(result, endEpoch.Int64())
	if err != nil {
		return
	}
	logger.Infof("保存高度-%d-转账文件成功，耗时:%s", endEpoch, time.Now().Sub(now).String())
	return
}

func (c CapitalTask) CreatedTransferRedis(sourceFile interface{}, epoch int64) (err error) {
	fileName := strconv.Itoa(int(epoch)) + "_transfer.json"
	err = c.redis.SetNoExpire(fileName, sourceFile)
	if err != nil {
		logger.Errorf("set transfer file %s to redis error: %v", fileName, err)
	}
	return
}

func CreatedTransferFile(sourceFile string, epoch int64) (err error) {
	fileName := strconv.Itoa(int(epoch)) + "_transfer.json"
	var file *os.File
	file, err = os.Create(fileName)
	if err != nil {
		return
	}
	_, err = io.WriteString(file, sourceFile)
	return err
}
