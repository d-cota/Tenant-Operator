package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// StudentAPISpec defines the desired state of StudentAPI
// +k8s:openapi-gen=true
type StudentAPISpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	//student id
	//+kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	//student name
	//+kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	//student surname
	//+kubebuilder:validation:MinLength=1
	Surname string `json:"surname"`
}

// StudentAPIStatus defines the observed state of StudentAPI
// +k8s:openapi-gen=true
type StudentAPIStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// +listType=set
	Nodes []string `json:"nodes"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StudentAPI is the Schema for the studentapis API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=studentapis,scope=Namespaced
type StudentAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StudentAPISpec   `json:"spec,omitempty"`
	Status StudentAPIStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StudentAPIList contains a list of StudentAPI
type StudentAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StudentAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StudentAPI{}, &StudentAPIList{})
}
