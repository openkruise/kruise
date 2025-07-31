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

package writer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openkruise/kruise/pkg/webhook/util/generator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fakes & Helpers ---

type fakeCertGenerator struct{}

func (g *fakeCertGenerator) Generate(dnsName string) (*generator.Artifacts, error) {
	return &generator.Artifacts{
		CAKey:  []byte("fake-ca-key"),
		CACert: []byte("fake-ca-cert"),
		Cert:   []byte("fake-cert"),
		Key:    []byte("fake-key"),
	}, nil
}

func (g *fakeCertGenerator) SetCA(_ []byte, _ []byte) {
	// no-op for test
}

func setupTestDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// --- Tests ---

func TestNewFSCertWriter_Validation(t *testing.T) {
	cases := []struct {
		name    string
		options FSCertWriterOptions
		wantErr bool
	}{
		{
			name: "valid options",
			options: FSCertWriterOptions{
				Path:          t.TempDir(),
				CertGenerator: &fakeCertGenerator{},
			},
			wantErr: false,
		},
		{
			name:    "missing path",
			options: FSCertWriterOptions{},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewFSCertWriter(c.options)
			if c.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFSCertWriter_EnsureCert_WritesCerts(t *testing.T) {
	dir := setupTestDir(t)

	writer, err := NewFSCertWriter(FSCertWriterOptions{
		Path:          dir,
		CertGenerator: &fakeCertGenerator{},
	})
	require.NoError(t, err)

	fsWriter, ok := writer.(*fsCertWriter)
	require.True(t, ok, "expected writer to be *fsCertWriter")

	artifacts, changed, err := fsWriter.EnsureCert("test.example.com")
	require.NoError(t, err)
	assert.True(t, changed)

	t.Run("artifacts match", func(t *testing.T) {
		assert.Equal(t, []byte("fake-cert"), artifacts.Cert)
		assert.Equal(t, []byte("fake-key"), artifacts.Key)
	})

	t.Run("files written", func(t *testing.T) {
		expectedFiles := []string{
			CAKeyName, CACertName,
			ServerCertName, ServerCertName2,
			ServerKeyName, ServerKeyName2,
		}
		for _, name := range expectedFiles {
			t.Run(name, func(t *testing.T) {
				_, err := os.Stat(filepath.Join(dir, name))
				assert.NoError(t, err)
			})
		}
	})
}

func TestFSCertWriter_Read_ReturnsCorrectArtifacts(t *testing.T) {
	dir := setupTestDir(t)

	writer, err := NewFSCertWriter(FSCertWriterOptions{
		Path:          dir,
		CertGenerator: &fakeCertGenerator{},
	})
	require.NoError(t, err)

	fsWriter, ok := writer.(*fsCertWriter)
	require.True(t, ok)

	_, _, err = fsWriter.EnsureCert("test.example.com")
	require.NoError(t, err)

	readArtifacts, err := fsWriter.read()
	require.NoError(t, err)

	assert.Equal(t, []byte("fake-ca-key"), readArtifacts.CAKey)
	assert.Equal(t, []byte("fake-ca-cert"), readArtifacts.CACert)
	assert.Equal(t, []byte("fake-cert"), readArtifacts.Cert)
	assert.Equal(t, []byte("fake-key"), readArtifacts.Key)
}

func TestFSCertWriter_Read_FailsIfMissingFiles(t *testing.T) {
	dir := setupTestDir(t)

	writer, err := NewFSCertWriter(FSCertWriterOptions{Path: dir})
	require.NoError(t, err)

	fsWriter, ok := writer.(*fsCertWriter)
	require.True(t, ok)

	_, err = fsWriter.read()
	assert.Error(t, err, "Expected read to fail with missing cert files")
}
