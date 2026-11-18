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
	"crypto/rand"
	"encoding/hex"
	"strconv"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	"github.com/DataKnifeAI/terraria-operator/internal/worldcfg"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const (
	dataMountPath = terrariav1alpha1.WorldsMountPath
	volData       = "data"
	volPlugins    = ComponentPlugins
	volBackups    = "backups"
	portNameGame  = "game"
	portNameREST  = "rest"
)

// BuildPVC creates the worlds / tshock data volume.
// When ExistingClaim is set, returns nil — caller adopts the named claim.
func BuildPVC(server *terrariav1alpha1.TerrariaServer) *corev1.PersistentVolumeClaim {
	if server.Spec.Persistence.ExistingClaim != "" {
		return nil
	}
	size := worldcfg.PersistenceSize(server.Spec)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PVCName(server),
			Namespace: server.Namespace,
			Labels:    LabelsFor(server),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
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

// BuildService exposes game TCP 7777 and optional REST 7878 (ClusterIP-only for REST).
func BuildService(server *terrariav1alpha1.TerrariaServer) *corev1.Service {
	port := worldcfg.GamePort()
	svcType := corev1.ServiceTypeClusterIP
	if !worldcfg.GatewayEnabled(server.Spec) && server.Spec.Gateway.ServiceType != "" {
		svcType = server.Spec.Gateway.ServiceType
	}

	ports := []corev1.ServicePort{
		{
			Name:       portNameGame,
			Protocol:   corev1.ProtocolTCP,
			Port:       port,
			TargetPort: intstr.FromInt32(port),
		},
	}
	if worldcfg.AdminAPIEnabled(server.Spec) {
		rest := worldcfg.RESTPort(server.Spec)
		ports = append(ports, corev1.ServicePort{
			Name:       portNameREST,
			Protocol:   corev1.ProtocolTCP,
			Port:       rest,
			TargetPort: intstr.FromInt32(rest),
		})
	}

	spec := corev1.ServiceSpec{
		Type:     svcType,
		Selector: LabelsFor(server),
		Ports:    ports,
	}
	if !worldcfg.GatewayEnabled(server.Spec) && svcType == corev1.ServiceTypeLoadBalancer {
		if addr := server.Spec.Gateway.Address; addr != "" {
			spec.LoadBalancerIP = addr
		}
		spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyCluster
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName(server),
			Namespace: server.Namespace,
			Labels:    LabelsFor(server),
		},
		Spec: spec,
	}
}

// BuildRESTBootstrapSecret creates an owned Secret holding a RestManagerPassword
// used for one-shot token create via /v2/token/create/{password}.
func BuildRESTBootstrapSecret(server *terrariav1alpha1.TerrariaServer, existing *corev1.Secret) *corev1.Secret {
	if !worldcfg.AdminAPIEnabled(server.Spec) {
		return nil
	}
	if server.Spec.AdminAPI.TokenSecretRef != nil && server.Spec.AdminAPI.TokenSecretRef.Name != "" {
		return nil
	}
	data := map[string][]byte{}
	if existing != nil && existing.Data != nil {
		for k, v := range existing.Data {
			data[k] = v
		}
	}
	if len(data[SecretKeyRestPassword]) == 0 {
		data[SecretKeyRestPassword] = []byte(randomHex(16))
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TokenSecretName(server),
			Namespace: server.Namespace,
			Labels:    LabelsFor(server),
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}

// BuildStatefulSet creates the single-replica StatefulSet for Terraria/TShock.
func BuildStatefulSet(server *terrariav1alpha1.TerrariaServer, args []string) *appsv1.StatefulSet {
	replicas := int32(1)
	grace := worldcfg.GraceSeconds(server.Spec)
	labels := LabelsFor(server)
	image := worldcfg.ImageRef(server.Spec)
	pullPolicy := server.Spec.ServerImage.PullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}
	port := worldcfg.GamePort()
	resources := defaultResources(server)

	containerPorts := []corev1.ContainerPort{
		{Name: portNameGame, ContainerPort: port, Protocol: corev1.ProtocolTCP},
	}
	if worldcfg.AdminAPIEnabled(server.Spec) {
		rest := worldcfg.RESTPort(server.Spec)
		containerPorts = append(containerPorts, corev1.ContainerPort{
			Name: portNameREST, ContainerPort: rest, Protocol: corev1.ProtocolTCP,
		})
	}

	readiness := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(port)},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       15,
		TimeoutSeconds:      5,
		FailureThreshold:    8,
	}
	if worldcfg.AdminAPIEnabled(server.Spec) {
		rest := worldcfg.RESTPort(server.Spec)
		readiness = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/status",
					Port: intstr.FromInt32(rest),
				},
			},
			InitialDelaySeconds: 45,
			PeriodSeconds:       20,
			TimeoutSeconds:      5,
			FailureThreshold:    8,
		}
	}
	liveness := readiness.DeepCopy()

	env := []corev1.EnvVar{}
	if worldcfg.AdminAPIEnabled(server.Spec) {
		env = append(env,
			corev1.EnvVar{Name: "TSHOCK_REST_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "TSHOCK_REST_PORT", Value: strconv.FormatInt(int64(worldcfg.RESTPort(server.Spec)), 10)},
			corev1.EnvVar{
				Name: "TSHOCK_REST_TOKEN",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: TokenSecretName(server)},
						Key:                  SecretKeyToken,
						Optional:             ptr.To(true),
					},
				},
			},
		)
	}

	volumeMounts := []corev1.VolumeMount{
		{Name: volData, MountPath: dataMountPath},
	}
	volumes := []corev1.Volume{{
		Name: volData,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: PVCName(server),
			},
		},
	}}
	var initContainers []corev1.Container
	if server.Spec.Plugins.Enabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name: volPlugins, MountPath: PluginsPath(server.Spec),
		})
		volumes = append(volumes, corev1.Volume{
			Name: volPlugins,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: PluginsPVCName(server),
				},
			},
		})
		if init := PluginFetchInitContainer(server); init != nil {
			initContainers = append(initContainers, *init)
		}
	}

	// Graceful stop: REST save when token is present, then brief wait for flush.
	lifecycle := &corev1.Lifecycle{
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/sh", "-c", gracefulStopScript(server)},
			},
		},
	}

	podSpec := corev1.PodSpec{
		TerminationGracePeriodSeconds: &grace,
		ImagePullSecrets:              server.Spec.ImagePullSecrets,
		NodeSelector:                  server.Spec.NodeSelector,
		InitContainers:                initContainers,
		SecurityContext: &corev1.PodSecurityContext{
			FSGroup: ptr.To(int64(0)),
		},
		Containers: []corev1.Container{{
			Name:            terrariav1alpha1.ContainerName,
			Image:           image,
			ImagePullPolicy: pullPolicy,
			Args:            args,
			Env:             env,
			TTY:             true,
			Stdin:           true,
			Ports:           containerPorts,
			Resources:       resources,
			VolumeMounts:    volumeMounts,
			Lifecycle:       lifecycle,
			ReadinessProbe:  readiness,
			LivenessProbe:   liveness,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
			},
		}},
		Volumes: volumes,
	}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      StatefulSetName(server),
			Namespace: server.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: ServiceName(server),
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					MaxUnavailable: ptr.To(intstr.FromInt32(1)),
					Partition:      ptr.To(int32(0)),
				},
			},
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       podSpec,
			},
		},
	}
}

func defaultResources(server *terrariav1alpha1.TerrariaServer) corev1.ResourceRequirements {
	if server.Spec.Resources != nil {
		return *server.Spec.Resources
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "terraria-operator-bootstrap"
	}
	return hex.EncodeToString(b)
}

func gracefulStopScript(server *terrariav1alpha1.TerrariaServer) string {
	if !worldcfg.AdminAPIEnabled(server.Spec) {
		return "sleep 8"
	}
	port := worldcfg.RESTPort(server.Spec)
	return "if [ -n \"${TSHOCK_REST_TOKEN:-}\" ]; then " +
		"wget -q -O- \"http://127.0.0.1:" + strconv.FormatInt(int64(port), 10) + "/v2/server/save?token=${TSHOCK_REST_TOKEN}\" || true; " +
		"fi; sleep 8"
}
