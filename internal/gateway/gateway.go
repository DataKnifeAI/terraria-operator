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

// Package gateway owns Envoy Gateway + TCPRoute wiring for TerrariaServer.
package gateway

import (
	"fmt"
	"strings"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	"github.com/DataKnifeAI/terraria-operator/internal/provisioner"
	"github.com/DataKnifeAI/terraria-operator/internal/worldcfg"
	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	ListenerGameTCP  = "game-tcp"
	defaultClassName = "envoy"
)

// Names holds derived Gateway resource names.
type Names struct {
	Gateway    string
	EnvoyProxy string
	EnvoySvc   string
	GameRoute  string
}

// DeriveNames returns resource names for a server.
func DeriveNames(server *terrariav1alpha1.TerrariaServer) Names {
	base := strings.TrimSuffix(server.Name, "-server")
	gwName := base + "-gateway"
	if server.Spec.Gateway.GatewayName != "" {
		gwName = server.Spec.Gateway.GatewayName
	}
	epName := "game-" + base + "-kubevip"
	if server.Spec.Gateway.EnvoyProxyName != "" {
		epName = server.Spec.Gateway.EnvoyProxyName
	}
	return Names{
		Gateway:    gwName,
		EnvoyProxy: epName,
		EnvoySvc:   server.Name + "-envoy",
		GameRoute:  base + "-game-tcp",
	}
}

func className(spec terrariav1alpha1.TerrariaServerSpec) string {
	if spec.Gateway.ClassName != "" {
		return spec.Gateway.ClassName
	}
	return defaultClassName
}

func gatewayLabels(server *terrariav1alpha1.TerrariaServer) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       provisioner.AppName,
		"app.kubernetes.io/instance":   server.Name,
		"app.kubernetes.io/managed-by": provisioner.ManagedBy,
		"app.kubernetes.io/component":  "envoy-gateway",
	}
}

// BuildEnvoyBackendService is the ClusterIP Service TCPRoute targets.
func BuildEnvoyBackendService(server *terrariav1alpha1.TerrariaServer) *corev1.Service {
	names := DeriveNames(server)
	port := worldcfg.GamePort()
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.EnvoySvc,
			Namespace: server.Namespace,
			Labels:    gatewayLabels(server),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: provisioner.LabelsFor(server),
			Ports: []corev1.ServicePort{
				{Name: "game", Protocol: corev1.ProtocolTCP, Port: port, TargetPort: intstr.FromInt32(port)},
			},
		},
	}
}

// BuildEnvoyProxy pins the LoadBalancer VIP via Envoy Gateway infrastructure.
func BuildEnvoyProxy(server *terrariav1alpha1.TerrariaServer) *egv1a1.EnvoyProxy {
	names := DeriveNames(server)
	return &egv1a1.EnvoyProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.EnvoyProxy,
			Namespace: server.Namespace,
			Labels:    gatewayLabels(server),
		},
		Spec: egv1a1.EnvoyProxySpec{
			Logging: egv1a1.ProxyLogging{
				Level: map[egv1a1.ProxyLogComponent]egv1a1.LogLevel{
					egv1a1.LogComponentDefault: egv1a1.LogLevelWarn,
				},
			},
			Provider: &egv1a1.EnvoyProxyProvider{
				Type: egv1a1.ProviderTypeKubernetes,
				Kubernetes: &egv1a1.EnvoyProxyKubernetesProvider{
					EnvoyService: &egv1a1.KubernetesServiceSpec{
						Type:                  ptr.To(egv1a1.ServiceTypeLoadBalancer),
						LoadBalancerIP:        ptr.To(server.Spec.Gateway.Address),
						ExternalTrafficPolicy: ptr.To(egv1a1.ServiceExternalTrafficPolicyCluster),
					},
				},
			},
		},
	}
}

// BuildGateway creates the Gateway with a single TCP listener on 7777.
func BuildGateway(server *terrariav1alpha1.TerrariaServer) *gatewayv1.Gateway {
	names := DeriveNames(server)
	sameNS := &gatewayv1.AllowedRoutes{
		Namespaces: &gatewayv1.RouteNamespaces{From: ptr.To(gatewayv1.NamespacesFromSame)},
	}
	port := worldcfg.GamePort()
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Gateway,
			Namespace: server.Namespace,
			Labels:    gatewayLabels(server),
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(className(server.Spec)),
			Addresses: []gatewayv1.GatewaySpecAddress{
				{Type: ptr.To(gatewayv1.IPAddressType), Value: server.Spec.Gateway.Address},
			},
			Infrastructure: &gatewayv1.GatewayInfrastructure{
				ParametersRef: &gatewayv1.LocalParametersReference{
					Group: gatewayv1.Group(egv1a1.GroupVersion.Group),
					Kind:  gatewayv1.Kind("EnvoyProxy"),
					Name:  names.EnvoyProxy,
				},
			},
			Listeners: []gatewayv1.Listener{{
				Name:          gatewayv1.SectionName(ListenerGameTCP),
				Port:          gatewayv1.PortNumber(port),
				Protocol:      gatewayv1.TCPProtocolType,
				AllowedRoutes: sameNS,
			}},
		},
	}
}

// BuildTCPRoute routes game TCP to the envoy backend Service.
func BuildTCPRoute(server *terrariav1alpha1.TerrariaServer) *gatewayv1.TCPRoute {
	names := DeriveNames(server)
	port := worldcfg.GamePort()
	return &gatewayv1.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.GameRoute,
			Namespace: server.Namespace,
			Labels:    gatewayLabels(server),
		},
		Spec: gatewayv1.TCPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name:        gatewayv1.ObjectName(names.Gateway),
					Namespace:   ptr.To(gatewayv1.Namespace(server.Namespace)),
					SectionName: ptr.To(gatewayv1.SectionName(ListenerGameTCP)),
				}},
			},
			Rules: []gatewayv1.TCPRouteRule{{
				BackendRefs: []gatewayv1.BackendRef{{
					BackendObjectReference: gatewayv1.BackendObjectReference{
						Name: gatewayv1.ObjectName(names.EnvoySvc),
						Port: ptr.To(gatewayv1.PortNumber(port)),
					},
				}},
			}},
		},
	}
}

// ConnectionAddress returns the preferred client-facing IP.
func ConnectionAddress(server *terrariav1alpha1.TerrariaServer, gw *gatewayv1.Gateway) string {
	if gw != nil && len(gw.Status.Addresses) > 0 {
		return gw.Status.Addresses[0].Value
	}
	return server.Spec.Gateway.Address
}

// MissingGatewayClassMessage explains a missing GatewayClass.
func MissingGatewayClassMessage(class string) string {
	return fmt.Sprintf("GatewayClass %q not found or not Accepted — install Envoy Gateway (or set gateway.enabled=false and use Service LoadBalancer)", class)
}
