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

import terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"

const (
	LabelAppName      = "app.kubernetes.io/name"
	LabelAppInstance  = "app.kubernetes.io/instance"
	LabelAppManagedBy = "app.kubernetes.io/managed-by"
	LabelAppComponent = "app.kubernetes.io/component"

	ManagedBy     = "terraria-operator"
	ComponentGame = "game-server"
	AppName       = "terraria-server"

	SecretKeyToken        = "token"
	SecretKeyRestPassword = "rest-password"
	SecretKeyGamePassword = "password"
)

func LabelsFor(server *terrariav1alpha1.TerrariaServer) map[string]string {
	return map[string]string{
		LabelAppName:      AppName,
		LabelAppInstance:  server.Name,
		LabelAppManagedBy: ManagedBy,
		LabelAppComponent: ComponentGame,
	}
}

func PVCName(server *terrariav1alpha1.TerrariaServer) string {
	if server.Spec.Persistence.ExistingClaim != "" {
		return server.Spec.Persistence.ExistingClaim
	}
	return server.Name + "-data"
}

func StatefulSetName(server *terrariav1alpha1.TerrariaServer) string {
	return server.Name
}

func ServiceName(server *terrariav1alpha1.TerrariaServer) string {
	return server.Name
}

func SecretName(server *terrariav1alpha1.TerrariaServer) string {
	return server.Name + "-secrets"
}

func TokenSecretName(server *terrariav1alpha1.TerrariaServer) string {
	if server.Spec.AdminAPI.TokenSecretRef != nil && server.Spec.AdminAPI.TokenSecretRef.Name != "" {
		return server.Spec.AdminAPI.TokenSecretRef.Name
	}
	return server.Name + "-rest-token"
}
