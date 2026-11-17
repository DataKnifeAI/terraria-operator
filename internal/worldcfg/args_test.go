/*
Copyright 2026 DataKnifeAI.

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

package worldcfg

import (
	"testing"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
)

func TestArgsFromSpecSizesAndDifficulties(t *testing.T) {
	cases := []struct {
		name       string
		size       string
		difficulty string
		wantAuto   string
		wantDiff   string
	}{
		{"small-classic", "SMALL", "CLASSIC", "1", "0"},
		{"medium-expert", "MEDIUM", "EXPERT", "2", "1"},
		{"large-master", "LARGE", "MASTER", "3", "2"},
		{"medium-journey", "MEDIUM", "JOURNEY", "2", "3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := false
			args, err := ArgsFromSpec(terrariav1alpha1.TerrariaServerSpec{
				World: terrariav1alpha1.WorldConfig{
					Name:       "main",
					Size:       tc.size,
					Difficulty: tc.difficulty,
					MaxPlayers: 8,
				},
				Gateway: terrariav1alpha1.GatewayConfig{Enabled: &gw},
			}, "")
			if err != nil {
				t.Fatalf("ArgsFromSpec: %v", err)
			}
			got := map[string]string{}
			for i := 0; i+1 < len(args); i += 2 {
				got[args[i]] = args[i+1]
			}
			if got["-autocreate"] != tc.wantAuto {
				t.Fatalf("autocreate=%s want %s", got["-autocreate"], tc.wantAuto)
			}
			if got["-difficulty"] != tc.wantDiff {
				t.Fatalf("difficulty=%s want %s", got["-difficulty"], tc.wantDiff)
			}
			if got["-maxplayers"] != "8" {
				t.Fatalf("maxplayers=%s", got["-maxplayers"])
			}
		})
	}
}

func TestImageRefPinnedAndVanilla(t *testing.T) {
	if got := ImageRef(terrariav1alpha1.TerrariaServerSpec{}); got != terrariav1alpha1.DefaultImage {
		t.Fatalf("default image=%s", got)
	}
	if got := ImageRef(terrariav1alpha1.TerrariaServerSpec{
		Version: terrariav1alpha1.VersionConfig{Mode: "PINNED", PinnedTag: "v6.1.0"},
	}); got != "ghcr.io/pryaxis/tshock:v6.1.0" {
		t.Fatalf("pinned=%s", got)
	}
	if got := ImageRef(terrariav1alpha1.TerrariaServerSpec{ServerType: "VANILLA"}); got != terrariav1alpha1.DefaultVanillaImage {
		t.Fatalf("vanilla=%s", got)
	}
}
