/*
Copyright 2021 The Kruise Authors.

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

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// Generate OpenAPI spec definitions for Kruise Resources
func main() {
	generateSwaggerJSON()
}

func generateSwaggerJSON() {
	version := "v0.1.0"
	if len(os.Args) > 1 {
		version = os.Args[1]
		if !strings.HasPrefix(version, "v") {
			version = "v" + version
		}
	}
	oAPIDefsAppsPub := appspub.GetOpenAPIDefinitions(func(name string) spec.Ref {
		return spec.MustCreateRef("#/definitions/" + common.EscapeJsonPointer(swaggify(name)))
	})
	oAPIDefsAppsV1alpha1 := appsv1alpha1.GetOpenAPIDefinitions(func(name string) spec.Ref {
		return spec.MustCreateRef("#/definitions/" + common.EscapeJsonPointer(swaggify(name)))
	})
	oAPIDefsAppsV1beta1 := appsv1beta1.GetOpenAPIDefinitions(func(name string) spec.Ref {
		return spec.MustCreateRef("#/definitions/" + common.EscapeJsonPointer(swaggify(name)))
	})
	defs := spec.Definitions{}
	for defName, val := range oAPIDefsAppsPub {
		defs[swaggify(defName)] = val.Schema
	}
	for defName, val := range oAPIDefsAppsV1alpha1 {
		defs[swaggify(defName)] = val.Schema
	}
	for defName, val := range oAPIDefsAppsV1beta1 {
		defs[swaggify(defName)] = val.Schema
	}
	swagger := spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Swagger:     "2.0",
			Definitions: defs,
			Paths:       &spec.Paths{Paths: map[string]spec.PathItem{}},
			Info: &spec.Info{
				InfoProps: spec.InfoProps{
					Title:   "Kruise",
					Version: version,
				},
			},
		},
	}
	jsonBytes, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		log.Fatal(err.Error())
	}
	fmt.Println(string(jsonBytes))
}

// swaggify converts the github package
// e.g.:
// github.com/openkruise/kruise/pkg/apis/apps/v1alpha1.SidecarSet
// to:
// kruise.apps.v1alpha1.SidecarSet
func swaggify(name string) string {
	name = strings.Replace(name, "github.com/openkruise/kruise/apis", "kruise", -1)
	parts := strings.Split(name, "/")
	hostParts := strings.Split(parts[0], ".")
	// reverses something like k8s.io to io.k8s
	for i, j := 0, len(hostParts)-1; i < j; i, j = i+1, j-1 {
		hostParts[i], hostParts[j] = hostParts[j], hostParts[i]
	}
	parts[0] = strings.Join(hostParts, ".")
	return strings.Join(parts, ".")
}
