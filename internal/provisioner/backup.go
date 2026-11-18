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
	"fmt"
	"strconv"
	"strings"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	"github.com/DataKnifeAI/terraria-operator/internal/worldcfg"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func backupLabels(server *terrariav1alpha1.TerrariaServer) map[string]string {
	return map[string]string{
		LabelAppName:      AppName,
		LabelAppInstance:  server.Name,
		LabelAppManagedBy: ManagedBy,
		LabelAppComponent: ComponentBackup,
	}
}

func backupSize(spec terrariav1alpha1.TerrariaServerSpec) string {
	if strings.TrimSpace(spec.Backup.Size) != "" {
		return spec.Backup.Size
	}
	return "4Gi"
}

func backupSchedule(spec terrariav1alpha1.TerrariaServerSpec) string {
	if strings.TrimSpace(spec.Backup.Schedule) != "" {
		return spec.Backup.Schedule
	}
	return "0 4 * * *"
}

func backupRetain(spec terrariav1alpha1.TerrariaServerSpec) int32 {
	if spec.Backup.Retain > 0 {
		return spec.Backup.Retain
	}
	return 7
}

func backupImage(spec terrariav1alpha1.TerrariaServerSpec) string {
	if strings.TrimSpace(spec.Backup.Image) != "" {
		return spec.Backup.Image
	}
	return "busybox:1.36"
}

func backupRESTSaveFirst(spec terrariav1alpha1.TerrariaServerSpec) bool {
	if spec.Backup.RESTSaveFirst == nil {
		return true
	}
	return *spec.Backup.RESTSaveFirst
}

// BuildBackupPVC creates a PVC that stores tar.gz archives of the worlds volume.
func BuildBackupPVC(server *terrariav1alpha1.TerrariaServer) *corev1.PersistentVolumeClaim {
	if !server.Spec.Backup.Enabled {
		return nil
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      BackupPVCName(server),
			Namespace: server.Namespace,
			Labels:    backupLabels(server),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(backupSize(server.Spec)),
				},
			},
		},
	}
	sc := server.Spec.Backup.StorageClassName
	if sc == "" {
		sc = server.Spec.Persistence.StorageClassName
	}
	if sc != "" {
		pvc.Spec.StorageClassName = &sc
	}
	return pvc
}

// BuildBackupCronJob creates a CronJob that optionally REST-saves then tars worlds.
func BuildBackupCronJob(server *terrariav1alpha1.TerrariaServer) *batchv1.CronJob {
	if !server.Spec.Backup.Enabled {
		return nil
	}
	retain := backupRetain(server.Spec)
	restPort := worldcfg.RESTPort(server.Spec)
	svc := ServiceName(server)
	ns := server.Namespace

	var saveBlock string
	if backupRESTSaveFirst(server.Spec) && worldcfg.AdminAPIEnabled(server.Spec) {
		saveBlock = fmt.Sprintf(`
if [ -n "${REST_TOKEN:-}" ]; then
  echo "requesting REST save"
  wget -q -O- "http://%s.%s.svc:%d/v2/server/save?token=${REST_TOKEN}" || true
  sleep 5
fi
`, svc, ns, restPort)
	}

	script := fmt.Sprintf(`set -euo pipefail
%s
STAMP=$(date -u +%%Y%%m%%dT%%H%%M%%SZ)
DEST=/backups/${STAMP}.tar.gz
echo "backing up /data -> ${DEST}"
tar -C /data -czf "${DEST}" .
echo "pruning to last %d archives"
ls -1t /backups/*.tar.gz 2>/dev/null | tail -n +%s | xargs -r rm -f
ls -lah /backups/
`, saveBlock, retain, strconv.FormatInt(int64(retain)+1, 10))

	labels := backupLabels(server)
	backoff := int32(1)
	ttl := int32(86400)
	successful := int32(3)
	failed := int32(1)
	concurrency := batchv1.ForbidConcurrent

	env := []corev1.EnvVar{}
	if backupRESTSaveFirst(server.Spec) && worldcfg.AdminAPIEnabled(server.Spec) {
		env = append(env, corev1.EnvVar{
			Name: "REST_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: TokenSecretName(server)},
					Key:                  SecretKeyToken,
					Optional:             ptrBool(true),
				},
			},
		})
	}

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      BackupCronJobName(server),
			Namespace: server.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   backupSchedule(server.Spec),
			ConcurrencyPolicy:          concurrency,
			SuccessfulJobsHistoryLimit: &successful,
			FailedJobsHistoryLimit:     &failed,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: batchv1.JobSpec{
					BackoffLimit:            &backoff,
					TTLSecondsAfterFinished: &ttl,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							NodeSelector:  server.Spec.NodeSelector,
							Containers: []corev1.Container{{
								Name:            "backup",
								Image:           backupImage(server.Spec),
								ImagePullPolicy: corev1.PullIfNotPresent,
								Command:         []string{"sh", "-c", script},
								Env:             env,
								VolumeMounts: []corev1.VolumeMount{
									{Name: volData, MountPath: "/data", ReadOnly: true},
									{Name: volBackups, MountPath: "/backups"},
								},
							}},
							Volumes: []corev1.Volume{
								{
									Name: volData,
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: PVCName(server),
											ReadOnly:  true,
										},
									},
								},
								{
									Name: volBackups,
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: BackupPVCName(server),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func ptrBool(v bool) *bool { return &v }
