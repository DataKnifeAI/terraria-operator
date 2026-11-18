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

package provisioner

import (
	"strings"
	"testing"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildBackupCronJob(t *testing.T) {
	server := &terrariav1alpha1.TerrariaServer{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: "games"},
		Spec: terrariav1alpha1.TerrariaServerSpec{
			Backup: terrariav1alpha1.BackupConfig{
				Enabled:  true,
				Schedule: "0 5 * * *",
				Retain:   3,
			},
		},
	}
	cj := BuildBackupCronJob(server)
	if cj == nil {
		t.Fatal("expected CronJob")
	}
	if cj.Name != "main-backup" || cj.Spec.Schedule != "0 5 * * *" {
		t.Fatalf("unexpected cronjob meta/schedule: %s %s", cj.Name, cj.Spec.Schedule)
	}
	script := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command[2]
	if !strings.Contains(script, "tar -C /data") {
		t.Fatalf("missing tar in script: %s", script)
	}
	if !strings.Contains(script, "v2/server/save") {
		t.Fatalf("expected REST save in script: %s", script)
	}
}

func TestBuildPluginsPVC(t *testing.T) {
	server := &terrariav1alpha1.TerrariaServer{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: "game"},
		Spec:       terrariav1alpha1.TerrariaServerSpec{Plugins: terrariav1alpha1.PluginsConfig{Enabled: true}},
	}
	pvc := BuildPluginsPVC(server)
	if pvc == nil || pvc.Name != "main-plugins" {
		t.Fatalf("unexpected pvc %#v", pvc)
	}
}
