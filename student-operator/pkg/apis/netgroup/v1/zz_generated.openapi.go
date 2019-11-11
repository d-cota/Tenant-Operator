// +build !ignore_autogenerated

// This file was autogenerated by openapi-gen. Do not edit it manually!

package v1

import (
	spec "github.com/go-openapi/spec"
	common "k8s.io/kube-openapi/pkg/common"
)

func GetOpenAPIDefinitions(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return map[string]common.OpenAPIDefinition{
		"./pkg/apis/netgroup/v1.StudentAPI":       schema_pkg_apis_netgroup_v1_StudentAPI(ref),
		"./pkg/apis/netgroup/v1.StudentAPISpec":   schema_pkg_apis_netgroup_v1_StudentAPISpec(ref),
		"./pkg/apis/netgroup/v1.StudentAPIStatus": schema_pkg_apis_netgroup_v1_StudentAPIStatus(ref),
	}
}

func schema_pkg_apis_netgroup_v1_StudentAPI(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "StudentAPI is the Schema for the studentapis API",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"),
						},
					},
					"spec": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("./pkg/apis/netgroup/v1.StudentAPISpec"),
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("./pkg/apis/netgroup/v1.StudentAPIStatus"),
						},
					},
				},
			},
		},
		Dependencies: []string{
			"./pkg/apis/netgroup/v1.StudentAPISpec", "./pkg/apis/netgroup/v1.StudentAPIStatus", "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"},
	}
}

func schema_pkg_apis_netgroup_v1_StudentAPISpec(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "StudentAPISpec defines the desired state of StudentAPI",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"id": {
						SchemaProps: spec.SchemaProps{
							Description: "student id",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"name": {
						SchemaProps: spec.SchemaProps{
							Description: "student name",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"surname": {
						SchemaProps: spec.SchemaProps{
							Description: "student surname",
							Type:        []string{"string"},
							Format:      "",
						},
					},
				},
				Required: []string{"id", "name", "surname"},
			},
		},
	}
}

func schema_pkg_apis_netgroup_v1_StudentAPIStatus(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "StudentAPIStatus defines the observed state of StudentAPI",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"nodes": {
						VendorExtensible: spec.VendorExtensible{
							Extensions: spec.Extensions{
								"x-kubernetes-list-type": "set",
							},
						},
						SchemaProps: spec.SchemaProps{
							Type: []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Type:   []string{"string"},
										Format: "",
									},
								},
							},
						},
					},
				},
				Required: []string{"nodes"},
			},
		},
	}
}
