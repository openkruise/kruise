/*
Copyright 2026 The Kruise Authors.

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
	"path"
	"testing"

	"github.com/onsi/gomega"

	"github.com/openkruise/kruise/pkg/webhook/util/generator"
)

func TestWriteCertsToDirPermissions(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Use a directory that does not exist yet so that prepareToWrite creates it.
	dir := path.Join(t.TempDir(), "certs")
	certs := &generator.Artifacts{
		CAKey:  []byte("ca-key"),
		CACert: []byte("ca-cert"),
		Key:    []byte("key"),
		Cert:   []byte("cert"),
	}
	g.Expect(WriteCertsToDir(dir, certs)).Should(gomega.Succeed())

	dirInfo, err := os.Stat(dir)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(dirInfo.Mode().Perm()).Should(gomega.Equal(os.FileMode(0700)))

	expectedModes := map[string]os.FileMode{
		CAKeyName:       0600,
		ServerKeyName:   0600,
		ServerKeyName2:  0600,
		CACertName:      0644,
		ServerCertName:  0644,
		ServerCertName2: 0644,
	}
	for name, mode := range expectedModes {
		// os.Stat follows the symbolic links created by the atomic writer.
		info, err := os.Stat(path.Join(dir, name))
		g.Expect(err).ShouldNot(gomega.HaveOccurred(), name)
		g.Expect(info.Mode().Perm()).Should(gomega.Equal(mode), name)
	}

	read, err := (&fsCertWriter{FSCertWriterOptions: &FSCertWriterOptions{Path: dir}}).read()
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(read).Should(gomega.Equal(certs))
}
