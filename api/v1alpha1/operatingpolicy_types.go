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

// SignalTarget asks a signal's window mean to stay within ±TolerancePercent of Value.
type SignalTarget struct {
	// +kubebuilder:validation:MinLength=1
	Signal string `json:"signal"`

	Value float64 `json:"value"`

	// +kubebuilder:validation:Minimum=0
	TolerancePercent float64 `json:"tolerancePercent"`
}

// SignalConstraint asks a signal's window mean to stay within [Min, Max]. Either bound may be
// omitted.
type SignalConstraint struct {
	// +kubebuilder:validation:MinLength=1
	Signal string `json:"signal"`

	// +optional
	Min *float64 `json:"min,omitempty"`

	// +optional
	Max *float64 `json:"max,omitempty"`
}

// OperatingPolicySpec is one way of operating the plant: what J must not exceed, which signals
// must hit which targets, and which limits must hold.
type OperatingPolicySpec struct {
	// CostFunctionRef names a CostFunction in the same namespace.
	// +kubebuilder:validation:MinLength=1
	CostFunctionRef string `json:"costFunctionRef"`

	// Description is free text (e.g. "Downs & Vogel mode 1, base case").
	// +optional
	Description string `json:"description,omitempty"`

	// WindowSeconds is the averaging window requested from the historian.
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=1
	// +optional
	WindowSeconds int32 `json:"windowSeconds,omitempty"`

	// MaxCost is the budget for J. Omitted means J is observed but not judged.
	// +optional
	MaxCost *float64 `json:"maxCost,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=signal
	Targets []SignalTarget `json:"targets,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=signal
	Constraints []SignalConstraint `json:"constraints,omitempty"`

	// PersistenceEvaluations is how many consecutive failing evaluations are needed before the
	// plant is declared non-compliant (persistence rule from control-loop performance monitoring,
	// Bradu et al. 2018), so short transients don't flip the verdict.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	PersistenceEvaluations int32 `json:"persistenceEvaluations,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=op
// +kubebuilder:printcolumn:name="CostFunction",type=string,JSONPath=`.spec.costFunctionRef`
// +kubebuilder:printcolumn:name="MaxCost",type=number,JSONPath=`.spec.maxCost`
// +kubebuilder:printcolumn:name="Window",type=integer,JSONPath=`.spec.windowSeconds`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OperatingPolicy declares targets, constraints and a cost budget over a CostFunction.
type OperatingPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec OperatingPolicySpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OperatingPolicyList contains a list of OperatingPolicy.
type OperatingPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OperatingPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OperatingPolicy{}, &OperatingPolicyList{})
}
