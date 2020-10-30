/*


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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TcpIngressSpec defines the desired state of TcpIngress
type TcpIngressSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of TcpIngress. Edit TcpIngress_types.go to remove/update
	//Foo string `json:"foo,omitempty"`

	// Specifies the name of the referenced service.
	ServiceName string        `json:"serviceName"`
	Ports       []ServicePort `json:"ports"`
}

type ServicePort struct {
	ServicePort  int32 `json:"servicePort"`
	ExternalPort int32 `json:"externalPort"`
}

// TcpIngressStatus defines the observed state of TcpIngress
type TcpIngressStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	LoadBalancer corev1.LoadBalancerStatus `json:"loadBalancer,omitempty"`
}

// +kubebuilder:object:root=true

// TcpIngress is the Schema for the tcpingresses API
type TcpIngress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TcpIngressSpec   `json:"spec,omitempty"`
	Status TcpIngressStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TcpIngressList contains a list of TcpIngress
type TcpIngressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TcpIngress `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TcpIngress{}, &TcpIngressList{})
}
