package calc_change_actor_task

import (
	"context"
	"errors"
	"testing"

	"github.com/gozelle/mix"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// fakeChangeActorRepo 只覆写本用例用到的仓储方法：内嵌接口让其余方法保持
// 「未实现即 panic」，避免用例在没走到的分支上假绿。
type fakeChangeActorRepo struct {
	repository.ChangeActorTask
	balances     []*po.ActorBalance
	balancesErr  error
	getCalls     int
	queriedEpoch chain.Epoch
}

func (f *fakeChangeActorRepo) GetActorBalances(ctx context.Context, epoch chain.Epoch) ([]*po.ActorBalance, error) {
	f.getCalls++
	f.queriedEpoch = epoch
	return f.balances, f.balancesErr
}

// fakeAgg 只覆写 ParentTipset：用于证明「有余额时确实越过了空余额守卫、继续走原流程」。
type fakeAgg struct {
	londobell.Agg
	err   error
	calls int
}

func (f *fakeAgg) ParentTipset(ctx context.Context, start chain.Epoch) ([]*londobell.ParentTipset, error) {
	f.calls++
	return nil, f.err
}

// TestCalcEmptyBalancesReturnsRetryableErrorInsteadOfSilentSuccess 覆盖「balances 未落库时静默空跑、
// 仍被记为已执行」这条漏数路径（实测 [6280000,6412000] 内漏 1333 个高度）：
// 旧实现 `if len(balances) == 0 { return }` 以 nil（成功）返回，框架据此写 chain.sync_task_epochs，
// 该高度此后每轮都被当成「已执行」跳过、永不再算。
func TestCalcEmptyBalancesReturnsRetryableErrorInsteadOfSilentSuccess(t *testing.T) {

	repo := &fakeChangeActorRepo{} // 空余额且无错误：旧实现正是在这里静默成功
	task := NewCalcChangeActorTask(repo)

	epoch := chain.Epoch(6280001)
	ctx := syncer.NewTestContext(nil, nil, epoch)

	err := task.Calc(ctx)

	require.Error(t, err, "balances 为空不得以成功返回（否则该高度被记为已执行、永不再算）")
	require.Contains(t, err.Error(), "6280001", "错误信息必须带高度，便于按高度定位")
	require.Contains(t, err.Error(), "actor_balances", "错误信息必须点明是余额未落库")

	// 必须是「可重试」：*mix.Warn ⇒ 框架按 Warn 记录该高度，且分类为未就绪 ⇒
	// 不计入/不清零「连续 N 次失败即跳过」防线，下一轮会重算该高度。
	var warn *mix.Warn
	require.True(t, errors.As(err, &warn), "应当用 *mix.Warn（框架按 Warn 级别留痕、按未就绪重试）")
	require.Equal(t, syncer.ErrorKindNotReady, syncer.ClassifySyncError(err),
		"必须分类为未就绪：既不被跳过防线计入、也不以成功落 chain.sync_task_epochs")

	require.Equal(t, 1, repo.getCalls)
	require.Equal(t, epoch, repo.queriedEpoch, "必须按当前高度查余额（查错高度会静默取到空集）")
}

// TestCalcKeepsRepoError 保证新增分支没有吞掉仓储错误
func TestCalcKeepsRepoError(t *testing.T) {

	repoErr := errors.New("db down")
	repo := &fakeChangeActorRepo{balancesErr: repoErr}
	task := NewCalcChangeActorTask(repo)
	ctx := syncer.NewTestContext(nil, nil, chain.Epoch(6280002))

	err := task.Calc(ctx)

	require.ErrorIs(t, err, repoErr, "仓储报错原样上抛，不得被空余额分支覆盖")
}

// TestCalcGoesPastEmptyGuardWhenBalancesExist 保证守卫只拦「空余额」：
// 有余额时必须继续走原流程（这里用「ParentTipset 报错」证明已越过守卫并调到了 agg）。
func TestCalcGoesPastEmptyGuardWhenBalancesExist(t *testing.T) {

	aggErr := errors.New("agg parent tipset failed")
	repo := &fakeChangeActorRepo{balances: []*po.ActorBalance{{Epoch: 6280003, ActorId: "f0100"}}}
	agg := &fakeAgg{err: aggErr}
	task := NewCalcChangeActorTask(repo)

	ctx := syncer.NewTestContext(nil, agg, chain.Epoch(6280003))

	err := task.Calc(ctx)

	require.ErrorIs(t, err, aggErr)
	require.Equal(t, 1, agg.calls, "有余额时必须继续走原流程（调用 agg 取父 tipset）")
	require.Equal(t, 1, repo.getCalls)
}
