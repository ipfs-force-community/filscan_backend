package _app

import (
	logging "github.com/ipfs/go-log/v2"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	signals  = make(chan os.Signal, 1)
	handlers []Handler
	lock     = sync.Mutex{}
	pid      int
)

var log = logging.Logger("utils:exit")

func init() {
	lvl, _ := logging.LevelFromString("debug")
	logging.SetAllLoggers(lvl)
	logging.SetupLogging(logging.Config{
		Stdout: true,
	})
	pid = os.Getpid()
	signal.Notify(signals,
		os.Interrupt,
		os.Kill,
		syscall.SIGINT,
		syscall.SIGTERM,
	)
}

type Handler func() error

func AddExitHandler(handler Handler) {
	lock.Lock()
	defer lock.Unlock()
	handlers = append(handlers, handler)
}

func Pid() int {
	return pid
}

func Exit() {
	signals <- os.Interrupt
}

func ExitDone() {
	err := execHandles()
	if err != nil {
		log.Errorf("pid: %d exit failed, error: %s", pid, err)
	} else {
		log.Infof("pid: %d, exit done", pid)
		os.Exit(0)
	}
}

func WaitExitSignal() <-chan os.Signal {
	exit := make(chan os.Signal, 1)
	go func() {
		for {
			select {
			case s := <-signals:
				log.Infof("pid: %d, received signal '%s' and exiting...", pid, s)
				err := execHandles()
				if err != nil {
					log.Errorf("pid: %d exit failed, error: %s", pid, err)
				} else {
					log.Infof("pid: %d, exit done", pid)
					close(signals)
					exit <- s
					return
				}
			}
		}
	}()
	return exit
}

func WaitExit() {
	for {
		select {
		case <-signals:
			log.Infof("pid: %d, exiting...", pid)
			err := execHandles()
			if err != nil {
				log.Errorf("pid: %d exit failed, error: %s", pid, err)
			} else {
				log.Infof("pid: %d, exit done", pid)
				close(signals)
				os.Exit(0)
			}
		}
	}
}

func execHandles() (err error) {
	lock.Lock()
	defer lock.Unlock()
	for _, handler := range handlers {
		err = handler()
		if err != nil {
			return
		}
	}
	return
}
