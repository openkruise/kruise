[![ReportCard][ReportCard-Image]][ReportCard-URL] [![Build][Build-Status-Image]][Build-Status-URL] [![GoDoc][GoDoc-Image]][GoDoc-URL]

## Executor: flexible, high-level exec.Cmd for golang

Package executor implements a high level execution context with monitoring,
control, and logging features. It is made for services which execute lots of
small programs and need to carefully control i/o and processes.

Executor can:

  * Terminate on signal or after a timeout via /x/net/context
  * Output a message on an interval if the program is still running.
    The periodic message can be turned off by setting `LogInterval` of executor to a value <= 0
  * Capture split-stream stdio, and make it easier to get at io pipes.

Example:

```go
  e := executor.New(exec.Command("/bin/sh", "echo hello"))
  e.Start() // start
  fmt.Println(e.PID()) // get the pid
  fmt.Printf("%v\n", e) // pretty string output
  er, err := e.Wait(context.Background()) // wait for termination
  fmt.Println(er.ExitStatus) // => 0
  
  // lets capture some io, and timeout after a while
  e := executor.NewCapture(exec.Command("/bin/sh", "yes"))
  e.Start()
  ctx, _ := context.WithTimeout(context.Background(), 10 * time.Second)
  er, err := e.Wait(ctx) // wait for only 10 seconds
  fmt.Println(err == context.DeadlineExceeded)
  fmt.Println(er.Stdout) // yes\nyes\nyes\n...
```

## Authors:

* Erik Hollensbe

## Sponsorship

Project Contiv is sponsored by Cisco Systems, Inc.

[ReportCard-URL]: https://goreportcard.com/report/github.com/contiv/executor
[ReportCard-Image]: http://goreportcard.com/badge/contiv/executor
[Build-Status-URL]: http://travis-ci.org/contiv/executor
[Build-Status-Image]: https://travis-ci.org/contiv/executor.svg?branch=master
[GoDoc-URL]: https://godoc.org/github.com/contiv/executor
[GoDoc-Image]: https://godoc.org/github.com/contiv/executor?status.svg
