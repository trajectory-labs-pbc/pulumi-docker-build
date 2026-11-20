// Copyright 2024, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"context"
	"fmt"
	"strings"

	csgen "github.com/pulumi/pulumi-dotnet/pulumi-language-dotnet/v3/codegen"
	provider "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	pschema "github.com/pulumi/pulumi-go-provider/middleware/schema"
	"github.com/pulumi/pulumi-java/pkg/codegen/java"
	gogen "github.com/pulumi/pulumi/pkg/v3/codegen/go"
	tsgen "github.com/pulumi/pulumi/pkg/v3/codegen/nodejs"
	pygen "github.com/pulumi/pulumi/pkg/v3/codegen/python"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

var (
	_ infer.CustomConfigure = (*Config)(nil)
	_ infer.Annotated       = (*Config)(nil)
	_ infer.Annotated       = (*Registry)(nil)
)

// Config configures the buildx provider.
type Config struct {
	Host       string     `pulumi:"host,optional"`
	Registries []Registry `pulumi:"registries,optional"`

	host *host
}

// Annotate provides user-facing descriptions and defaults for Config's fields.
func (c *Config) Annotate(a infer.Annotator) {
	a.Describe(&c.Host, "The build daemon's address.")
	a.SetDefault(&c.Host, "", "DOCKER_HOST")
}

// Configure validates and processes user-provided configuration values.
func (c *Config) Configure(ctx context.Context) error {
	h, err := newHost(ctx, c)
	if err != nil {
		return fmt.Errorf("getting host: %w", err)
	}
	c.host = h
	return nil
}

// GetRegistries returns the config's registries, if any.
func (c Config) GetRegistries() []Registry {
	return c.Registries
}

// getHost returns the config's host, or nil if the config is also nil.
func (c *Config) getHost() *host {
	if c == nil {
		return nil
	}
	return c.host
}

// NewBuildxProvider returns a new buildx provider.
func NewBuildxProvider(clientF clientF) provider.Provider {
	config := &Config{}

	prov := infer.Provider(
		infer.Options{
			Metadata: pschema.Metadata{
				DisplayName: "docker-build",
				Keywords:    []string{"docker", "buildkit", buildxName, "kind/native"},
				Description: "A Pulumi provider for building modern Docker images with buildx and BuildKit.",
				Homepage:    "https://pulumi.com",
				Publisher:   "Pulumi",
				License:     "Apache-2.0",
				Repository:  "https://github.com/pulumi/pulumi-docker-build",
				LanguageMap: map[string]any{
					"go": gogen.GoPackageInfo{
						// GenerateResourceContainerTypes: true,
						RespectSchemaVersion: true,
						Generics:             gogen.GenericsSettingSideBySide,
						PackageImportAliases: map[string]string{
							"github.com/pulumi/pulumi-docker-build/sdk/go/dockerbuild": "dockerbuild",
						},
						ImportBasePath: "github.com/pulumi/pulumi-docker-build/sdk/go/dockerbuild",
					},
					"csharp": csgen.CSharpPackageInfo{
						RespectSchemaVersion: true,
						PackageReferences: map[string]string{
							"Pulumi": "3.*",
						},
					},
					"java": java.PackageInfo{
						BuildFiles:                      "gradle",
						GradleNexusPublishPluginVersion: "1.1.0",
						Dependencies: map[string]string{
							"com.pulumi:pulumi":               "0.20.0",
							"com.google.code.gson:gson":       "2.8.9",
							"com.google.code.findbugs:jsr305": "3.0.2",
						},
					},
					"nodejs": tsgen.NodePackageInfo{
						RespectSchemaVersion: true,
					},
					"python": pygen.PackageInfo{
						RespectSchemaVersion: true,
						PyProject: struct {
							Enabled bool `json:"enabled,omitempty"`
						}{Enabled: true},
					},
				},
			},
			Resources: []infer.InferredResource{
				infer.Resource(&Image{clientF: clientF, config: config}),
				infer.Resource(&Index{clientF: clientF, config: config}),
			},
			ModuleMap: map[tokens.ModuleName]tokens.ModuleName{
				"internal": "index",
			},
			Config: infer.Config(config),
		},
	)

	// pulumi-go-provider's infer diff removes the engine's `version` input and every
	// `__`-prefixed key from the new config inputs, but not from the old ones, and it
	// replaces the provider on any config difference. A default provider upgrade
	// (`default_0_0_21` -> `default_0_0_23`) makes the engine diff the old provider's
	// inputs with the new provider's DiffConfig, so `__pulumi-go-provider-infer` and
	// `__pulumi-go-provider-version` in the old inputs read as removed and every Image
	// is replaced. A replaced Image deletes the original's digest from its registry,
	// which is the replacement's own digest when the content did not change. Remove
	// the same keys from the old inputs before the diff. See
	// pulumi/pulumi-go-provider#288 and pulumi/pulumi-docker-build#606.
	diffConfig := prov.DiffConfig
	prov.DiffConfig = func(ctx context.Context, req provider.DiffRequest) (provider.DiffResponse, error) {
		req.OldInputs = withoutEngineConfigKeys(req.OldInputs)
		return diffConfig(ctx, req)
	}

	return prov
}

// Schema returns our package specification.
func Schema(ctx context.Context, version string) schema.PackageSpec {
	p := NewBuildxProvider(nil)
	spec, err := provider.GetSchema(ctx, "docker-build", version, p)
	contract.AssertNoErrorf(err, "missing schema")
	return spec
}

// withoutEngineConfigKeys returns m without the keys infer's diff removes from new
// config inputs: `version` and every `__`-prefixed key.
func withoutEngineConfigKeys(m property.Map) property.Map {
	for k := range m.All {
		if k == "version" || strings.HasPrefix(k, "__") {
			m = m.Delete(k)
		}
	}
	return m
}
