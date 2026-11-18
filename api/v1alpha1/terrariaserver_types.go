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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PhasePending   = "Pending"
	PhaseDeploying = "Deploying"
	PhaseRunning   = "Running"
	PhaseFailed    = "Failed"

	ConditionReady             = "Ready"
	ConditionGatewayConfigured = "GatewayConfigured"
	ConditionUpdateAvailable   = "UpdateAvailable"
	ConditionAdminAPIReady     = "AdminAPIReady"

	DefaultImage           = "ghcr.io/pryaxis/tshock:stable"
	DefaultVanillaImage    = "ryshe/terraria:latest"
	DefaultGamePort        = int32(7777)
	DefaultRESTPort        = int32(7878)
	DefaultMaxPlayers      = int32(16)
	DefaultPersistenceSize = "4Gi"
	DefaultGraceSeconds    = int64(60)
	DefaultUpdateInterval  = "6h"

	ServerTypeTShock  = "TSHOCK"
	ServerTypeVanilla = "VANILLA"

	VersionModeStable = "STABLE"
	VersionModePinned = "PINNED"

	WorldSizeSmall  = "SMALL"
	WorldSizeMedium = "MEDIUM"
	WorldSizeLarge  = "LARGE"

	DifficultyClassic = "CLASSIC"
	DifficultyExpert  = "EXPERT"
	DifficultyMaster  = "MASTER"
	DifficultyJourney = "JOURNEY"

	ContainerName = "terraria"

	// WorldsMountPath is the documented Worlds directory for ryshe/terraria-style
	// layouts. Validate against ghcr.io/pryaxis/tshock before production pin.
	WorldsMountPath = "/root/.local/share/Terraria/Worlds"

	// PluginsMountPath is the TShock ServerPlugins directory inside the container.
	PluginsMountPath = "/tshock/ServerPlugins"

	// AnnotationCommand triggers a one-shot REST admin action; stripped after run.
	// Examples: "save", "broadcast Hello", "kick Alice", "ban Bob", "off"
	AnnotationCommand = "terraria.dataknife.ai/command"
)

// ServerImageConfig selects the container image. Default is first-party Pryaxis TShock.
type ServerImageConfig struct {
	// Image is the container image. Empty uses the default for serverType.
	// +optional
	Image string `json:"image,omitempty"`

	// PullPolicy for the game server container.
	// +kubebuilder:default=IfNotPresent
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// VersionConfig controls image tag selection for the TShock image.
type VersionConfig struct {
	// Mode selects STABLE (:stable tag) or PINNED (pinnedTag).
	// +kubebuilder:default=STABLE
	// +kubebuilder:validation:Enum=STABLE;PINNED
	// +optional
	Mode string `json:"mode,omitempty"`

	// PinnedTag is the exact image tag when mode=PINNED (e.g. "v6.1.0").
	// +optional
	PinnedTag string `json:"pinnedTag,omitempty"`
}

// WorldConfig maps to Terraria dedicated-server command-line args.
type WorldConfig struct {
	// Name is the world name (-worldname) and .wld basename.
	// +kubebuilder:default=main
	// +optional
	Name string `json:"name,omitempty"`

	// Size maps to -autocreate: SMALL=1, MEDIUM=2, LARGE=3.
	// +kubebuilder:default=MEDIUM
	// +kubebuilder:validation:Enum=SMALL;MEDIUM;LARGE
	// +optional
	Size string `json:"size,omitempty"`

	// Difficulty maps to -difficulty: CLASSIC=0, EXPERT=1, MASTER=2, JOURNEY=3.
	// +kubebuilder:default=CLASSIC
	// +kubebuilder:validation:Enum=CLASSIC;EXPERT;MASTER;JOURNEY
	// +optional
	Difficulty string `json:"difficulty,omitempty"`

	// Seed is optional; blank means random.
	// +optional
	Seed string `json:"seed,omitempty"`

	// MaxPlayers maps to -maxplayers.
	// +kubebuilder:default=16
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=255
	// +optional
	MaxPlayers int32 `json:"maxPlayers,omitempty"`
}

// AccessConfig controls join password and whitelist posture.
type AccessConfig struct {
	// PasswordSecretRef refs a Secret with key "password" for -pass.
	// +optional
	PasswordSecretRef *corev1.LocalObjectReference `json:"passwordSecretRef,omitempty"`

	// Whitelist enables TShock whitelist mode when supported by config.
	// +optional
	Whitelist bool `json:"whitelist,omitempty"`
}

// PersistenceConfig configures the worlds / tshock.sqlite PVC.
type PersistenceConfig struct {
	// Size is the PVC capacity. Default 4Gi.
	// +kubebuilder:default="4Gi"
	// +optional
	Size string `json:"size,omitempty"`

	// StorageClassName selects the StorageClass. Empty uses the cluster default.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// ExistingClaim adopts an existing PVC by name (DR / restore).
	// +optional
	ExistingClaim string `json:"existingClaim,omitempty"`
}

// PluginsConfig mounts or fetches TShock plugin DLLs.
type PluginsConfig struct {
	// Enabled mounts plugin storage into ServerPlugins/.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// URLs optionally fetch release artifacts (DLL/zip) into ServerPlugins/ via init container.
	// +optional
	URLs []string `json:"urls,omitempty"`

	// MountRef is a bring-your-own PVC name with DLLs (takes precedence over owned PVC).
	// +optional
	MountRef *corev1.LocalObjectReference `json:"mountRef,omitempty"`

	// Size is the plugins PVC capacity when the operator creates one. Default 1Gi.
	// +kubebuilder:default="1Gi"
	// +optional
	Size string `json:"size,omitempty"`

	// Path overrides the container mount for ServerPlugins/. Default /tshock/ServerPlugins.
	// +optional
	Path string `json:"path,omitempty"`
}

// BackupConfig configures scheduled tar backups of the worlds PVC.
type BackupConfig struct {
	// Enabled creates a backup PVC + CronJob. Default false.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Schedule is a Cron expression (UTC). Default "0 4 * * *" (04:00 daily).
	// +kubebuilder:default="0 4 * * *"
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Retain is how many compressed archives to keep. Default 7.
	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=90
	// +optional
	Retain int32 `json:"retain,omitempty"`

	// Size is the backup PVC capacity. Default 4Gi.
	// +kubebuilder:default="4Gi"
	// +optional
	Size string `json:"size,omitempty"`

	// StorageClassName for the backup PVC. Empty inherits persistence.storageClassName.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// Image is the backup job container image. Default busybox:1.36.
	// +kubebuilder:default="busybox:1.36"
	// +optional
	Image string `json:"image,omitempty"`

	// RESTSaveFirst when true (default) hits TShock REST /save before tarring.
	// +kubebuilder:default=true
	// +optional
	RESTSaveFirst *bool `json:"restSaveFirst,omitempty"`
}

// AdminAPIConfig controls the TShock REST API (TCP 7878, ClusterIP-only).
type AdminAPIConfig struct {
	// Enabled toggles REST on 7878. Default true; required by Server Manager.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// TokenSecretRef is a bring-your-own Secret with key "token".
	// When unset, the operator bootstraps an owned Secret via REST token create.
	// +optional
	TokenSecretRef *corev1.LocalObjectReference `json:"tokenSecretRef,omitempty"`

	// Port overrides the REST listen port (default 7878).
	// +kubebuilder:default=7878
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`
}

// UpdateConfig controls the TShock GitHub release compat watcher.
type UpdateConfig struct {
	// CheckInterval is how often the reconciler polls TShock GitHub releases.
	// +kubebuilder:default="6h"
	// +optional
	CheckInterval string `json:"checkInterval,omitempty"`

	// AutoUpdateImage when true patches version to PINNED at the latest TShock
	// release tag when an update is available (image roll via StatefulSet recreate).
	// Default false — opt-in only.
	// +optional
	AutoUpdateImage bool `json:"autoUpdateImage,omitempty"`
}

// GatewayConfig configures Envoy Gateway exposure (TCP-only for Terraria).
type GatewayConfig struct {
	// Enabled creates Gateway + TCPRoute resources. Default true.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Address is the dedicated external IP (Kube-VIP / MetalLB).
	// Required when enabled is true; also used as LoadBalancerIP when
	// enabled is false and serviceType is LoadBalancer.
	// +optional
	// +kubebuilder:validation:Pattern=`^$|^([0-9]{1,3}\.){3}[0-9]{1,3}$`
	Address string `json:"address,omitempty"`

	// ClassName is the GatewayClass name.
	// +kubebuilder:default="envoy"
	// +optional
	ClassName string `json:"className,omitempty"`

	// GatewayName overrides the Gateway resource name.
	// +optional
	GatewayName string `json:"gatewayName,omitempty"`

	// EnvoyProxyName overrides the EnvoyProxy resource name.
	// +optional
	EnvoyProxyName string `json:"envoyProxyName,omitempty"`

	// ServiceType overrides the backing Service type when gateway is disabled.
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +kubebuilder:default=ClusterIP
	// +optional
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
}

// AdminUIConfig is a Phase 3 stub for Server Manager.
type AdminUIConfig struct {
	// Enabled starts the Server Manager UI (Phase 3).
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

// TerrariaServerSpec defines the desired state of a Terraria dedicated server.
type TerrariaServerSpec struct {
	// ServerType selects TSHOCK (default) or VANILLA (forces ryshe-style image).
	// +kubebuilder:default=TSHOCK
	// +kubebuilder:validation:Enum=TSHOCK;VANILLA
	// +optional
	ServerType string `json:"serverType,omitempty"`

	// ServerImage selects the container image.
	// +optional
	ServerImage ServerImageConfig `json:"serverImage,omitempty"`

	// Version controls STABLE vs PINNED image tags.
	// +optional
	Version VersionConfig `json:"version,omitempty"`

	// World maps to Terraria server command-line args.
	// +optional
	World WorldConfig `json:"world,omitempty"`

	// Access controls optional join password and whitelist.
	// +optional
	Access AccessConfig `json:"access,omitempty"`

	// Persistence configures the worlds / tshock data PVC.
	// +optional
	Persistence PersistenceConfig `json:"persistence,omitempty"`

	// Plugins mounts TShock plugin DLLs (owned PVC and/or URL fetch).
	// +optional
	Plugins PluginsConfig `json:"plugins,omitempty"`

	// Backup configures an optional CronJob that tars the worlds PVC.
	// +optional
	Backup BackupConfig `json:"backup,omitempty"`

	// AdminAPI controls TShock REST (7878, ClusterIP-only).
	// +optional
	AdminAPI AdminAPIConfig `json:"adminAPI,omitempty"`

	// Update configures the TShock release compat watcher / opt-in image apply.
	// +optional
	Update UpdateConfig `json:"update,omitempty"`

	// Gateway configures Envoy Gateway exposure (TCP 7777 only).
	// +optional
	Gateway GatewayConfig `json:"gateway,omitempty"`

	// AdminUI is Phase 3 — Server Manager UI (stub).
	// +optional
	AdminUI AdminUIConfig `json:"adminUI,omitempty"`

	// ImagePullSecrets for private registries.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Resources overrides default CPU/memory.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// TerminationGracePeriodSeconds for clean world flush on stop.
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=30
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// NodeSelector pins the game server pod to specific nodes.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// TerrariaServerStatus defines the observed state of TerrariaServer.
type TerrariaServerStatus struct {
	// Phase is Pending, Deploying, Running, or Failed.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Ready is true when the StatefulSet has a ready replica.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// InstalledTerrariaVersion is scraped from TShock REST /logs or startup when known.
	// +optional
	InstalledTerrariaVersion string `json:"installedTerrariaVersion,omitempty"`

	// LatestTShockRelease is the newest TShock GitHub release tag (without leading v).
	// +optional
	LatestTShockRelease string `json:"latestTShockRelease,omitempty"`

	// LatestTShockTerrariaVersion is the Terraria version that release supports.
	// +optional
	LatestTShockTerrariaVersion string `json:"latestTShockTerrariaVersion,omitempty"`

	// UpdateAvailable is true when installed Terraria < latest TShock-supported.
	// +optional
	UpdateAvailable bool `json:"updateAvailable,omitempty"`

	// ConnectionAddress is the IP clients should use.
	// +optional
	ConnectionAddress string `json:"connectionAddress,omitempty"`

	// ConnectionPort is the TCP game port (default 7777).
	// +optional
	ConnectionPort int32 `json:"connectionPort,omitempty"`

	// Message is a human-readable status detail.
	// +optional
	Message string `json:"message,omitempty"`

	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastUpdateCheck is when TShock GitHub releases were last polled.
	// +optional
	LastUpdateCheck *metav1.Time `json:"lastUpdateCheck,omitempty"`

	// LastCommand is the last annotation command that was executed.
	// +optional
	LastCommand string `json:"lastCommand,omitempty"`

	// LastCommandResult is a short result / error for LastCommand.
	// +optional
	LastCommandResult string `json:"lastCommandResult,omitempty"`

	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=trs;terraria
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.connectionAddress`
// +kubebuilder:printcolumn:name="Port",type=integer,JSONPath=`.status.connectionPort`
// +kubebuilder:printcolumn:name="Terraria",type=string,JSONPath=`.status.installedTerrariaVersion`
// +kubebuilder:printcolumn:name="Update",type=boolean,JSONPath=`.status.updateAvailable`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// TerrariaServer is the Schema for the terrariaservers API.
type TerrariaServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TerrariaServerSpec   `json:"spec,omitempty"`
	Status TerrariaServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TerrariaServerList contains a list of TerrariaServer.
type TerrariaServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TerrariaServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TerrariaServer{}, &TerrariaServerList{})
}
