package redis

const (
	// 默认的分布式锁过期时间
	DefaultLockExpireSeconds = 10
	// 看门狗工作时间间隙
	WatchDogWorkStepSeconds = 5
)

type LockOption func(*LockOptions)

func WithBlock() LockOption {
	return func(o *LockOptions) {
		o.isBlock = true
	}
}

func WithBlockWaitingSeconds(waitingSeconds int64) LockOption {
	return func(o *LockOptions) {
		o.blockWaitingSeconds = waitingSeconds
	}
}

func WithExpireSeconds(expireSeconds int64) LockOption {
	return func(o *LockOptions) {
		o.expireSeconds = expireSeconds
	}
}

func repairLock(o *LockOptions) {
	if o.isBlock && o.blockWaitingSeconds <= 0 {
		// 默认阻塞等待时间上限为 5 秒
		o.blockWaitingSeconds = 5
	}

	// 倘若未设置分布式锁的过期时间，则会启动 watchdog
	if o.expireSeconds > 0 {
		return
	}

	// 用户未显式指定锁的过期时间，则此时会启动看门狗
	o.expireSeconds = DefaultLockExpireSeconds
	o.watchDogMode = true
}

type LockOptions struct {
	isBlock             bool
	blockWaitingSeconds int64
	expireSeconds       int64
	watchDogMode        bool
}
