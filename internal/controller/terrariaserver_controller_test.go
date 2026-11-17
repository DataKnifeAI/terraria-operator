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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
)

var _ = Describe("TerrariaServer Controller", func() {
	Context("When reconciling a TerrariaServer", func() {
		const (
			resourceName      = "main"
			resourceNamespace = "default"
		)

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: resourceNamespace}
		gatewayDisabled := false

		makeServer := func() *terrariav1alpha1.TerrariaServer {
			return &terrariav1alpha1.TerrariaServer{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: resourceNamespace},
				Spec: terrariav1alpha1.TerrariaServerSpec{
					ServerType: "TSHOCK",
					World: terrariav1alpha1.WorldConfig{
						Name:       "main",
						Size:       "MEDIUM",
						Difficulty: "CLASSIC",
						MaxPlayers: 16,
					},
					Gateway: terrariav1alpha1.GatewayConfig{
						Enabled:     &gatewayDisabled,
						ServiceType: corev1.ServiceTypeClusterIP,
					},
					Persistence: terrariav1alpha1.PersistenceConfig{Size: "4Gi"},
					AdminAPI:    terrariav1alpha1.AdminAPIConfig{Enabled: &gatewayDisabled},
				},
			}
		}

		deleteServer := func() {
			resource := &terrariav1alpha1.TerrariaServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if apierrors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			controllerutil.RemoveFinalizer(resource, finalizerName)
			_ = k8sClient.Update(ctx, resource)
			_ = k8sClient.Delete(ctx, resource)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, &terrariav1alpha1.TerrariaServer{})
				return apierrors.IsNotFound(err)
			}, time.Second*5, time.Millisecond*100).Should(BeTrue())
		}

		BeforeEach(func() {
			deleteServer()
		})

		AfterEach(func() {
			deleteServer()
			_ = k8sClient.DeleteAllOf(ctx, &appsv1.StatefulSet{}, client.InNamespace(resourceNamespace))
			_ = k8sClient.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(resourceNamespace))
			_ = k8sClient.DeleteAllOf(ctx, &corev1.PersistentVolumeClaim{}, client.InNamespace(resourceNamespace))
			_ = k8sClient.DeleteAllOf(ctx, &corev1.Secret{}, client.InNamespace(resourceNamespace))
		})

		It("should reject when gateway enabled without address", func() {
			s := makeServer()
			enabled := true
			s.Spec.Gateway.Enabled = &enabled
			s.Spec.Gateway.Address = ""
			Expect(k8sClient.Create(ctx, s)).To(Succeed())

			r := &TerrariaServerReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			got := &terrariav1alpha1.TerrariaServer{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, got)).To(Succeed())
			Expect(got.Status.Phase).To(Equal(terrariav1alpha1.PhaseFailed))

			sts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, typeNamespacedName, sts)
			Expect(err).To(HaveOccurred())
		})

		It("should create PVC, Service, StatefulSet and be deterministic on second reconcile", func() {
			Expect(k8sClient.Create(ctx, makeServer())).To(Succeed())

			r := &TerrariaServerReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}

			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			pvc := &corev1.PersistentVolumeClaim{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName + "-data", Namespace: resourceNamespace}, pvc)).To(Succeed())

			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, svc)).To(Succeed())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))

			args := sts.Spec.Template.Spec.Containers[0].Args
			Expect(args).To(ContainElement("-autocreate"))
			Expect(args).To(ContainElement("2"))
			Expect(args).To(ContainElement("-difficulty"))
			Expect(args).To(ContainElement("0"))
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal(terrariav1alpha1.DefaultImage))
		})

		It("should adopt existingClaim and enforce replicas=1", func() {
			claim := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "adopted-world", Namespace: resourceNamespace},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("4Gi")},
					},
				},
			}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			s := makeServer()
			s.Spec.Persistence.ExistingClaim = "adopted-world"
			Expect(k8sClient.Create(ctx, s)).To(Succeed())

			r := &TerrariaServerReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal("adopted-world"))
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))

			_ = k8sClient.Delete(ctx, claim)
		})
	})
})
