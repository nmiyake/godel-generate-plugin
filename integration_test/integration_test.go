// Copyright (c) 2018 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package integration_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/nmiyake/pkg/dirs"
	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/godel/v2/framework/pluginapitester"
	"github.com/palantir/godel/v2/pkg/products"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateVerify(t *testing.T) {
	restoreEnvVars := setEnvVars(map[string]string{
		"GO111MODULE": "",
		"GOFLAGS":     "",
	})
	defer restoreEnvVars()

	pluginPath, err := products.Bin("generate-plugin")
	require.NoError(t, err)

	projectDir, cleanup, err := dirs.TempDir("", "")
	require.NoError(t, err)
	defer cleanup()

	const generateYML = `
generators:
  foo:
    go-generate-dir: gen
    gen-paths:
      paths:
        - "gen/output.txt"
`
	err = os.MkdirAll(path.Join(projectDir, "godel", "config"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(path.Join(projectDir, "godel", "config", "generate-plugin.yml"), []byte(generateYML), 0644)
	require.NoError(t, err)

	specs := []gofiles.GoFileSpec{
		{
			RelPath: "gen/go.mod",
			Src: `module github.com/palantir/godel-generate-plugin/integration_test
`,
		},
		{
			RelPath: "gen/testbar.go",
			Src: `package testbar

//go:generate go run generator_main.go
`,
		},
		{
			RelPath: "gen/generator_main.go",
			Src: `// +build ignore

package main

import (
	"io/ioutil"
)

func main() {
	if err := ioutil.WriteFile("output.txt", []byte("foo-output"), 0644); err != nil {
		panic(err)
	}
}
`,
		},
	}

	_, err = gofiles.Write(projectDir, specs)
	require.NoError(t, err)

	err = ioutil.WriteFile(path.Join(projectDir, "gen", "output.txt"), []byte("original"), 0644)
	require.NoError(t, err)

	outputBuf := &bytes.Buffer{}
	runPluginCleanup, err := pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "generate", []string{
		"--verify",
	}, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.EqualError(t, err, "")

	want := "Generators produced output that differed from what already exists: [foo]\n  foo:\n    gen/output.txt: previously had checksum 0682c5f2076f099c34cfdd15a9e063849ed437a49677e6fcc5b4198c76575be5, now has checksum 380a300b764683667309818ff127a401c6ea6ab1959f386fe0f05505d660ba37\n"
	assert.Equal(t, want, outputBuf.String())
}

func TestUpgradeConfig(t *testing.T) {
	pluginPath, err := products.Bin("generate-plugin")
	require.NoError(t, err)
	pluginProvider := pluginapitester.NewPluginProvider(pluginPath)

	pluginapitester.RunUpgradeConfigTest(t,
		pluginProvider,
		nil,
		[]pluginapitester.UpgradeConfigTestCase{
			{
				Name: "legacy config is upgraded",
				ConfigFiles: map[string]string{
					"godel/config/generate.yml": `
generators:
  foo:
    go-generate-dir: gen
    gen-paths:
      paths:
        - "gen/output.txt"
    environment:
      # comment on environment variable
      GOOS: linux
`,
				},
				Legacy:     true,
				WantOutput: "Upgraded configuration for generate-plugin.yml\n",
				WantFiles: map[string]string{
					"godel/config/generate-plugin.yml": `generators:
  foo:
    go-generate-dir: gen
    gen-paths:
      paths:
      - gen/output.txt
    environment:
      GOOS: linux
`,
				},
			},
			{
				Name: "legacy config upgrade omits empty fields",
				ConfigFiles: map[string]string{
					"godel/config/generate.yml": `
generators:
  foo:
    go-generate-dir: gen
    gen-paths:
      paths:
        - "gen/output.txt"
`,
				},
				Legacy:     true,
				WantOutput: "Upgraded configuration for generate-plugin.yml\n",
				WantFiles: map[string]string{
					"godel/config/generate-plugin.yml": `generators:
  foo:
    go-generate-dir: gen
    gen-paths:
      paths:
      - gen/output.txt
`,
				},
			},
			{
				Name: "current config is unmodified",
				ConfigFiles: map[string]string{
					"godel/config/generate-plugin.yml": `
generators:
  foo:
    go-generate-dir: gen
    gen-paths:
      paths:
        - "gen/output.txt"
    environment:
      # comment on environment variable
      GOOS: linux
`,
				},
				WantOutput: "",
				WantFiles: map[string]string{
					"godel/config/generate-plugin.yml": `
generators:
  foo:
    go-generate-dir: gen
    gen-paths:
      paths:
        - "gen/output.txt"
    environment:
      # comment on environment variable
      GOOS: linux
`,
				},
			},
		},
	)
}

func setEnvVars(envVars map[string]string) func() {
	origVars := make(map[string]string)
	var unsetVars []string
	for k := range envVars {
		val, ok := os.LookupEnv(k)
		if !ok {
			unsetVars = append(unsetVars, k)
			continue
		}
		origVars[k] = val
	}

	for k, v := range envVars {
		_ = os.Setenv(k, v)
	}

	return func() {
		for _, k := range unsetVars {
			_ = os.Unsetenv(k)
		}
		for k, v := range origVars {
			_ = os.Setenv(k, v)
		}
	}
}
