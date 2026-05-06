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

	"github.com/onsi/gomega"
	"github.com/openkruise/kruise/pkg/webhook/util/generator"
)

func TestPrepareToWrite(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tmpDir, err := os.MkdirTemp("", "webhook-certs-*")
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer os.RemoveAll(tmpDir)

	// Test with a completely new directory that doesn't exist
	newDir := filepath.Join(tmpDir, "new-certs")
	err = prepareToWrite(newDir)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Verify directory was created with the correct permissions
	info, err := os.Stat(newDir)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(info.IsDir()).To(gomega.BeTrue())
	// Depending on umask, the permission might be 0750 or less, we just check it exists.
	
	// Test again with an existing directory
	err = prepareToWrite(newDir)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Write dummy files and verify prepareToWrite cleans non-symlinks
	dummyFile := filepath.Join(newDir, ServerCertName)
	err = os.WriteFile(dummyFile, []byte("dummy cert"), 0640)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	
	err = prepareToWrite(newDir)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	
	// File should be deleted because it was not a symlink
	_, err = os.Stat(dummyFile)
	g.Expect(os.IsNotExist(err)).To(gomega.BeTrue())
}

func TestCertToProjectionMap(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	
	artifacts := &generator.Artifacts{
		CAKey:  []byte("ca-key"),
		CACert: []byte("ca-cert"),
		Key:    []byte("server-key"),
		Cert:   []byte("server-cert"),
	}
	
	projectionMap := certToProjectionMap(artifacts)
	
	// Check keys and their modes
	g.Expect(projectionMap).To(gomega.HaveKey(CAKeyName))
	g.Expect(projectionMap[CAKeyName].Mode).To(gomega.Equal(int32(0600)))
	g.Expect(projectionMap[CAKeyName].Data).To(gomega.Equal([]byte("ca-key")))

	g.Expect(projectionMap).To(gomega.HaveKey(CACertName))
	g.Expect(projectionMap[CACertName].Mode).To(gomega.Equal(int32(0640)))
	g.Expect(projectionMap[CACertName].Data).To(gomega.Equal([]byte("ca-cert")))

	g.Expect(projectionMap).To(gomega.HaveKey(ServerKeyName))
	g.Expect(projectionMap[ServerKeyName].Mode).To(gomega.Equal(int32(0600)))
	g.Expect(projectionMap[ServerKeyName].Data).To(gomega.Equal([]byte("server-key")))

	g.Expect(projectionMap).To(gomega.HaveKey(ServerCertName))
	g.Expect(projectionMap[ServerCertName].Mode).To(gomega.Equal(int32(0640)))
	g.Expect(projectionMap[ServerCertName].Data).To(gomega.Equal([]byte("server-cert")))

	g.Expect(projectionMap).To(gomega.HaveKey(ServerKeyName2))
	g.Expect(projectionMap[ServerKeyName2].Mode).To(gomega.Equal(int32(0600)))

	g.Expect(projectionMap).To(gomega.HaveKey(ServerCertName2))
	g.Expect(projectionMap[ServerCertName2].Mode).To(gomega.Equal(int32(0640)))
}

func TestFSCertWriter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tmpDir, err := os.MkdirTemp("", "webhook-certs-*")
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer os.RemoveAll(tmpDir)

	writer, err := NewFSCertWriter(FSCertWriterOptions{
		Path: tmpDir,
	})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	dnsName := "kruise-webhook-service.svc"
	certs, changed, err := writer.EnsureCert(dnsName)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(changed).To(gomega.BeTrue())
	g.Expect(certs).NotTo(gomega.BeNil())
	
	// Try reading the written certs
	certs2, changed2, err := writer.EnsureCert(dnsName)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(changed2).To(gomega.BeFalse())
	g.Expect(certs2.Cert).To(gomega.Equal(certs.Cert))
}
