/*
Copyright 2025 The Kruise Authors.

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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitProtectionLogger_Success(t *testing.T) {
	// Save and restore original global state to ensure test isolation.
	origLogPath := protectionLogPath
	origLogger := protectionLogger
	defer func() {
		protectionLogPath = origLogPath
		protectionLogger = origLogger
	}()

	// Create a temporary directory that is automatically cleaned up.
	tempDir := t.TempDir()
	protectionLogPath = tempDir

	err := InitProtectionLogger()
	require.NoError(t, err, "InitProtectionLogger() should not fail in a valid directory")

	logFile := filepath.Join(tempDir, "protection.log")
	assert.FileExists(t, logFile, "Log file should be created after successful initialization")
}

func TestLoggerProtectionInfo_LogsCorrectly(t *testing.T) {
	// Save and restore original global state.
	origLogPath := protectionLogPath
	origLogger := protectionLogger
	defer func() {
		protectionLogPath = origLogPath
		protectionLogger = origLogger
	}()

	tempDir := t.TempDir()
	protectionLogPath = tempDir
	err := InitProtectionLogger()
	require.NoError(t, err, "Logger initialization is a prerequisite for this test")

	testData := ProtectionLoggerInfo{
		Event:     "TestEvent",
		Kind:      "Pod",
		Namespace: "default",
		Name:      "test-pod",
		UserAgent: "unit-test",
	}
	LoggerProtectionInfo(testData.Event, testData.Kind, testData.Namespace, testData.Name, testData.UserAgent)

	// Read the log file and verify its content.
	logFile := filepath.Join(tempDir, "protection.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err, "Failed to read log file")

	var loggedInfo ProtectionLoggerInfo
	err = json.Unmarshal(content[:len(content)-1], &loggedInfo)
	require.NoError(t, err, "Logged content should be valid JSON")

	assert.Equal(t, testData, loggedInfo, "Logged data should match the input data")
}

func TestLoggerProtectionInfo_UninitializedNoOp(t *testing.T) {
	origLogPath := protectionLogPath
	origLogger := protectionLogger
	defer func() {
		protectionLogPath = origLogPath
		protectionLogger = origLogger
	}()

	protectionLogger = nil
	protectionLogPath = t.TempDir()

	LoggerProtectionInfo("event", "kind", "ns", "name", "agent")

	logFile := filepath.Join(protectionLogPath, "protection.log")
	assert.NoFileExists(t, logFile, "Log file should not be created without initialization")
}

func TestInitProtectionLogger_MkdirFailure(t *testing.T) {
	// Save and restore original global state.
	origLogPath := protectionLogPath
	origLogger := protectionLogger
	defer func() {
		protectionLogPath = origLogPath
		protectionLogger = origLogger
	}()

	tempDir := t.TempDir()
	blockingFile := filepath.Join(tempDir, "file.txt")
	err := os.WriteFile(blockingFile, []byte("blocker"), 0644)
	require.NoError(t, err)

	// Attempt to initialize the logger in a path that is blocked by the file.
	protectionLogPath = filepath.Join(blockingFile, "subdir")
	err = InitProtectionLogger()

	assert.Error(t, err, "Expected an error but got nil")
	assert.True(t, strings.Contains(err.Error(), "MkdirAll"), "Error message should mention MkdirAll")
	assert.Nil(t, protectionLogger, "Logger should remain nil after a failed initialization")
}

func TestInitProtectionLogger_OpenFileFailure(t *testing.T) {
	// Save and restore original global state.
	origLogPath := protectionLogPath
	origLogger := protectionLogger
	defer func() {
		protectionLogPath = origLogPath
		protectionLogger = origLogger
	}()

	// Create a temporary directory for the logs.
	tempDir := t.TempDir()
	protectionLogPath = tempDir

	err := os.MkdirAll(filepath.Join(tempDir, "protection.log"), 0755)
	require.NoError(t, err)

	err = InitProtectionLogger()

	assert.Error(t, err, "Expected an error but got nil")
	assert.True(t, strings.Contains(err.Error(), "openFile"), "Error message should mention openFile")
	assert.Nil(t, protectionLogger, "Logger should remain nil after a failed initialization")
}
