/*
Copyright 2023 The Kruise Authors.

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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

var (
	protectionLogger  *log.Logger
	protectionLogPath string
)

const (
	ProtectionEventPub                = "PodUnavailableBudget"
	ProtectionEventDeletionProtection = "DeletionProtection"
)

type ProtectionLoggerInfo struct {
	// PUB, ProtectionDeletion
	Event     string
	Kind      string
	Namespace string
	Name      string
	UserAgent string
}

func init() {
	flag.StringVar(&protectionLogPath, "protection-log-path", "/log", "protection log path, for example pub, delete_protection")
}
func InitProtectionLogger() error {
	err := os.MkdirAll(protectionLogPath, 0644)
	if err != nil {
		return fmt.Errorf("MkdirAll(%s) failed: %s", protectionLogPath, err.Error())
	}
	file, err := os.OpenFile(filepath.Join(protectionLogPath, "protection.log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("openFile(%s) failed: %s", filepath.Join(protectionLogPath, "protection.log"), err.Error())
	}
	protectionLogger = log.New(file, "", 0)
	return nil
}

func LoggerProtectionInfo(event, kind, ns, name, userAgent string) {
	// compatible with go test
	if protectionLogger == nil {
		return
	}
	info := ProtectionLoggerInfo{
		Event:     event,
		Kind:      kind,
		Namespace: ns,
		Name:      name,
		UserAgent: userAgent,
	}
	by, _ := json.Marshal(info)
	protectionLogger.Println(string(by))
}
