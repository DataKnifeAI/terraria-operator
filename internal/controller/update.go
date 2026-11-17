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
	"time"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	"github.com/DataKnifeAI/terraria-operator/internal/tshock"
	"github.com/DataKnifeAI/terraria-operator/internal/worldcfg"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type updateResult struct {
	LatestTShockRelease         string
	LatestTShockTerrariaVersion string
	InstalledTerrariaVersion    string
	UpdateAvailable             bool
}

// checkUpdates polls TShock GitHub releases and surfaces Terraria compat in status.
// Phase 1 is status-only. Phase 2 may apply autoUpdateImage retags.
func (r *TerrariaServerReconciler) checkUpdates(ctx context.Context, server *terrariav1alpha1.TerrariaServer) (updateResult, time.Duration) {
	log := logf.FromContext(ctx)
	interval := CheckInterval(server.Spec)

	if server.Status.LastUpdateCheck != nil {
		elapsed := time.Since(server.Status.LastUpdateCheck.Time)
		if elapsed < interval && server.Status.LatestTShockRelease != "" {
			return updateResult{
				LatestTShockRelease:         server.Status.LatestTShockRelease,
				LatestTShockTerrariaVersion: server.Status.LatestTShockTerrariaVersion,
				InstalledTerrariaVersion:    server.Status.InstalledTerrariaVersion,
				UpdateAvailable:             server.Status.UpdateAvailable,
			}, interval - elapsed
		}
	}

	client := r.Versions
	if client == nil {
		client = tshock.DefaultVersionClient()
	}

	info, err := client.LatestStable(ctx)
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

	res := updateResult{
		LatestTShockRelease:         info.TShockVersion,
		LatestTShockTerrariaVersion: info.TerrariaVersion,
		InstalledTerrariaVersion:    installed,
		UpdateAvailable:             updateAvail,
	}

	// Phase 1: never mutate image. Phase 2: honor autoUpdateImage.
	if updateAvail && worldcfg.AutoUpdateImage(server.Spec) {
		log.Info("autoUpdateImage requested but Phase 1 is status-only; skipping apply",
			"latestTShock", info.TShockVersion, "latestTerraria", info.TerrariaVersion)
	}

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
