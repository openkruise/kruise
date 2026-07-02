/*
Copyright 2024 The Kruise Authors.

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
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
)

func TestFilterCloseErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "unknown error returns false",
			err:  errors.New("some unknown error"),
			want: false,
		},
		{
			name: "direct io.EOF returns true",
			err:  io.EOF,
			want: true,
		},
		{
			name: "wrapped io.EOF returns true",
			err:  fmt.Errorf("read failed: %w", io.EOF),
			want: true,
		},
		{
			name: "use of closed network connection string returns true",
			err:  errors.New("read tcp: use of closed network connection"),
			want: true,
		},
		{
			name: "connect no such file or directory returns true",
			err:  errors.New("dial unix /var/run/foo.sock: connect: no such file or directory"),
			want: true,
		},
		{
			name: "net.OpError write EPIPE returns true",
			err: &net.OpError{
				Op: "write",
				Err: &os.SyscallError{
					Syscall: "write",
					Err:     syscall.EPIPE,
				},
			},
			want: true,
		},
		{
			name: "net.OpError read ECONNRESET returns true",
			err: &net.OpError{
				Op: "read",
				Err: &os.SyscallError{
					Syscall: "read",
					Err:     syscall.ECONNRESET,
				},
			},
			want: true,
		},
		{
			name: "net.OpError write ECONNRESET (not EPIPE) returns false",
			err: &net.OpError{
				Op: "write",
				Err: &os.SyscallError{
					Syscall: "write",
					Err:     syscall.ECONNRESET,
				},
			},
			want: false,
		},
		{
			name: "net.OpError read EPIPE (not ECONNRESET) returns false",
			err: &net.OpError{
				Op: "read",
				Err: &os.SyscallError{
					Syscall: "read",
					Err:     syscall.EPIPE,
				},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterCloseErr(tc.err)
			if got != tc.want {
				t.Errorf("FilterCloseErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
