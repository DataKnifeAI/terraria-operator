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

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	"github.com/DataKnifeAI/terraria-operator/internal/gateway"
	"github.com/DataKnifeAI/terraria-operator/internal/worldcfg"
	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type gatewayResult struct {
	Configured bool
	Address    string
	Message    string
}

func (r *TerrariaServerReconciler) reconcileGatewayStack(
	ctx context.Context,
	server *terrariav1alpha1.TerrariaServer,
) (gatewayResult, error) {
	names := gateway.DeriveNames(server)

	if !worldcfg.GatewayEnabled(server.Spec) {
		_ = r.deleteOwnedGatewayStack(ctx, server, names)
		msg := "gateway disabled; using Service only (set gateway.serviceType=NodePort|LoadBalancer)"
		addr := server.Spec.Gateway.Address
		if server.Spec.Gateway.ServiceType == corev1.ServiceTypeLoadBalancer && addr != "" {
			msg = fmt.Sprintf("gateway disabled; Service LoadBalancer VIP %s (TCP %d)", addr, worldcfg.GamePort())
		}
		return gatewayResult{Configured: true, Address: addr, Message: msg}, nil
	}

	class := server.Spec.Gateway.ClassName
	if class == "" {
		class = "envoy"
	}
	gc := &gatewayv1.GatewayClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: class}, gc); err != nil {
		if apierrors.IsNotFound(err) {
			return gatewayResult{
				Configured: false,
				Message:    gateway.MissingGatewayClassMessage(class),
			}, nil
		}
		return gatewayResult{}, err
	}
	accepted := false
	for _, c := range gc.Status.Conditions {
		if c.Type == string(gatewayv1.GatewayClassConditionStatusAccepted) && c.Status == metav1.ConditionTrue {
			accepted = true
			break
		}
	}
	if !accepted {
		return gatewayResult{
			Configured: false,
			Message:    fmt.Sprintf("GatewayClass %q exists but is not Accepted", class),
		}, nil
	}

	envoySvc := gateway.BuildEnvoyBackendService(server)
	if err := controllerutil.SetControllerReference(server, envoySvc, r.Scheme); err != nil {
		return gatewayResult{}, err
	}
	if err := r.applyObject(ctx, envoySvc); err != nil {
		return gatewayResult{}, err
	}

	ep := gateway.BuildEnvoyProxy(server)
	if err := controllerutil.SetControllerReference(server, ep, r.Scheme); err != nil {
		return gatewayResult{}, err
	}
	if err := r.applyObject(ctx, ep); err != nil {
		return gatewayResult{}, err
	}

	gw := gateway.BuildGateway(server)
	if err := controllerutil.SetControllerReference(server, gw, r.Scheme); err != nil {
		return gatewayResult{}, err
	}
	if err := r.applyObject(ctx, gw); err != nil {
		return gatewayResult{}, err
	}

	tcpRoute := gateway.BuildTCPRoute(server)
	if err := controllerutil.SetControllerReference(server, tcpRoute, r.Scheme); err != nil {
		return gatewayResult{}, err
	}
	if err := r.applyObject(ctx, tcpRoute); err != nil {
		return gatewayResult{}, err
	}

	live := &gatewayv1.Gateway{}
	_ = r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: names.Gateway}, live)
	addr := gateway.ConnectionAddress(server, live)
	return gatewayResult{
		Configured: true,
		Address:    addr,
		Message:    fmt.Sprintf("Envoy Gateway %s on %s (TCP %d)", names.Gateway, addr, worldcfg.GamePort()),
	}, nil
}

func (r *TerrariaServerReconciler) deleteOwnedGatewayStack(
	ctx context.Context,
	server *terrariav1alpha1.TerrariaServer,
	names gateway.Names,
) error {
	toDelete := []client.Object{
		&gatewayv1alpha2.TCPRoute{ObjectMeta: metav1.ObjectMeta{Name: names.GameRoute, Namespace: server.Namespace}},
		&gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: names.Gateway, Namespace: server.Namespace}},
		&egv1a1.EnvoyProxy{ObjectMeta: metav1.ObjectMeta{Name: names.EnvoyProxy, Namespace: server.Namespace}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: names.EnvoySvc, Namespace: server.Namespace}},
	}
	for _, obj := range toDelete {
		if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
