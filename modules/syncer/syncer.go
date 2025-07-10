package syncer

import (
	"context"
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"

	"github.com/gozelle/async/parallel"
	logging "github.com/gozelle/logger"
	"github.com/gozelle/mix"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WithName 必须，配置同步器名称，要求唯一
func WithName(name string) Option {
	return func(conf *config) {
		conf.name = strings.TrimSpace(name)
	}
}

// WithDB 必须，配置数据库连接
func WithDB(db *gorm.DB) Option {
	return func(conf *config) {
		conf.db = db
	}
}

// WithLondobellAgg 必须，配置 Londobell Agg
func WithLondobellAgg(agg londobell.Agg) Option {
	return func(conf *config) {
		conf.agg = agg
	}
}

func WithRedis(redis *redis.Redis) Option {
	return func(conf *config) {
		conf.redis = redis
	}
}

func WithLondobellMinerAgg(agg londobell.MinerAgg) Option {
	return func(conf *config) {
		conf.minerAgg = agg
	}
}

// WithLondobellAdapter 必须，配置 Londobell Adapter
func WithLondobellAdapter(adapter londobell.Adapter) Option {
	return func(conf *config) {
		conf.adapter = adapter
	}
}

// WithInitEpoch 配置首次同步时的初始高度
func WithInitEpoch(initEpoch *int64) Option {
	return func(conf *config) {
		conf.initEpoch = initEpoch
	}
}

// WithStopEpoch 当同步高度 = stopEpoch, 时，则停止同步
func WithStopEpoch(stopEpoch *int64) Option {
	return func(conf *config) {
		conf.stopEpoch = stopEpoch
	}
}

// WithErrorWaitDuration 配置同步器执行错误重试等待时间
func WithErrorWaitDuration(duration time.Duration) Option {
	return func(conf *config) {
		conf.errorWaitDuration = duration
	}
}

// WithTaskGroup 添加并行任务组
func WithTaskGroup(group ...TaskGroup) Option {
	return func(conf *config) {
		conf.taskGroups = append(conf.taskGroups, group...)
	}
}

// WithCalculators 添加串行计算器
func WithCalculators(calc ...Calculator) Option {
	return func(conf *config) {
		conf.calculators = append(conf.calculators, calc...)
	}
}

// WithEpochsChunk 配置并发同步高度的最大并发数
func WithEpochsChunk(chunk int64) Option {
	return func(conf *config) {
		conf.epochsChunk = chunk
	}
}

// WithContextBuilder 在每一个高度同步器，准备该同步器所需要的上下文
func WithContextBuilder(builder ...func(ctx *Context) error) Option {
	return func(conf *config) {
		conf.contextBuilders = append(conf.contextBuilders, builder...)
	}
}

// WithDry 以 Dry 模式运行
// 行为：无论 syncer 的某个高度是否同步过，都会再执行一遍任务
// 执行任务时，如果任务对应高度已执行过，需要删除数据库记录才能再执行
func WithDry(v bool) Option {
	return func(conf *config) {
		conf.dry = v
	}
}

// WithEpochsThreshold 设置同步步长阈值
func WithEpochsThreshold(v int64) Option {
	return func(conf *config) {
		conf.epochsThreshold = v
	}
}

// WithGlobalRollback 全局回滚函数
func WithGlobalRollback(h ...func(ctx context.Context, gteEpoch chain.Epoch) error) Option {
	return func(conf *config) {
		conf.globalRollback = append(conf.globalRollback, h...)
	}
}

type Option func(conf *config)

type config struct {
	name              string
	db                *gorm.DB
	agg               londobell.Agg
	minerAgg          londobell.MinerAgg
	adapter           londobell.Adapter
	redis             *redis.Redis
	initEpoch         *int64
	stopEpoch         *int64
	repo              repository.SyncerRepo
	errorWaitDuration time.Duration                    // 同步错误等待时间
	epochsThreshold   int64                            // 并发同步段阈值
	epochsChunk       int64                            // 并发同步的并发数
	taskGroups        []TaskGroup                      // 同步任务分组，Runner 之间并发执行，任务之间同步执行
	calculators       []Calculator                     // 与同步器相关联的计算器
	contextBuilders   []func(ctx *Context) (err error) // 准备前置 Context
	dry               bool                             // Dry 模式，只正常执行同步任务和计算任务，不做 Tipset 检查及回滚检查等
	globalRollback    []func(ctx context.Context, gteEpoch chain.Epoch) error
}

func isValidSyncerName(name string) bool {
	// 正则表达式：只能小写字母、中横线、数字，且不能以数字开头
	pattern := "^[a-z][-a-z0-9]*$"
	match, err := regexp.MatchString(pattern, name)
	if err != nil {
		return false
	}
	return match
}

// 检查 config 是否满足必要项
func (c config) valid() error {

	if c.name == "" {
		return fmt.Errorf("required name")
	}

	if !isValidSyncerName(c.name) {
		return fmt.Errorf("错误名称: %s, 同步器名称只能包含: 小写字母、中横线、数字", c.name)
	}

	for _, v := range c.taskGroups {
		for _, vv := range v {
			if !isValidSyncerName(vv.Name()) {
				return fmt.Errorf("错误名称: %s, 任务名称只能包含: 小写字母、中横线、数字", vv.Name())
			}
		}
	}

	for _, v := range c.calculators {
		if !isValidSyncerName(v.Name()) {
			return fmt.Errorf("错误名称: %s, 任务名称只能包含: 小写字母、中横线、数字", v.Name())
		}
	}

	if c.db == nil {
		return fmt.Errorf("requried db")
	}
	if c.agg == nil {
		return fmt.Errorf("requried londobell agg")
	}
	if c.adapter == nil {
		return fmt.Errorf("requried londobell adapter")
	}
	if len(c.taskGroups) == 0 && len(c.calculators) == 0 {
		return fmt.Errorf("同步器: %s 的任务和计算器都为空", c.name)
	}

	return nil
}

func NewSyncer(options ...Option) *Syncer {
	conf := &config{}
	for _, o := range options {
		o(conf)
	}

	return &Syncer{
		config: conf,
		log:    logging.NewLogger("syncer"),
	}
}

type Syncer struct {
	epoch chain.Epoch
	repo  repository.SyncerRepo
	log   *logging.Logger
	quit  chan struct{}
	*config
}

func (s *Syncer) Init() (err error) {

	err = s.config.valid()
	if err != nil {
		return
	}
	if s.dry {
		s.log = logging.NewLogger(fmt.Sprintf("[DRY]syncer::%s", s.name))
	} else {
		s.log = logging.NewLogger(fmt.Sprintf("syncer::%s", s.name))
	}

	if s.errorWaitDuration <= 0 {
		s.errorWaitDuration = 15 * time.Second
	}
	if s.epochsChunk <= 0 {
		s.epochsChunk = 3
	}

	if s.epochsThreshold <= 0 {
		s.epochsThreshold = 20
	}

	s.repo = dal.NewSyncerDal(s.db)

	// 初始化 syncer 高度
	if !s.dry {
		var syncer *po.SyncSyncer
		syncer, err = s.repo.GetSyncerOrNil(context.Background(), s.name)
		if err != nil {
			return
		}
		if syncer != nil {
			s.epoch = chain.Epoch(syncer.Epoch) + 1
			s.log.Infof("从数据库初始化高度: %s", s.epoch)
		} else {
			if s.initEpoch != nil {
				s.epoch = chain.Epoch(*s.initEpoch)
				s.log.Infof("从配置文件初始化高度: %s", s.epoch)
			} else {
				var final *londobell.Tipset
				final, err = s.getAggFinalTipset()
				if err != nil {
					return
				}
				s.epoch = chain.Epoch(final.ID - 1)
				s.log.Infof("从 Agg 初始化高度: %s", s.epoch)

			}
		}
	} else {
		s.epoch = chain.Epoch(*s.initEpoch)
		s.log.Infof("[Dry模式] 从配置文件初始化高度: %s", s.epoch)
	}

	return
}

func (s *Syncer) ManualSetEpoch(epoch chain.Epoch) {
	s.log.Infof("手动调整初始化高度: %s", epoch)
	s.epoch = epoch
}

func (s *Syncer) Quit() {
	s.quit <- struct{}{}
}

// Run 执行同步器
func (s *Syncer) Run() {

	timer := time.NewTimer(time.Second)
	defer func() {
		timer.Stop()
	}()

	for {
		select {
		case <-timer.C:
			s.run()
			if s.stopEpoch != nil && s.epoch.Int64() > *s.stopEpoch {
				s.log.Infof("高度: %s 超过截止高度: %s 退出同步", s.epoch, chain.Epoch(*s.stopEpoch))
				return
			}
			timer.Reset(time.Second)
		case <-s.quit:
			s.log.Infof("退出同步器")
			return
		}
	}
}

// 获取 AGG 最后高度
// 需要从 AGG 中取交易详情等东西，所以以 AGG 的最新高度为准
func (s *Syncer) getAggFinalTipset() (final *londobell.Tipset, err error) {

	ctx, cancle := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() {
		cancle()
	}()

	tipsets, err := s.agg.LatestTipset(ctx)
	if err != nil {
		err = fmt.Errorf("call agg latest tipset error: %s", err)
		return
	}
	if l := len(tipsets); l != 1 {
		err = fmt.Errorf("expect 1 final tipset, got: %d", l)
		return
	}
	final = tipsets[0]
	return
}

// 检查 Agg 同步的最新高度
// 如果同步器超过了该高度，则需要等待
func (s *Syncer) checkAggFinalTipsetHeight(final chain.Epoch) (sleep bool) {
	if s.epoch > final {
		sleep = true
		return
	}
	return
}

// 准备同步上下文
// 整个 Syncer 都共用一个 DataMap
func (s *Syncer) prepareTaskContext(datamap *Datamap, task Namer, epoch chain.Epoch, empty bool) (ctx *Context) {

	logger := s.log.With(
		zap.Int64("epoch", epoch.Int64()),
		zap.String("time", epoch.Format()),
	)

	if task.Name() != "" {
		logger = logger.With(zap.String("task", task.Name()))
	}

	ctx = &Context{
		context:       context.Background(),
		db:            s.db,
		agg:           s.agg,
		minerAgg:      s.minerAgg,
		adapter:       s.adapter,
		epoch:         epoch,
		datamap:       datamap,
		start:         time.Now(),
		SugaredLogger: logger,
		empty:         empty,
		dry:           s.dry,
	}
	return
}

// 执行同步任务
func (s *Syncer) run() {

	var err error
	var finalHeight chain.Epoch
	var final *londobell.Tipset
	final, err = s.getAggFinalTipset()
	if err != nil {
		return
	}
	finalHeight = chain.Epoch(final.ID - 1) // FinalHeight 拿的是 block 高度，会反复 revert

	next := chain.CurrentEpoch()
	if s.epoch == next {
		wait := (chain.CurrentEpoch() + 1).Time().Add(3 * time.Second).Sub(time.Now()) // 延迟一点，减少回滚
		s.log.Infof("等待高度 %s: %s", next, wait)
		time.Sleep(wait)
		return
	}

	if sleep := s.checkAggFinalTipsetHeight(finalHeight); sleep {
		d := 3 * time.Second
		s.log.Infof("期待高度: %s, 当前 AGG Final Tipset: %s, 等待: %s",
			s.epoch, finalHeight, d)
		time.Sleep(d)
		return
	}

	defer func() {
		if err != nil {
			switch err.(type) {
			case *mix.Warn:
				s.log.Warnf("高度 [%s,%s] 执行失败: %s, 等待: %s 重试", s.epoch, finalHeight, err, s.errorWaitDuration)
			default:
				if strings.Contains(err.Error(), "未获取到 miner 数据") ||
					strings.Contains(err.Error(), "未到") {
					s.log.Warnf("高度 [%s,%s] 执行失败: %s, 等待: %s 重试", s.epoch, finalHeight, err, s.errorWaitDuration)
				} else {
					s.log.Errorf("高度 [%s,%s] 执行错误: %s, 等待: %s 重试", s.epoch, finalHeight, err, s.errorWaitDuration) //多个并行时，单个出错 退出加报错，需要看看
				}
			}
			time.Sleep(s.errorWaitDuration)
		}
	}()

	err = s.sync(finalHeight)
	if err != nil {
		return
	}
}

// 计算并发执行任务
func (s *Syncer) sync(end chain.Epoch) (err error) {

	start := time.Now()
	if !s.dry {
		// 检查链的一致性
		checkStart := s.epoch - 10
		var rollEpoch chain.Epoch

		rollEpoch, err = s.CheckBlockChainConsistency(checkStart, s.epoch) // 检查当前链上epoch的数据 与之前所有数据的差距
		if err != nil {
			s.log.Infof("[%s,%s] 触发回滚: %s", start, s.epoch, err)
			err = s.Rollback(s.epoch, rollEpoch) //这里实际上是end是同步到了哪儿， rollEpoch是哪里发生了冲突
			return
		}
	}
	var runners []parallel.Runner[*po.SyncSyncerEpoch]
	count := int64(end - s.epoch + 1)

	// 当同步高度范围超过一个小时，限制到本次并发到一个小时结束
	// 以方便计算任务进行，否则计算任务将会一直积压
	threshold := s.epochsThreshold
	if count >= threshold {
		end = s.epoch + chain.Epoch(threshold) - 1
	}

	if s.stopEpoch != nil && *s.stopEpoch < end.Int64() {
		end = chain.Epoch(*s.stopEpoch) // 修正 End 不超过 Stop Epoch
	}

	total := int64(end - s.epoch + 1)
	left := atomic.NewInt64(total)
	syncerEpochs := map[int64]*po.SyncSyncerEpoch{}

	generate := func(epoch chain.Epoch) parallel.Runner[*po.SyncSyncerEpoch] {
		return func(_ context.Context) (*po.SyncSyncerEpoch, error) {
			r, e := s.execGroups(epoch, total, left, start, chain.NewLCRCRange(s.epoch, end))
			if e != nil {
				return nil, e
			}
			return r, nil
		}
	}
	for i := s.epoch; i <= end; i++ {
		runners = append(runners, generate(i))
	}

	s.log.Debugf("开始并发同步: from: %s  to: %s, 共 %d 个高度", s.epoch, end, total)

	results := parallel.Run[*po.SyncSyncerEpoch](context.TODO(), uint(s.epochsChunk), runners)
	err = parallel.Wait[*po.SyncSyncerEpoch](results, func(v *po.SyncSyncerEpoch) error {
		syncerEpochs[v.Epoch] = v //将同步成功的syncSyncerEpoch保存。数据来源于prepareSyncerEpoch(epoch chain.Epoch)，从agg拿数据
		return nil
	})
	if err != nil {
		return
	}

	// 一段高度追完后，则开始按高度顺序逐步执行计算任务
	for i := s.epoch; i <= end; i++ {
		now := time.Now()
		err = s.execCalculators(i, syncerEpochs[i.Int64()], chain.NewLCRCRange(s.epoch, end)) // 这里要执行计算，并把同步来，从agg prepare的数据存进去。才能判断是否分叉
		if err != nil {
			return
		}
		current := chain.CurrentEpoch()
		s.log.Infof("高度 %s 计算完成, 剩余: %d, 距最新高度: %s 还差: %d, 耗时: %s, 高度总耗时: %s",
			i, end-i, current, int64(current-i), time.Since(now), time.Since(start))
	}

	if !s.dry {
		// 检查链的一致性
		checkStart := end - 10
		var rollEpoch chain.Epoch

		rollEpoch, err = s.CheckConsistency(checkStart, end)
		if err != nil {

			s.log.Infof("[%s,%s] 触发回滚: %s", start, end, err)
			err = s.Rollback(end, rollEpoch) //这里实际上是end是同步到了哪儿， rollEpoch是哪里发生了冲突
			if err != nil {
				return
			}
			return
		}

		// 保存同步器最新的有效高度
		err = s.repo.SaveSyncer(context.Background(), &po.SyncSyncer{
			Name:  s.name,
			Epoch: end.Int64(),
		})
		if err != nil {
			return
		}
	}

	s.epoch = end + 1

	return
}

func (s *Syncer) CheckBlockChainConsistency(start, end chain.Epoch) (epoch chain.Epoch, err error) {
	epoch = s.epoch // 如果非回滚类错误，则重试执行一遍
	// 检查链一致性
	epochs, err := s.repo.GetSyncSyncerEpochs(context.Background(), s.name, chain.NewLCRCRange(start, end-1))
	if err != nil {
		return s.epoch, err
	}

	l := len(epochs)
	if l < 2 {
		s.log.Infof("高度: [%s, %s] ignore check consistency", start, end)
		return
	}

	defer func() {
		if err == nil {
			s.log.Infof("[%s,%s] 同步前的一致性检查通过，无链回滚现象, 长度: %d", start, end, l)
		}
	}()

	se := &po.SyncSyncerEpoch{}
	se, err = s.prepareSyncerEpoch(end)
	if err != nil {
		return epoch, err
	}

	a := se
	for i := 0; i < l; i++ {
		b := epochs[i]
		if chain.CompareStringsCidsEquals(a.ParentKeys, b.Keys) {
			if a.Epoch != se.Epoch { // 尤其要注意空高度的问题
				err = fmt.Errorf("链分叉: %s ParentKeys: %v not match %s Keys: %v",
					chain.Epoch(a.Epoch),
					a.ParentKeys,
					chain.Epoch(b.Epoch),
					b.Keys)
				epoch = chain.Epoch(b.Epoch)
				return epoch, err
			}
			break
		}
		a = b
	}
	return epoch, nil
}

// CheckConsistency 检查同步高度区块的一致性
func (s *Syncer) CheckConsistency(start, end chain.Epoch) (epoch chain.Epoch, err error) {
	epoch = s.epoch // 如果非回滚类错误，则重试执行一遍
	// 检查链一致性
	epochs, err := s.repo.GetSyncSyncerEpochs(context.Background(), s.name, chain.NewLCRCRange(start, end-1))
	if err != nil {
		return epoch, err
	}

	l := len(epochs)
	if l < 2 {
		s.log.Infof("高度: [%s, %s] ignore check consistency", start, end)
		return
	}

	defer func() {
		if err == nil {
			s.log.Infof("[%s,%s] 一致性检查通过, 长度: %d", start, end, l)
		}
	}()

	a := epochs[0]
	for i := 1; i < l; i++ {
		b := epochs[i]
		if !chain.CompareStringsCidsEquals(a.ParentKeys, b.Keys) {
			err = fmt.Errorf("链分叉: %s ParentKeys: %v not match %s Keys: %v",
				chain.Epoch(a.Epoch),
				a.ParentKeys,
				chain.Epoch(b.Epoch),
				b.Keys)
			epoch = chain.Epoch(b.Epoch)
			return epoch, err
		}
		a = b
	}

	// 检查同步任务一致性
	var tasks []string
	for _, v := range s.taskGroups {
		for _, vv := range v {
			tasks = append(tasks, vv.Name())
		}
	}
	for _, v := range s.calculators {
		tasks = append(tasks, v.Name())
	}

	tasksEpochs, err := s.repo.GetSyncTasksEpochs(context.Background(), tasks, chain.NewLCRCRange(s.epoch, end))
	if err != nil {
		return epoch, err
	}

	empties := map[int64]struct{}{}
	for _, v := range epochs {
		if v.Empty {
			empties[v.Epoch] = struct{}{}
		}
	}

	tasksMap := map[string]map[int64]struct{}{}
	for _, v := range tasksEpochs {
		if _, ok := tasksMap[v.Task]; !ok {
			tasksMap[v.Task] = map[int64]struct{}{}
		}
		tasksMap[v.Task][v.Epoch] = struct{}{}
	}
	for _, name := range tasks {
		if _, ok := tasksMap[name]; !ok {
			err = fmt.Errorf("任务高度 %s 缺失", name)
			return epoch, err
		}
		for i := s.epoch.Int64(); i <= end.Int64(); i++ {
			if _, ok := empties[i]; ok {
				continue
			}
			if _, ok := tasksMap[name][i]; !ok {
				err = fmt.Errorf("任务高度 %s 高度 %s 不一致", name, chain.Epoch(i))
				return epoch, err
			}
		}
	}

	return epoch, nil
}

func (s *Syncer) Rollback(from, to chain.Epoch) (err error) {

	tx := s.db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
			err = fmt.Errorf("回滚失败: %s", err)
			return
		}
	}()

	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("rollback recover error: %s", err)
			debug.PrintStack()
		}
	}()

	ctx := _dal.ContextWithDB(context.Background(), tx)

	for _, v := range s.calculators {
		err = v.RollBack(ctx, to)
		if err != nil {
			return
		}
		s.log.Infof("calculator %s rollback success at %s", v.Name(), to)
	}

	for _, v := range s.taskGroups {
		for _, vv := range v {
			err = vv.RollBack(ctx, to)
			if err != nil {
				return
			}
			s.log.Infof("task %s rollback success at %s", vv.Name(), to)
		}
	}

	for _, v := range s.globalRollback {
		err = v(ctx, to)
		if err != nil {
			return
		}
	}

	err = s.repo.DeleteSyncTaskEpochs(ctx, to, s.name)
	if err != nil {
		return
	}

	err = s.repo.DeleteSyncSyncerEpochs(ctx, to, s.name)
	if err != nil {
		return
	}

	err = s.repo.SaveSyncer(ctx, &po.SyncSyncer{
		Name:  s.name,
		Epoch: to.Int64() - 1,
	})
	if err != nil {
		return
	}

	err = tx.Commit().Error // 回滚在一个事务中，将所有数据库进行提交
	if err != nil {
		return
	}

	// 重置回滚高度
	s.epoch = to
	wait := 3 * time.Second
	s.log.Infof("链回滚成功，数据从: %s 回滚至: %s 高度, 回滚区间：%d 等待: %s", from, s.epoch, from-s.epoch+1, wait)
	time.Sleep(wait)

	return
}

type EpochEmpty struct {
	Epoch chain.Epoch
	Empty bool
}

func (s *Syncer) isEmpty(current chain.Epoch) (empty bool, parent []string, err error) {

	tipsets, err := s.agg.ParentTipset(context.Background(), current)
	if err != nil {
		return
	}

	if len(tipsets) == 0 {
		err = fmt.Errorf("get epoch: %s parent tipset is nil", current)
		return
	}

	parent = tipsets[0].Cids

	var ts []*londobell.Tipset
	ts, err = s.agg.Tipset(context.Background(), current)
	if err != nil {
		return
	}
	if len(ts) == 0 {
		empty = true
		return
	}

	return

}

func (s *Syncer) prepareSyncerEpoch(epoch chain.Epoch) (item *po.SyncSyncerEpoch, err error) {

	empty, parent, err := s.isEmpty(epoch)
	if err != nil {
		return
	}

	var key []string

	if !empty {
		var aggTipsets []*londobell.Tipset
		aggTipsets, err = s.agg.Tipset(context.Background(), epoch)
		if err != nil {
			return
		}
		if l := len(aggTipsets); l != 1 {
			err = fmt.Errorf("%s expect 1 tipest from agg, got: %d", epoch, l)
			return
		}
		key = aggTipsets[0].Cids
	} else {
		s.log.Infof("空高度: %s", epoch)
	}

	item = &po.SyncSyncerEpoch{
		Epoch:      epoch.Int64(),
		Name:       s.name,
		Keys:       key,
		ParentKeys: parent,
		Empty:      empty,
		Cost:       0,
	}

	return
}

// 并发执行任务分组
func (s *Syncer) execGroups(epoch chain.Epoch, total int64, left *atomic.Int64, start time.Time, epochs chain.LCRCRange) (r *po.SyncSyncerEpoch, err error) {

	now := time.Now()
	defer func() {
		if err == nil {
			left.Store(left.Sub(1))
			s.log.Debugf("%s 高度同步完成，共 %d 并发任务, 还剩 %d 个, 耗时: %s 并发总耗时: %s", epoch, total, left.Load(), time.Since(now), time.Since(start))
		}
	}()

	r, err = s.prepareSyncerEpoch(epoch)
	if err != nil {
		return
	}

	datamap := &Datamap{}

	for _, builder := range s.contextBuilders {
		err = builder(s.prepareTaskContext(datamap, EmptyName{}, epoch, r.Empty))
		if err != nil {
			switch err.(type) {
			case *mix.Warn:
				s.log.Warnf("epoch: %s 执行 ContextBuilder 失败: %s", epoch, err)
			default:
				s.log.Errorf("epoch: %s 执行 ContextBuilder 错误: %s", epoch, err)
			}
			return
		}
	}

	var runners []parallel.Runner[parallel.Null]
	generate := func(group TaskGroup) parallel.Runner[parallel.Null] {
		return func(_ context.Context) (parallel.Null, error) {
			for _, task := range group {
				ctx := s.prepareTaskContext(datamap, task, epoch, r.Empty)
				ctx.epochs = epochs
				e := s.execTaskOrCalculator(ctx, task, execTask)
				if e != nil {
					return nil, e
				}
			}
			return nil, nil
		}
	}

	for _, v := range s.taskGroups {
		runners = append(runners, generate(v))
	}

	results := parallel.Run[parallel.Null](context.TODO(), 10, runners)
	err = parallel.Wait[parallel.Null](results, nil)
	if err != nil {
		return
	}

	return
}

// 执行计算器
// 会按照同步的高度顺序执行
func (s *Syncer) execCalculators(epoch chain.Epoch, se *po.SyncSyncerEpoch, epochs chain.LCRCRange) (err error) {
	datamap := &Datamap{}
	for _, calc := range s.calculators {
		ctx := s.prepareTaskContext(datamap, calc, epoch, se.Empty) //上下文
		ctx.epochs = epochs
		ctx.lastCalc = epoch == epochs.LteEnd
		err = s.execTaskOrCalculator(ctx, calc, execCalculator)
		if err != nil {
			return
		}
	}

	if !s.dry {
		err = s.repo.SaveSyncSyncerEpoch(context.Background(), se)
		if err != nil {
			return
		}
	}

	return
}

type Execer interface {
	Calculator
	Task
}

const (
	execTask = iota
	execCalculator
)

func (s *Syncer) execTaskOrCalculator(ctx *Context, execer any, kind int) (err error) {

	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("recover error: %v", e)
			debug.PrintStack()
		}
	}()

	var name string
	var exec func(ctx *Context) error

	switch kind {
	case execCalculator:
		v := execer.(Calculator)
		name = v.Name()
		exec = func(c *Context) error {
			return v.Calc(c)
		}
	case execTask:
		v := execer.(Task)
		name = v.Name()
		exec = func(c *Context) error {
			return v.Exec(c)
		}
	default:
		err = fmt.Errorf("exec task or calculator unsupported: %d", kind)
		return
	}

	if !s.dry {
		// 判断计算器是否执行过
		var item *po.SyncTaskEpoch
		item, err = s.repo.GetSyncTaskEpochOrNil(ctx.context, ctx.Epoch(), name)
		if err != nil {
			return
		}
		if item != nil {
			return
		}
	}

	tx := s.db.Begin()
	now := time.Now()

	defer func() {
		if err != nil {
			switch err.(type) {
			case *mix.Warn:
				ctx.Warnf("%s 高度任务 %s 执行错误: %s", ctx.Epoch(), name, err)
			default:
				ctx.Errorf("%s 高度任务 %s 执行错误: %s", ctx.Epoch(), name, err)
			}
			tx.Rollback()
		} else {
			err = tx.Commit().Error
			if err != nil {
				return
			}
			//ctx.Errorf("%s 高度任务 %s 事务提交成功", ctx.Epoch(), name)
		}
	}()

	ctx.context = _dal.ContextWithDB(ctx.context, tx)
	err = exec(ctx)
	if err != nil {
		return
	}

	if !s.dry {
		// 保存任务
		err = s.repo.SaveSyncTaskEpochEpoch(ctx.context, &po.SyncTaskEpoch{
			Epoch:  ctx.Epoch().Int64(),
			Task:   name,
			Syncer: s.name,
			Cost:   chain.TimeCost(now),
		})
		if err != nil {
			return
		}
	}

	return
}
