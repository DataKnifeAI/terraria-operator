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
	"github.com/DataKnifeAI/terraria-operator/internal/provisioner"
	"github.com/DataKnifeAI/terraria-operator/internal/tshock"
	"github.com/DataKnifeAI/terraria-operator/internal/worldcfg"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type tokenResult struct {
	Ready   bool
	Message string
}

// ensureRESTToken bootstraps a TShock REST token when needed.
// Flow:
//  1. If adminAPI.tokenSecretRef is set, require key "token" and treat as ready.
//  2. Else ensure owned Secret with rest-password; if token already present, probe status.
//  3. Else call /v2/token/create/{password} against the in-cluster Service and store token.
//
// Idempotent: existing token is reused when REST status succeeds.
func (r *TerrariaServerReconciler) ensureRESTToken(ctx context.Context, server *terrariav1alpha1.TerrariaServer) (tokenResult, error) {
	log := logf.FromContext(ctx)
	if !worldcfg.AdminAPIEnabled(server.Spec) {
		return tokenResult{Ready: true, Message: "admin API disabled"}, nil
	}

	secretName := provisioner.TokenSecretName(server)
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: secretName}, secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return tokenResult{}, err
	}
	if apierrors.IsNotFound(err) {
		secret = nil
	}

	if server.Spec.AdminAPI.TokenSecretRef != nil && server.Spec.AdminAPI.TokenSecretRef.Name != "" {
		if secret == nil || len(secret.Data[provisioner.SecretKeyToken]) == 0 {
			return tokenResult{Ready: false, Message: fmt.Sprintf("tokenSecretRef %q missing key %q", secretName, provisioner.SecretKeyToken)}, nil
		}
		return tokenResult{Ready: true, Message: "using bring-your-own REST token"}, nil
	}

	desired := provisioner.BuildRESTBootstrapSecret(server, secret)
	if desired == nil {
		return tokenResult{Ready: true, Message: "no REST secret needed"}, nil
	}
	if err := r.applyObject(ctx, desired); err != nil {
		return tokenResult{}, err
	}
	if err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: secretName}, secret); err != nil {
		return tokenResult{}, err
	}

	baseURL := fmt.Sprintf("http://%s.%s.svc:%d", provisioner.ServiceName(server), server.Namespace, worldcfg.RESTPort(server.Spec))
	token := string(secret.Data[provisioner.SecretKeyToken])
	if token != "" {
		cli := &tshock.Client{BaseURL: baseURL, Token: token, HTTP: r.RESTHTTP}
		if _, err := cli.GetStatus(ctx); err == nil {
			return tokenResult{Ready: true, Message: "REST token valid"}, nil
		}
		log.Info("existing REST token failed status probe; will recreate", "error", err)
	}

	restPass := string(secret.Data[provisioner.SecretKeyRestPassword])
	if restPass == "" {
		return tokenResult{Ready: false, Message: "REST bootstrap password missing from secret"}, nil
	}

	cli := &tshock.Client{BaseURL: baseURL, HTTP: r.RESTHTTP}
	newToken, err := cli.CreateToken(ctx, restPass, "terraria-operator")
	if err != nil {
		// Pod may not be ready yet — requeue without failing reconcile hard.
		log.Info("REST token bootstrap deferred", "error", err)
		return tokenResult{Ready: false, Message: fmt.Sprintf("waiting for REST token bootstrap: %v", err)}, nil
	}

	secret.Data[provisioner.SecretKeyToken] = []byte(newToken)
	if err := r.Update(ctx, secret); err != nil {
		return tokenResult{}, err
	}
	return tokenResult{Ready: true, Message: "REST token bootstrapped"}, nil
}
