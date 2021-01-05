package utils

import (
	"fmt"
	"strings"
	"time"

	"github.com/alibaba/pouch/pkg/exec"
)

// MakeFSVolume is used to make file system on device with format type and options.
func MakeFSVolume(fscmd []string, devicePath string, timeout time.Duration) error {
	args := []string{"-t"}
	args = append(args, fscmd...)
	args = append(args, devicePath)
	exit, stdout, stderr, err := exec.Run(timeout, "mkfs", args...)
	if err != nil || exit != 0 {
		return fmt.Errorf("error creating filesystem on %s with cmd: %s. Error: (%v) (%v) (%v)",
			devicePath, fscmd, err, strings.TrimSpace(stdout), strings.TrimSpace(stderr))
	}

	return nil
}

// MountVolume is used to mount device to directory with options.
func MountVolume(mountCmd []string, devicePath, mountPath string, timeout time.Duration) error {
	args := []string{"-t"}
	args = append(args, mountCmd...)
	args = append(args, devicePath)
	args = append(args, mountPath)
	exit, stdout, stderr, err := exec.Run(timeout, "mount", args...)
	if err != nil || exit != 0 {
		return fmt.Errorf("error mount %s on %s with cmd: %s. Error: (%v) (%v) (%v)",
			devicePath, mountPath, mountCmd, err, strings.TrimSpace(stdout), strings.TrimSpace(stderr))
	}

	return nil
}

// IsMountpoint is used to check the directory is mountpoint or not.
func IsMountpoint(dir string) bool {
	exit, _, _, err := exec.Run(10*time.Second, "mountpoint", dir)
	if err != nil {
		return false
	} else if exit != 0 {
		return false
	}

	return true
}
