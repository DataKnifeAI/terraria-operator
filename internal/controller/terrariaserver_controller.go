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
	"net/http"
	"time"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	"github.com/DataKnifeAI/terraria-operator/internal/provisioner"
	"github.com/DataKnifeAI/terraria-operator/internal/tshock"
	"github.com/DataKnifeAI/terraria-operator/internal/worldcfg"
	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const finalizerName = "terraria.dataknife.ai/finalizer"

// TerrariaServerReconciler reconciles a TerrariaServer object.
type TerrariaServerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Versions *tshock.VersionClient
	RESTHTTP *http.Client
}

// +kubebuilder:rbac:groups=terraria.dataknife.ai,resources=terrariaservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=terraria.dataknife.ai,resources=terrariaservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=terraria.dataknife.ai,resources=terrariaservers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.envoyproxy.io,resources=envoyproxies,verbs=get;list;watch;create;update;patch;delete

func (r *TerrariaServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	server := &terrariav1alpha1.TerrariaServer{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(server, finalizerName) {
		controllerutil.AddFinalizer(server, finalizerName)
		if err := r.Update(ctx, server); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if !server.DeletionTimestamp.IsZero() {
		controllerutil.RemoveFinalizer(server, finalizerName)
		if err := r.Update(ctx, server); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if err := worldcfg.ValidateSpec(server.Spec); err != nil {
		if r.Recorder != nil {
			r.Recorder.Event(server, corev1.EventTypeWarning, "ValidationFailed", err.Error())
		}
		_ = r.patchStatus(ctx, server, func(s *terrariav1alpha1.TerrariaServer) {
			s.Status.Phase = terrariav1alpha1.PhaseFailed
			s.Status.Ready = false
			s.Status.Message = err.Error()
			s.Status.ObservedGeneration = s.Generation
			meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
				Type:               terrariav1alpha1.ConditionReady,
				Status:             metav1.ConditionFalse,
				Reason:             "ValidationFailed",
				Message:            err.Error(),
				ObservedGeneration: s.Generation,
			})
		})
		log.Info("validation failed — not creating children", "error", err)
		return ctrl.Result{}, nil
	}

	password := ""
	if ref := server.Spec.Access.PasswordSecretRef; ref != nil && ref.Name != "" {
		sec := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: ref.Name}, sec); err != nil {
			_ = r.patchStatus(ctx, server, func(s *terrariav1alpha1.TerrariaServer) {
				s.Status.Phase = terrariav1alpha1.PhaseFailed
				s.Status.Ready = false
				s.Status.Message = fmt.Sprintf("passwordSecretRef %q: %v", ref.Name, err)
			})
			return ctrl.Result{}, nil
		}
		password = string(sec.Data[provisioner.SecretKeyGamePassword])
	}

	args, err := worldcfg.ArgsFromSpec(server.Spec, password)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 1. REST token Secret (bootstrap shell; token filled after pod is up).
	if secret := provisioner.BuildRESTBootstrapSecret(server, nil); secret != nil {
		existing := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}, existing); err == nil {
			secret = provisioner.BuildRESTBootstrapSecret(server, existing)
		}
		if secret != nil {
			if err := controllerutil.SetControllerReference(server, secret, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.applyObject(ctx, secret); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// 2. PVC (respect existingClaim).
	if pvc := provisioner.BuildPVC(server); pvc != nil {
		if err := controllerutil.SetControllerReference(server, pvc, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.applyObject(ctx, pvc); err != nil {
			return ctrl.Result{}, err
		}
	} else if claim := server.Spec.Persistence.ExistingClaim; claim != "" {
		existing := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: claim}, existing); err != nil {
			_ = r.patchStatus(ctx, server, func(s *terrariav1alpha1.TerrariaServer) {
				s.Status.Phase = terrariav1alpha1.PhaseFailed
				s.Status.Ready = false
				s.Status.Message = fmt.Sprintf("existingClaim %q not found: %v", claim, err)
			})
			return ctrl.Result{}, nil
		}
	}

	// 2b. Plugins PVC.
	if pvc := provisioner.BuildPluginsPVC(server); pvc != nil {
		if err := controllerutil.SetControllerReference(server, pvc, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.applyObject(ctx, pvc); err != nil {
			return ctrl.Result{}, err
		}
	} else if server.Spec.Plugins.Enabled && server.Spec.Plugins.MountRef != nil && server.Spec.Plugins.MountRef.Name != "" {
		existing := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: server.Spec.Plugins.MountRef.Name}, existing); err != nil {
			_ = r.patchStatus(ctx, server, func(s *terrariav1alpha1.TerrariaServer) {
				s.Status.Phase = terrariav1alpha1.PhaseFailed
				s.Status.Ready = false
				s.Status.Message = fmt.Sprintf("plugins.mountRef %q not found: %v", server.Spec.Plugins.MountRef.Name, err)
			})
			return ctrl.Result{}, nil
		}
	}

	// 2c. Backup PVC + CronJob.
	if err := r.reconcileBackup(ctx, server); err != nil {
		return ctrl.Result{}, err
	}

	// 3. Service.
	svc := provisioner.BuildService(server)
	if err := controllerutil.SetControllerReference(server, svc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.applyObject(ctx, svc); err != nil {
		return ctrl.Result{}, err
	}

	// 4. Gateway / TCPRoute.
	gw, err := r.reconcileGatewayStack(ctx, server)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 5. StatefulSet (replicas hard-capped at 1).
	sts := provisioner.BuildStatefulSet(server, args)
	if err := controllerutil.SetControllerReference(server, sts, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.applyObject(ctx, sts); err != nil {
		return ctrl.Result{}, err
	}

	ready := false
	phase := terrariav1alpha1.PhaseDeploying
	message := "reconciling child resources"
	current := &appsv1.StatefulSet{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: sts.Name}, current); err == nil {
		if current.Status.ReadyReplicas >= 1 {
			ready = true
			phase = terrariav1alpha1.PhaseRunning
			message = "terraria StatefulSet is ready"
		}
		if current.Spec.Replicas != nil && *current.Spec.Replicas != 1 {
			one := int32(1)
			current.Spec.Replicas = &one
			_ = r.Update(ctx, current)
		}
	}
	if gw.Message != "" {
		message = message + "; " + gw.Message
	}

	connAddr := gw.Address
	if !worldcfg.GatewayEnabled(server.Spec) {
		connAddr = server.Spec.Gateway.Address
	}

	// 6. REST token bootstrap (idempotent; deferred until REST is reachable).
	tokenRes, err := r.ensureRESTToken(ctx, server)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 6b. Annotation commands (REST-backed; strip after run).
	cmdRes, err := r.handleAnnotationCommand(ctx, server)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 7. Compat / update watcher (+ opt-in autoUpdateImage apply).
	updateRes, requeueAfter := r.checkUpdates(ctx, server)
	if updateRes.Applied {
		return ctrl.Result{Requeue: true}, nil
	}
	if updateRes.Message != "" {
		message = message + "; " + updateRes.Message
	}

	_ = r.patchStatus(ctx, server, func(s *terrariav1alpha1.TerrariaServer) {
		s.Status.Phase = phase
		s.Status.Ready = ready
		s.Status.Message = message
		s.Status.ConnectionAddress = connAddr
		s.Status.ConnectionPort = worldcfg.GamePort()
		s.Status.ObservedGeneration = server.Generation
		if cmdRes.Ran {
			s.Status.LastCommand = cmdRes.Command
			s.Status.LastCommandResult = cmdRes.Message
		}
		if updateRes.LatestTShockRelease != "" {
			s.Status.LatestTShockRelease = updateRes.LatestTShockRelease
			s.Status.LatestTShockTerrariaVersion = updateRes.LatestTShockTerrariaVersion
			s.Status.UpdateAvailable = updateRes.UpdateAvailable
			now := metav1.Now()
			s.Status.LastUpdateCheck = &now
		}
		if updateRes.InstalledTerrariaVersion != "" {
			s.Status.InstalledTerrariaVersion = updateRes.InstalledTerrariaVersion
		}
		meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
			Type:               terrariav1alpha1.ConditionReady,
			Status:             boolCond(ready),
			Reason:             readyReason(ready),
			Message:            message,
			ObservedGeneration: server.Generation,
		})
		gwStatus := metav1.ConditionFalse
		gwReason := "GatewayPending"
		if gw.Configured {
			gwStatus = metav1.ConditionTrue
			gwReason = "GatewayConfigured"
		}
		meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
			Type:               terrariav1alpha1.ConditionGatewayConfigured,
			Status:             gwStatus,
			Reason:             gwReason,
			Message:            gw.Message,
			ObservedGeneration: server.Generation,
		})
		adminStatus := metav1.ConditionFalse
		adminReason := "AdminAPIPending"
		if tokenRes.Ready {
			adminStatus = metav1.ConditionTrue
			adminReason = "AdminAPIReady"
		}
		meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
			Type:               terrariav1alpha1.ConditionAdminAPIReady,
			Status:             adminStatus,
			Reason:             adminReason,
			Message:            tokenRes.Message,
			ObservedGeneration: server.Generation,
		})
		updStatus := metav1.ConditionFalse
		updReason := "UpToDate"
		updMsg := "no update available"
		if updateRes.UpdateAvailable {
			updStatus = metav1.ConditionTrue
			updReason = "UpdateAvailable"
			updMsg = fmt.Sprintf("TShock %s supports Terraria %s (installed %s)",
				updateRes.LatestTShockRelease, updateRes.LatestTShockTerrariaVersion, updateRes.InstalledTerrariaVersion)
		}
		meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
			Type:               terrariav1alpha1.ConditionUpdateAvailable,
			Status:             updStatus,
			Reason:             updReason,
			Message:            updMsg,
			ObservedGeneration: server.Generation,
		})
	})

	result := ctrl.Result{}
	if requeueAfter > 0 {
		result.RequeueAfter = requeueAfter
	}
	if !tokenRes.Ready && worldcfg.AdminAPIEnabled(server.Spec) {
		if result.RequeueAfter == 0 || result.RequeueAfter > 30*time.Second {
			result.RequeueAfter = 30 * time.Second
		}
	}
	return result, nil
}

func (r *TerrariaServerReconciler) applyObject(ctx context.Context, obj client.Object) error {
	key := client.ObjectKeyFromObject(obj)
	existing := obj.DeepCopyObject().(client.Object)
	err := r.Get(ctx, key, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, obj)
	}
	if err != nil {
		return err
	}

	switch desired := obj.(type) {
	case *corev1.Service:
		cur := existing.(*corev1.Service)
		cur.Spec.Type = desired.Spec.Type
		cur.Spec.Ports = desired.Spec.Ports
		cur.Spec.Selector = desired.Spec.Selector
		cur.Spec.LoadBalancerIP = desired.Spec.LoadBalancerIP
		cur.Spec.ExternalTrafficPolicy = desired.Spec.ExternalTrafficPolicy
		if desired.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal {
			cur.Spec.HealthCheckNodePort = 0
		}
		cur.Labels = desired.Labels
		return r.Update(ctx, cur)
	case *appsv1.StatefulSet:
		cur := existing.(*appsv1.StatefulSet)
		cur.Spec.Replicas = desired.Spec.Replicas
		cur.Spec.Template = desired.Spec.Template
		cur.Spec.UpdateStrategy = desired.Spec.UpdateStrategy
		cur.Labels = desired.Labels
		return r.Update(ctx, cur)
	case *corev1.PersistentVolumeClaim:
		cur := existing.(*corev1.PersistentVolumeClaim)
		cur.Labels = desired.Labels
		return r.Update(ctx, cur)
	case *corev1.Secret:
		cur := existing.(*corev1.Secret)
		if cur.Data == nil {
			cur.Data = map[string][]byte{}
		}
		for k, v := range desired.Data {
			if len(cur.Data[k]) == 0 {
				cur.Data[k] = v
			}
		}
		cur.Labels = desired.Labels
		cur.OwnerReferences = desired.OwnerReferences
		return r.Update(ctx, cur)
	case *egv1a1.EnvoyProxy:
		cur := existing.(*egv1a1.EnvoyProxy)
		cur.Labels = desired.Labels
		cur.OwnerReferences = desired.OwnerReferences
		cur.Spec = desired.Spec
		return r.Update(ctx, cur)
	case *gatewayv1.Gateway:
		cur := existing.(*gatewayv1.Gateway)
		cur.Labels = desired.Labels
		cur.OwnerReferences = desired.OwnerReferences
		cur.Spec = desired.Spec
		return r.Update(ctx, cur)
	case *gatewayv1.TCPRoute:
		cur := existing.(*gatewayv1.TCPRoute)
		cur.Labels = desired.Labels
		cur.OwnerReferences = desired.OwnerReferences
		cur.Spec = desired.Spec
		return r.Update(ctx, cur)
	case *batchv1.CronJob:
		cur := existing.(*batchv1.CronJob)
		cur.Labels = desired.Labels
		cur.OwnerReferences = desired.OwnerReferences
		cur.Spec = desired.Spec
		return r.Update(ctx, cur)
	default:
		return fmt.Errorf("unsupported apply type %T", obj)
	}
}

func (r *TerrariaServerReconciler) reconcileBackup(ctx context.Context, server *terrariav1alpha1.TerrariaServer) error {
	if !server.Spec.Backup.Enabled {
		cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{
			Name: provisioner.BackupCronJobName(server), Namespace: server.Namespace,
		}}
		if err := r.Delete(ctx, cj); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}
	if pvc := provisioner.BuildBackupPVC(server); pvc != nil {
		if err := controllerutil.SetControllerReference(server, pvc, r.Scheme); err != nil {
			return err
		}
		if err := r.applyObject(ctx, pvc); err != nil {
			return err
		}
	}
	cj := provisioner.BuildBackupCronJob(server)
	if cj == nil {
		return nil
	}
	if err := controllerutil.SetControllerReference(server, cj, r.Scheme); err != nil {
		return err
	}
	return r.applyObject(ctx, cj)
}

func (r *TerrariaServerReconciler) patchStatus(ctx context.Context, server *terrariav1alpha1.TerrariaServer, mutate func(*terrariav1alpha1.TerrariaServer)) error {
	latest := &terrariav1alpha1.TerrariaServer{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(server), latest); err != nil {
		return err
	}
	mutate(latest)
	return r.Status().Update(ctx, latest)
}

func boolCond(ready bool) metav1.ConditionStatus {
	if ready {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func readyReason(ready bool) string {
	if ready {
		return "StatefulSetReady"
	}
	return "StatefulSetNotReady"
}

// SetupWithManager sets up the controller with the Manager.
func (r *TerrariaServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Versions == nil {
		r.Versions = tshock.DefaultVersionClient()
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&terrariav1alpha1.TerrariaServer{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&batchv1.CronJob{}).
		Owns(&gatewayv1.Gateway{}).
		Owns(&gatewayv1.TCPRoute{}).
		Owns(&egv1a1.EnvoyProxy{}).
		Named("terrariaserver").
		Complete(r)
}
