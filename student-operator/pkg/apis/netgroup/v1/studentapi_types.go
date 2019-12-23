package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StudentAPIInfo defines the general info of the user
// +k8s:openapi-gen=true
type StudentAPIInfo struct {
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

	//student PublickKey
	//+kubebuilder:validation:MinLength=1
	PublicKey string `json:"publicKey"`

	//user roles
	//+kubebuilder:validation:MinLength=1
	//+listType=set  
	Roles []string `json:"roles"`
}

// StudentAPISpec defines the desired state of the object
// +k8s:openapi-gen=true
type StudentAPISpec struct {
	// +listType=set
	Servers []string `json:"servers,omitempty"`
}

// StudentAPIStatus defines the observed state of the object
// +k8s:openapi-gen=true
type StudentAPIStatus struct {
	// +listType=set
	Servers []string `json:"servers,omitempty"`
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
	Info StudentAPIInfo 	`json:"info,omitempty"`
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
