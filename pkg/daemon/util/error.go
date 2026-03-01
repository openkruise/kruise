/*
Copyright 2021 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"io"
	"net"
	"os"
	"strings"
	"syscall"
)

// FilterCloseErr rewrites EOF and EPIPE errors to bool. Use when
// returning from call or handling errors from main read loop.
//
// This purposely ignores errors with a wrapped cause.
func FilterCloseErr(err error) bool {
	switch {
	case err == io.EOF:
		return true
	case strings.Contains(err.Error(), io.EOF.Error()):
		return true
	case strings.Contains(err.Error(), "use of closed network connection"):
		return true
	case strings.Contains(err.Error(), "connect: no such file or directory"):
		return true
	default:
		// if we have an epipe on a write or econnreset on a read , we cast to errclosed
		if oerr, ok := err.(*net.OpError); ok && (oerr.Op == "write" || oerr.Op == "read") {
			serr, sok := oerr.Err.(*os.SyscallError)
			if sok && ((serr.Err == syscall.EPIPE && oerr.Op == "write") ||
				(serr.Err == syscall.ECONNRESET && oerr.Op == "read")) {

				return true
			}
		}
	}

	return false
}
