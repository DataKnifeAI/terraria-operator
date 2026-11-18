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
	"strings"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	"github.com/DataKnifeAI/terraria-operator/internal/provisioner"
	"github.com/DataKnifeAI/terraria-operator/internal/tshock"
	"github.com/DataKnifeAI/terraria-operator/internal/worldcfg"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type commandResult struct {
	Ran     bool
	Command string
	Message string
}

// handleAnnotationCommand runs a one-shot REST admin action from
// terraria.dataknife.ai/command and strips the annotation.
// Supported: save | broadcast <msg> | kick <player> [reason] | ban <player> [reason] | off
func (r *TerrariaServerReconciler) handleAnnotationCommand(ctx context.Context, server *terrariav1alpha1.TerrariaServer) (commandResult, error) {
	log := logf.FromContext(ctx)
	raw, ok := server.Annotations[terrariav1alpha1.AnnotationCommand]
	if !ok {
		return commandResult{}, nil
	}
	cmd := strings.TrimSpace(raw)
	if cmd == "" {
		return r.clearCommandAnnotation(ctx, server, "", "empty command annotation cleared")
	}

	if !worldcfg.AdminAPIEnabled(server.Spec) {
		return r.clearCommandAnnotation(ctx, server, cmd, "admin API disabled; command ignored")
	}

	token, err := r.loadRESTToken(ctx, server)
	if err != nil || token == "" {
		msg := "REST token unavailable"
		if err != nil {
			msg = err.Error()
		}
		return r.clearCommandAnnotation(ctx, server, cmd, msg)
	}

	baseURL := fmt.Sprintf("http://%s.%s.svc:%d", provisioner.ServiceName(server), server.Namespace, worldcfg.RESTPort(server.Spec))
	cli := &tshock.Client{BaseURL: baseURL, Token: token, HTTP: r.RESTHTTP}

	runErr := runAdminCommand(ctx, cli, cmd)
	result := "ok"
	if runErr != nil {
		result = runErr.Error()
		log.Info("annotation command failed", "command", cmd, "error", runErr)
		if r.Recorder != nil {
			r.Recorder.Event(server, corev1.EventTypeWarning, "CommandFailed", result)
		}
	} else {
		log.Info("annotation command succeeded", "command", cmd)
		if r.Recorder != nil {
			r.Recorder.Event(server, corev1.EventTypeNormal, "CommandSucceeded", cmd)
		}
	}
	return r.clearCommandAnnotation(ctx, server, cmd, result)
}

func runAdminCommand(ctx context.Context, cli *tshock.Client, cmd string) error {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}
	verb := strings.ToLower(parts[0])
	switch verb {
	case "save":
		return cli.Save(ctx)
	case "broadcast", "say", "announce":
		msg := strings.TrimSpace(strings.TrimPrefix(cmd, parts[0]))
		if msg == "" {
			return fmt.Errorf("broadcast requires a message")
		}
		return cli.Broadcast(ctx, msg)
	case "kick":
		if len(parts) < 2 {
			return fmt.Errorf("kick requires a player name")
		}
		return cli.Kick(ctx, parts[1], strings.Join(parts[2:], " "))
	case "ban":
		if len(parts) < 2 {
			return fmt.Errorf("ban requires a player name")
		}
		return cli.Ban(ctx, parts[1], strings.Join(parts[2:], " "))
	case "off", "shutdown", "stop":
		return cli.Off(ctx)
	default:
		return fmt.Errorf("unsupported command %q (save|broadcast|kick|ban|off)", verb)
	}
}

func (r *TerrariaServerReconciler) clearCommandAnnotation(ctx context.Context, server *terrariav1alpha1.TerrariaServer, cmd, result string) (commandResult, error) {
	latest := &terrariav1alpha1.TerrariaServer{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(server), latest); err != nil {
		return commandResult{}, err
	}
	if latest.Annotations != nil {
		delete(latest.Annotations, terrariav1alpha1.AnnotationCommand)
	}
	if err := r.Update(ctx, latest); err != nil {
		return commandResult{}, err
	}
	server.Annotations = latest.Annotations
	return commandResult{Ran: true, Command: cmd, Message: result}, nil
}

func (r *TerrariaServerReconciler) loadRESTToken(ctx context.Context, server *terrariav1alpha1.TerrariaServer) (string, error) {
	sec := &corev1.Secret{}
	name := provisioner.TokenSecretName(server)
	if err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: name}, sec); err != nil {
		return "", err
	}
	return string(sec.Data[provisioner.SecretKeyToken]), nil
}
