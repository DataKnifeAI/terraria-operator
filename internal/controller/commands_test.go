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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	terrariav1alpha1 "github.com/DataKnifeAI/terraria-operator/api/v1alpha1"
	"github.com/DataKnifeAI/terraria-operator/internal/tshock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestParseAndRunAdminCommand(t *testing.T) {
	var saw string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	cli := &tshock.Client{BaseURL: ts.URL, Token: "tok", HTTP: ts.Client()}
	cases := []struct {
		cmd      string
		wantPath string
	}{
		{"save", "/v2/server/save"},
		{"broadcast hi there", "/v2/server/broadcast"},
		{"kick Alice", "/v2/players/kick"},
		{"ban Bob grief", "/v2/players/ban"},
		{"off", "/v2/server/off"},
	}
	for _, tc := range cases {
		saw = ""
		if err := runAdminCommand(context.Background(), cli, tc.cmd); err != nil {
			t.Fatalf("%s: %v", tc.cmd, err)
		}
		if !strings.Contains(saw, tc.wantPath) {
			t.Fatalf("%s: got %q want path containing %q", tc.cmd, saw, tc.wantPath)
		}
	}
}

func TestClearCommandAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = terrariav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	server := &terrariav1alpha1.TerrariaServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "main",
			Namespace: "default",
			Annotations: map[string]string{
				terrariav1alpha1.AnnotationCommand: "save",
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server).Build()
	r := &TerrariaServerReconciler{Client: c, Scheme: scheme}
	res, err := r.clearCommandAnnotation(context.Background(), server, "save", "ok")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Ran || res.Command != "save" || res.Message != "ok" {
		t.Fatalf("unexpected result %#v", res)
	}
	got := &terrariav1alpha1.TerrariaServer{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "main", Namespace: "default"}, got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Annotations[terrariav1alpha1.AnnotationCommand]; ok {
		t.Fatal("annotation not cleared")
	}
}
