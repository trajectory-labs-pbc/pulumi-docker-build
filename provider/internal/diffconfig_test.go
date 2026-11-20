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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	provider "github.com/pulumi/pulumi-go-provider"
	rpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

// diffConfig calls DiffConfig over gRPC the way the engine does: after a Configure
// that says the engine sends old inputs, so the diff reads req.OldInputs raw.
func diffConfig(t *testing.T, olds, news map[string]any) *rpc.DiffResponse {
	t.Helper()

	p := NewBuildxProvider(nil)
	p.Configure = func(context.Context, provider.ConfigureRequest) error { return nil }
	s, err := provider.RawServer("docker-build", "0.0.23", p)(nil)
	require.NoError(t, err)
	_, err = s.Configure(t.Context(), &rpc.ConfigureRequest{SendsOldInputs: true, SendsOldInputsToDelete: true})
	require.NoError(t, err)

	o, err := structpb.NewStruct(olds)
	require.NoError(t, err)
	n, err := structpb.NewStruct(news)
	require.NoError(t, err)
	resp, err := s.DiffConfig(t.Context(), &rpc.DiffRequest{
		Urn:       "urn:pulumi:s::p::pulumi:providers:docker-build::default_0_0_23",
		Olds:      o,
		OldInputs: o,
		News:      n,
	})
	require.NoError(t, err)
	return resp
}

// The engine diffs a default provider upgrade (default_0_0_21 -> default_0_0_23)
// with the new provider's DiffConfig, passing the old provider's inputs as they
// are in state. Those carry the engine's `version` and the framework's
// `__pulumi-go-provider-*` keys; a replace here replaces every Image.
func TestDiffConfigProviderUpgradeIsNotAReplace(t *testing.T) {
	t.Parallel()

	resp := diffConfig(t,
		map[string]any{
			"__pulumi-go-provider-infer":   true,
			"__pulumi-go-provider-version": "v1.3.2",
			"host":                         "",
			"version":                      "0.0.21",
		},
		map[string]any{
			"__pulumi-go-provider-infer":   true,
			"__pulumi-go-provider-version": "v1.7.0",
			"version":                      "0.0.23",
		},
	)
	assert.Empty(t, resp.GetReplaces())
	assert.Equal(t, rpc.PropertyDiff_UPDATE, resp.GetDetailedDiff()["version"].GetKind())
}

// A real config change still replaces the provider.
func TestDiffConfigHostChangeIsAReplace(t *testing.T) {
	t.Parallel()

	resp := diffConfig(t,
		map[string]any{
			"__pulumi-go-provider-infer":   true,
			"__pulumi-go-provider-version": "v1.7.0",
			"host":                         "",
			"version":                      "0.0.23",
		},
		map[string]any{
			"__pulumi-go-provider-infer":   true,
			"__pulumi-go-provider-version": "v1.7.0",
			"host":                         "tcp://127.0.0.1:2375",
			"version":                      "0.0.23",
		},
	)
	assert.Equal(t, []string{"host"}, resp.GetReplaces())
}
