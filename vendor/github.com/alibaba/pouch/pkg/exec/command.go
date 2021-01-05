package exec

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/contiv/executor"
	"github.com/sirupsen/logrus"
)

// Run returns running command result with timeout, returns are command exit code,
// stdout iostream, stderr iostream, error.
func Run(timeout time.Duration, bin string, args ...string) (int, string, string, error) {
	var cancel context.CancelFunc

	logrus.Debugf("run command: [%s] %v", bin, args)

	cmd := exec.Command(bin, args...)
	ctx := context.Background()
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	ret, err := executor.NewCapture(cmd).Run(ctx)
	if ret == nil {
		if err != nil {
			return -1, "", "", err
		}
		return -1, "", "", fmt.Errorf("unknown error")
	}
	return ret.ExitStatus, ret.Stdout, ret.Stderr, err
}

// RunWithRetry returns running command with "times" retries, must keep exit is 0 when execute success,
// returns are command exit code, stdout iostream, stderr iostream, error.
func RunWithRetry(times int, interval, timeout time.Duration, bin string, args ...string) (int, string, string, error) {
	var (
		ret    *executor.ExecResult
		err    error
		cancel context.CancelFunc
	)

	logrus.Debugf("run command with retry: [%s] %v", bin, args)

	for ; times > 0; times-- {
		cmd := exec.Command(bin, args...)
		ctx := context.Background()
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		ret, err = executor.NewCapture(cmd).Run(ctx)
		if err != nil || ret == nil || ret.ExitStatus != 0 {
			time.Sleep(interval)
			continue
		}

		return ret.ExitStatus, ret.Stdout, ret.Stderr, err
	}

	if ret != nil {
		return ret.ExitStatus, ret.Stdout, ret.Stderr, err
	}
	return -1, "", "", err
}

// Retry will run function with times, if err is nil, it will return nil,
// if over times, it will return last error.
func Retry(times int, interval time.Duration, f func() error) error {
	var err error
	for i := 0; i < times; i++ {
		err = f()
		if err == nil {
			return nil
		}
	}
	return err
}
