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

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	"github.com/DataKnifeAI/terraria-operator/internal/tshock"
	"github.com/DataKnifeAI/terraria-operator/internal/worldcfg"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type updateResult struct {
	LatestTShockRelease         string
	LatestTShockTerrariaVersion string
	InstalledTerrariaVersion    string
	UpdateAvailable             bool
	Applied                     bool
	Message                     string
}

// checkUpdates polls TShock GitHub releases and optionally applies autoUpdateImage.
func (r *TerrariaServerReconciler) checkUpdates(ctx context.Context, server *terrariav1alpha1.TerrariaServer) (updateResult, time.Duration) {
	log := logf.FromContext(ctx)
	interval := CheckInterval(server.Spec)

	if server.Status.LastUpdateCheck != nil {
		elapsed := time.Since(server.Status.LastUpdateCheck.Time)
		if elapsed < interval && server.Status.LatestTShockRelease != "" && !worldcfg.AutoUpdateImage(server.Spec) {
			return updateResult{
				LatestTShockRelease:         server.Status.LatestTShockRelease,
				LatestTShockTerrariaVersion: server.Status.LatestTShockTerrariaVersion,
				InstalledTerrariaVersion:    server.Status.InstalledTerrariaVersion,
				UpdateAvailable:             server.Status.UpdateAvailable,
			}, interval - elapsed
		}
	}

	verClient := r.Versions
	if verClient == nil {
		verClient = tshock.DefaultVersionClient()
	}

	info, err := verClient.LatestStable(ctx)
	if err != nil {
		log.Info("tshock release check failed", "error", err)
		return updateResult{
			LatestTShockRelease:         server.Status.LatestTShockRelease,
			LatestTShockTerrariaVersion: server.Status.LatestTShockTerrariaVersion,
			InstalledTerrariaVersion:    server.Status.InstalledTerrariaVersion,
			UpdateAvailable:             server.Status.UpdateAvailable,
		}, interval
	}

	if info.TerrariaVersion == "" {
		log.Info("could not parse Terraria version from TShock release notes; leaving status unset",
			"tag", info.TagName)
	}

	installed := server.Status.InstalledTerrariaVersion
	updateAvail := false
	if installed != "" && info.TerrariaVersion != "" {
		updateAvail = tshock.CompareVersions(installed, info.TerrariaVersion) < 0
	}
	// Also treat a newer TShock release tag as available when running STABLE floating tag.
	mode := strings.ToUpper(strings.TrimSpace(server.Spec.Version.Mode))
	if mode == "" {
		mode = terrariav1alpha1.VersionModeStable
	}
	pinned := strings.TrimPrefix(strings.TrimSpace(server.Spec.Version.PinnedTag), "v")
	if mode == terrariav1alpha1.VersionModePinned && pinned != "" && info.TShockVersion != "" {
		if tshock.CompareVersions(pinned, info.TShockVersion) < 0 {
			updateAvail = true
		}
	}

	res := updateResult{
		LatestTShockRelease:         info.TShockVersion,
		LatestTShockTerrariaVersion: info.TerrariaVersion,
		InstalledTerrariaVersion:    installed,
		UpdateAvailable:             updateAvail,
	}

	if !updateAvail || !worldcfg.AutoUpdateImage(server.Spec) {
		return res, interval
	}

	// Skip if custom image fully overrides repository tagging.
	if img := strings.TrimSpace(server.Spec.ServerImage.Image); img != "" && !strings.Contains(img, "pryaxis/tshock") {
		res.Message = "autoUpdateImage skipped: custom serverImage is not pryaxis/tshock"
		return res, interval
	}

	targetTag := info.TagName
	if targetTag == "" {
		targetTag = "v" + info.TShockVersion
	}
	if !strings.HasPrefix(targetTag, "v") {
		targetTag = "v" + targetTag
	}
	currentPinned := strings.TrimSpace(server.Spec.Version.PinnedTag)
	if mode == terrariav1alpha1.VersionModePinned && currentPinned == targetTag {
		res.UpdateAvailable = false
		return res, interval
	}

	patched := server.DeepCopy()
	patched.Spec.Version.Mode = terrariav1alpha1.VersionModePinned
	patched.Spec.Version.PinnedTag = targetTag
	// Clear floating image override so ImageRef uses repo:tag.
	if patched.Spec.ServerImage.Image == terrariav1alpha1.DefaultImage ||
		patched.Spec.ServerImage.Image == "ghcr.io/pryaxis/tshock:stable" {
		patched.Spec.ServerImage.Image = ""
	}
	if err := r.Patch(ctx, patched, client.MergeFrom(server)); err != nil {
		log.Info("failed to apply autoUpdateImage", "error", err)
		res.Message = fmt.Sprintf("autoUpdateImage patch failed: %v", err)
		return res, interval
	}
	log.Info("auto-updated version pin", "tag", targetTag, "terraria", info.TerrariaVersion)
	res.Applied = true
	res.UpdateAvailable = false
	res.Message = fmt.Sprintf("auto-updated to %s", targetTag)
	return res, interval
}

// CheckInterval parses update.checkInterval (default 6h).
func CheckInterval(spec terrariav1alpha1.TerrariaServerSpec) time.Duration {
	s := spec.Update.CheckInterval
	if s == "" {
		s = terrariav1alpha1.DefaultUpdateInterval
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		d = 6 * time.Hour
	}
	return d
}
