package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TenantInfo struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	//tenant id
	//+kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	//tenant name
	//+kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	//tenant surname
	//+kubebuilder:validation:MinLength=1
	Surname string `json:"surname"`

	//tenant email
	//+kubebuilder:validation:MinLength=1
	Email string `json:"email"`

	//tenant group
	//+kubebuilder:validation:MinLength=1
	Group string `json:"group,omitempty"`

	//tenant PublickKey
	//+kubebuilder:validation:MinLength=1
	PublicKey string `json:"publicKey"`

	//user roles
	//+kubebuilder:validation:MinLength=1
	//+listType=set
	Roles []string `json:"roles"`
}

// TenantSpec defines the desired state of Tenant
// +k8s:openapi-gen=true
type TenantSpec struct {
	// +listType=set
	Hosts []string `json:"hosts,omitempty"`
}

// TenantStatus defines the observed state of Tenant
// +k8s:openapi-gen=true
type TenantStat struct {
	// +listType=set
	Hosts []string `json:"hosts,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Tenant is the Schema for the tenants API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:stat
// +kubebuilder:resource:path=tenants,scope=Namespaced
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec TenantSpec `json:"spec,omitempty"`
	Stat TenantStat `json:"stat,omitempty"`
	Info TenantInfo `json:"info,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TenantList contains a list of Tenant
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}
