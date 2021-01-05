// Package executor implements a high level execution context with monitoring,
// control, and logging features. It is made for services which execute lots of
// small programs and need to carefully control i/o and processes.
//
// Executor can:
//
//   * Terminate on signal or after a timeout via /x/net/context
//   * Output a message on an interval if the program is still running.
//     The periodic message can be turned off by setting `LogInterval` of executor to a value <= 0
//   * Capture split-stream stdio, and make it easier to get at io pipes.
//
// Example:
//
//		e := executor.New(exec.Command("/bin/sh", "echo hello"))
//		e.Start() // start
//		fmt.Println(e.PID()) // get the pid
//		fmt.Printf("%v\n", e) // pretty string output
//		er, err := e.Wait(context.Background()) // wait for termination
//		fmt.Println(er.ExitStatus) // => 0
//
//		// lets capture some io, and timeout after a while
//		e := executor.NewCapture(exec.Command("/bin/sh", "yes"))
// 		e.Start()
//		ctx, _ := context.WithTimeout(context.Background(), 10 * time.Second)
//		er, err := e.Wait(ctx) // wait for only 10 seconds
//		fmt.Println(err == context.DeadlineExceeded)
//		fmt.Println(er.Stdout) // yes\nyes\nyes\n...
//
package executor

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"github.com/sirupsen/logrus"
)

// ExecResult is the result of a Wait() operation and contains various fields
// related to the post-mortem state of the process such as output and exit
// status.
type ExecResult struct {
	Stdout     string
	Stderr     string
	ExitStatus int
	Runtime    time.Duration

	executor *Executor
}

// Executor is the context used to execute a process. The runtime state is kept
// here. Please see the struct fields for more information.
//
// New(), NewIO(), or NewCapture() are the appropriate ways to initialize this type.
//
// No attempt is made to manage concurrent requests to this struct after
// the program has started.
type Executor struct {
	// The interval at which we will log that we are still running.
	LogInterval time.Duration

	// The function used for logging. Expects a format-style string and trailing args.
	LogFunc func(string, ...interface{})

	// The stdin as passed to the process.
	Stdin io.Reader

	io              bool
	capture         bool
	command         *exec.Cmd
	stdout          io.ReadCloser
	stderr          io.ReadCloser
	stdoutBuf       bytes.Buffer
	stderrBuf       bytes.Buffer
	startTime       time.Time
	terminateLogger chan struct{}
}

// New creates a new executor from an *exec.Cmd. You may modify the values
// before calling Start(). See Executor for more information. Use NewCapture if
// you want executor to capture output for you.
func New(cmd *exec.Cmd) *Executor {
	return newExecutor(false, false, cmd)
}

// NewIO creates a new executor but allows the Out() and Err() methods to provide
// a io.ReadCloser as a pipe from the stdout and error respectively. If you
// wish to read large volumes of output this is the way to go.
func NewIO(cmd *exec.Cmd) *Executor {
	return newExecutor(true, false, cmd)
}

// NewCapture creates an instance of executor suitable for capturing output.
// The Wait() call will automatically yield the stdout and stderr of the
// program. NOTE: this can potentially use unbounded amounts of ram; use carefully.
func NewCapture(cmd *exec.Cmd) *Executor {
	return newExecutor(true, true, cmd)
}

func newExecutor(useIO, useCapture bool, cmd *exec.Cmd) *Executor {
	return &Executor{
		io:              useIO,
		capture:         useCapture,
		LogInterval:     1 * time.Minute,
		LogFunc:         logrus.Debugf,
		command:         cmd,
		stdout:          nil,
		stderr:          nil,
		terminateLogger: make(chan struct{}),
	}
}

func (e *Executor) String() string {
	return fmt.Sprintf("%v (%v) (pid: %v)", e.command.Args, e.command.Path, e.PID())
}

// Start starts the command in the Executor context. It returns any error upon
// starting the process, but does not wait for it to complete. You may control
// it in a variety of ways (see Executor for more information).
func (e *Executor) Start() error {
	e.command.Stdin = e.Stdin

	e.startTime = time.Now()

	var err error

	if e.io {
		e.stdout, err = e.command.StdoutPipe()
		if err != nil {
			return err
		}

		e.stderr, err = e.command.StderrPipe()
		if err != nil {
			return err
		}
	}

	if err := e.command.Start(); err != nil {
		e.LogFunc("Error executing %v: %v", e, err)
		return err
	}

	if e.LogInterval > 0 {
		go e.logInterval()
	}

	return nil
}

// TimeRunning returns the amount of time the program is or was running. Also
// see ExecResult.Runtime.
func (e *Executor) TimeRunning() time.Duration {
	return time.Now().Sub(e.startTime)
}

func (e *Executor) logInterval() {
	for {
		select {
		case <-e.terminateLogger:
			return
		case <-time.After(e.LogInterval):
			e.LogFunc("%v has been running for %v", e, e.TimeRunning())
		}
	}
}

// PID yields the pid of the process (dead or alive), or 0 if the process has
// not been run yet.
func (e *Executor) PID() uint32 {
	if e.command.Process != nil {
		return uint32(e.command.Process.Pid)
	}

	return 0
}

// Wait waits for the process and return an ExecResult and any error it
// encountered along the way. While the error may or may not be nil, the
// ExecResult will always exist with as much information as we could get.
//
// Context is from https://godoc.org/golang.org/x/net/context (see
// https://blog.golang.org/context for usage). You can use it to set timeouts
// and cancel executions.
func (e *Executor) Wait(ctx context.Context) (*ExecResult, error) {
	defer close(e.terminateLogger)

	var err error
	errChan := make(chan error, 1)

	go func() {
		// make sure we have captured all output before we wait
		if e.capture {
			if _, err := bufio.NewReader(e.stdout).WriteTo(&e.stdoutBuf); err != nil {
				e.LogFunc("error reading stdout: %v", err)
			}
			if _, err := bufio.NewReader(e.stderr).WriteTo(&e.stderrBuf); err != nil {
				e.LogFunc("error reading stderr: %v", err)
			}
		}
		errChan <- e.command.Wait()
	}()

	select {
	case <-ctx.Done():
		if e.command.Process == nil {
			e.LogFunc("Could not terminate non-running command %v", e)
		} else {
			e.LogFunc("Command %v terminated due to timeout or cancellation. It may not have finished!", e)
			e.command.Process.Kill()
		}
		err = ctx.Err()
	case err = <-errChan:
	}

	res := &ExecResult{executor: e}

	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			res.ExitStatus = int(exit.ProcessState.Sys().(syscall.WaitStatus) / 256)
		}
	}

	if e.capture {
		res.Stdout = e.stdoutBuf.String()
		res.Stderr = e.stderrBuf.String()
	}

	res.Runtime = e.TimeRunning()

	return res, err
}

// Run calls Start(), then Wait(), and returns an ExecResult and error (if
// any). The error may be of many types including *exec.ExitError and
// context.Canceled, context.DeadlineExceeded.
func (e *Executor) Run(ctx context.Context) (*ExecResult, error) {
	if err := e.Start(); err != nil {
		return nil, err
	}

	er, err := e.Wait(ctx)
	if err != nil {
		return er, err
	}

	return er, nil
}

// Out returns an *os.File which is the stream of the standard output stream.
func (e *Executor) Out() io.ReadCloser {
	return e.stdout
}

// Err returns an io.ReadCloser which is the stream of the standard error stream.
func (e *Executor) Err() io.ReadCloser {
	return e.stderr
}

func (er *ExecResult) String() string {
	return fmt.Sprintf("Command: %v, Exit status %v, Runtime %v", er.executor.command.Args, er.ExitStatus, er.Runtime)
}
