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

// Package worldcfg maps TerrariaServer world settings to server command-line args.
package worldcfg

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// ArgsFromSpec builds Terraria dedicated-server args from the CR spec.
// Password is not embedded here — callers inject -pass via env/secret when present.
func ArgsFromSpec(spec terrariav1alpha1.TerrariaServerSpec, password string) ([]string, error) {
	if err := ValidateSpec(spec); err != nil {
		return nil, err
	}

	worldName := strings.TrimSpace(spec.World.Name)
	if worldName == "" {
		worldName = "main"
	}
	size := autocreate(spec.World.Size)
	diff := difficulty(spec.World.Difficulty)
	maxPlayers := spec.World.MaxPlayers
	if maxPlayers == 0 {
		maxPlayers = terrariav1alpha1.DefaultMaxPlayers
	}

	worldPath := filepath.Join(terrariav1alpha1.WorldsMountPath, worldName+".wld")
	args := []string{
		"-world", worldPath,
		"-autocreate", strconv.Itoa(size),
		"-worldname", worldName,
		"-maxplayers", strconv.FormatInt(int64(maxPlayers), 10),
		"-difficulty", strconv.Itoa(diff),
		"-port", strconv.FormatInt(int64(GamePort()), 10),
	}
	if seed := strings.TrimSpace(spec.World.Seed); seed != "" {
		args = append(args, "-seed", seed)
	}
	if password != "" {
		args = append(args, "-pass", password)
	}
	return args, nil
}

func autocreate(size string) int {
	switch strings.ToUpper(strings.TrimSpace(size)) {
	case terrariav1alpha1.WorldSizeSmall:
		return 1
	case terrariav1alpha1.WorldSizeLarge:
		return 3
	default:
		return 2
	}
}

func difficulty(d string) int {
	switch strings.ToUpper(strings.TrimSpace(d)) {
	case terrariav1alpha1.DifficultyExpert:
		return 1
	case terrariav1alpha1.DifficultyMaster:
		return 2
	case terrariav1alpha1.DifficultyJourney:
		return 3
	default:
		return 0
	}
}

// GamePort returns the Terraria game TCP port.
func GamePort() int32 {
	return terrariav1alpha1.DefaultGamePort
}

// RESTPort returns the effective TShock REST port.
func RESTPort(spec terrariav1alpha1.TerrariaServerSpec) int32 {
	if spec.AdminAPI.Port > 0 {
		return spec.AdminAPI.Port
	}
	return terrariav1alpha1.DefaultRESTPort
}

// ImageRef returns the container image reference including tag selection.
func ImageRef(spec terrariav1alpha1.TerrariaServerSpec) string {
	if img := strings.TrimSpace(spec.ServerImage.Image); img != "" {
		return img
	}
	serverType := strings.ToUpper(strings.TrimSpace(spec.ServerType))
	if serverType == terrariav1alpha1.ServerTypeVanilla {
		return terrariav1alpha1.DefaultVanillaImage
	}

	repo := "ghcr.io/pryaxis/tshock"
	mode := strings.ToUpper(strings.TrimSpace(spec.Version.Mode))
	if mode == terrariav1alpha1.VersionModePinned {
		tag := strings.TrimSpace(spec.Version.PinnedTag)
		if tag == "" {
			tag = "stable"
		}
		return repo + ":" + tag
	}
	return terrariav1alpha1.DefaultImage
}

// PersistenceSize returns the PVC size.
func PersistenceSize(spec terrariav1alpha1.TerrariaServerSpec) string {
	if strings.TrimSpace(spec.Persistence.Size) != "" {
		return spec.Persistence.Size
	}
	return terrariav1alpha1.DefaultPersistenceSize
}

// GraceSeconds returns terminationGracePeriodSeconds.
func GraceSeconds(spec terrariav1alpha1.TerrariaServerSpec) int64 {
	if spec.TerminationGracePeriodSeconds != nil {
		return *spec.TerminationGracePeriodSeconds
	}
	return terrariav1alpha1.DefaultGraceSeconds
}

// AdminAPIEnabled reports whether TShock REST should be enabled.
func AdminAPIEnabled(spec terrariav1alpha1.TerrariaServerSpec) bool {
	if strings.ToUpper(strings.TrimSpace(spec.ServerType)) == terrariav1alpha1.ServerTypeVanilla {
		return false
	}
	if spec.AdminAPI.Enabled == nil {
		return true
	}
	return *spec.AdminAPI.Enabled
}

// GatewayEnabled reports whether Envoy Gateway resources should be reconciled.
func GatewayEnabled(spec terrariav1alpha1.TerrariaServerSpec) bool {
	if spec.Gateway.Enabled == nil {
		return true
	}
	return *spec.Gateway.Enabled
}

// AutoUpdateImage reports whether opt-in image retag is enabled (Phase 2 apply).
func AutoUpdateImage(spec terrariav1alpha1.TerrariaServerSpec) bool {
	return spec.Update.AutoUpdateImage
}

// ValidateSpec returns a user-facing error if the CR cannot be reconciled.
func ValidateSpec(spec terrariav1alpha1.TerrariaServerSpec) error {
	serverType := strings.ToUpper(strings.TrimSpace(spec.ServerType))
	if serverType == "" {
		serverType = terrariav1alpha1.ServerTypeTShock
	}
	switch serverType {
	case terrariav1alpha1.ServerTypeTShock, terrariav1alpha1.ServerTypeVanilla:
	default:
		return fmt.Errorf("spec.serverType %q is not supported", spec.ServerType)
	}

	mode := strings.ToUpper(strings.TrimSpace(spec.Version.Mode))
	if mode == "" {
		mode = terrariav1alpha1.VersionModeStable
	}
	switch mode {
	case terrariav1alpha1.VersionModeStable:
	case terrariav1alpha1.VersionModePinned:
		if strings.TrimSpace(spec.Version.PinnedTag) == "" && strings.TrimSpace(spec.ServerImage.Image) == "" {
			return fmt.Errorf("spec.version.pinnedTag is required when mode=PINNED")
		}
	default:
		return fmt.Errorf("spec.version.mode %q is not supported", spec.Version.Mode)
	}

	if GatewayEnabled(spec) && strings.TrimSpace(spec.Gateway.Address) == "" {
		return fmt.Errorf("spec.gateway.address is required when gateway is enabled")
	}
	if !GatewayEnabled(spec) && spec.Gateway.ServiceType == corev1.ServiceTypeLoadBalancer && strings.TrimSpace(spec.Gateway.Address) == "" {
		return fmt.Errorf("spec.gateway.address is required when serviceType is LoadBalancer")
	}
	if interval := strings.TrimSpace(spec.Update.CheckInterval); interval != "" {
		if _, err := time.ParseDuration(interval); err != nil {
			return fmt.Errorf("spec.update.checkInterval: %w", err)
		}
	}
	return nil
}
