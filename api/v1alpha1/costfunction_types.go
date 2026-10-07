/*
Copyright 2026.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CostTerm is one additive term of the cost function: coefficient × Π mean(signal).
//
// A term with one signal is linear (e.g. price × compressor power). A term with two signals is a
// product (e.g. price × flow × composition). Unit conversions are folded into the coefficient.
type CostTerm struct {
	// Name identifies the term in status (e.g. "purge.A").
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Coefficient multiplies the product of the signals' window means.
	Coefficient float64 `json:"coefficient"`

	// Signals are historian keys (OPC-UA browse names, e.g. "xmeas.compressor.work").
	// +kubebuilder:validation:MinItems=1
	Signals []string `json:"signals"`

	// Description is free text, typically how the coefficient was derived.
	// +optional
	Description string `json:"description,omitempty"`
}

// CostFunctionSpec declares the objective function J = Σ terms.
type CostFunctionSpec struct {
	// Unit of J (e.g. "$/h").
	// +kubebuilder:validation:MinLength=1
	Unit string `json:"unit"`

	// Terms summed to compute J.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=name
	Terms []CostTerm `json:"terms"`

	// Reference is J at nominal operation, used for comparison in status.
	// +optional
	Reference *float64 `json:"reference,omitempty"`

	// Source cites where the function comes from (paper, table).
	// +optional
	Source string `json:"source,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=cf
// +kubebuilder:printcolumn:name="Unit",type=string,JSONPath=`.spec.unit`
// +kubebuilder:printcolumn:name="Reference",type=number,JSONPath=`.spec.reference`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CostFunction is a declared objective function over plant signals.
type CostFunction struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec CostFunctionSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// CostFunctionList contains a list of CostFunction.
type CostFunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CostFunction `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CostFunction{}, &CostFunctionList{})
}
