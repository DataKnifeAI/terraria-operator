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
	"strings"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func pluginsLabels(server *terrariav1alpha1.TerrariaServer) map[string]string {
	return map[string]string{
		LabelAppName:      AppName,
		LabelAppInstance:  server.Name,
		LabelAppManagedBy: ManagedBy,
		LabelAppComponent: ComponentPlugins,
	}
}

func pluginsSize(spec terrariav1alpha1.TerrariaServerSpec) string {
	if strings.TrimSpace(spec.Plugins.Size) != "" {
		return spec.Plugins.Size
	}
	return "1Gi"
}

// PluginsPath returns the container mount for ServerPlugins/.
func PluginsPath(spec terrariav1alpha1.TerrariaServerSpec) string {
	if p := strings.TrimSpace(spec.Plugins.Path); p != "" {
		return p
	}
	return terrariav1alpha1.PluginsMountPath
}

// BuildPluginsPVC creates an owned PVC for plugin DLLs when enabled and no mountRef.
func BuildPluginsPVC(server *terrariav1alpha1.TerrariaServer) *corev1.PersistentVolumeClaim {
	if !server.Spec.Plugins.Enabled {
		return nil
	}
	if server.Spec.Plugins.MountRef != nil && server.Spec.Plugins.MountRef.Name != "" {
		return nil
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PluginsPVCName(server),
			Namespace: server.Namespace,
			Labels:    pluginsLabels(server),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(pluginsSize(server.Spec)),
				},
			},
		},
	}
	if server.Spec.Persistence.StorageClassName != "" {
		sc := server.Spec.Persistence.StorageClassName
		pvc.Spec.StorageClassName = &sc
	}
	return pvc
}

// PluginFetchInitContainer downloads plugin URLs into the plugins volume.
func PluginFetchInitContainer(server *terrariav1alpha1.TerrariaServer) *corev1.Container {
	if !server.Spec.Plugins.Enabled || len(server.Spec.Plugins.URLs) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("set -euo pipefail\napk add --no-cache curl unzip >/dev/null\nmkdir -p /plugins\n")
	for i, u := range server.Spec.Plugins.URLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		lower := strings.ToLower(u)
		dest := fmt.Sprintf("/plugins/plugin-%d", i)
		fmt.Fprintf(&b, "echo 'fetching %s'\ncurl -fsSL -o %s %q\n", u, dest, u)
		if strings.HasSuffix(lower, ".zip") {
			fmt.Fprintf(&b, "unzip -o %s -d /plugins\nrm -f %s\n", dest, dest)
		} else {
			fmt.Fprintf(&b, "mv %s %s.dll\n", dest, dest)
		}
	}
	b.WriteString("ls -lah /plugins\n")
	return &corev1.Container{
		Name:            "fetch-plugins",
		Image:           "alpine:3.20",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"sh", "-c", b.String()},
		VolumeMounts: []corev1.VolumeMount{
			{Name: ComponentPlugins, MountPath: "/plugins"},
		},
	}
}
