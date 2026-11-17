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

package tshock

import "testing"

func TestParseTerrariaVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"TShock 6.1.0 for Terraria 1.4.5.6", "1.4.5.6"},
		{"## Notes\nBuilt against Terraria 1.4.5.8\n", "1.4.5.8"},
		{"Supports Terraria version: 1.4.4.9", "1.4.4.9"},
		{"no mention of the game here", ""},
		{"Terraria 1.4.5", "1.4.5"},
	}
	for _, tc := range cases {
		if got := ParseTerrariaVersion(tc.in); got != tc.want {
			t.Fatalf("ParseTerrariaVersion(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	if CompareVersions("1.4.5.6", "1.4.5.8") >= 0 {
		t.Fatal("expected 1.4.5.6 < 1.4.5.8")
	}
	if CompareVersions("1.4.5.8", "1.4.5.8") != 0 {
		t.Fatal("expected equal")
	}
	if CompareVersions("6.2.0", "6.1.0") <= 0 {
		t.Fatal("expected 6.2.0 > 6.1.0")
	}
}
