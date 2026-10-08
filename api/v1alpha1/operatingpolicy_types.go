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

// ControlLoop declares one control loop whose quality is monitored with the Predictability Index
// of Bradu et al. (2017): how much of the loop error SP − PV an autoregressive model can predict.
// The historian computes the index; the supervisor judges it against the thresholds below.
type ControlLoop struct {
	// Name identifies the loop in status (e.g. "reactor_pressure").
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// PV is the historian key of the measured variable.
	// +kubebuilder:validation:MinLength=1
	PV string `json:"pv"`

	// Setpoint of the loop, in the PV's unit.
	Setpoint float64 `json:"setpoint"`

	// OP is the historian key of the controller output (e.g. the valve position).
	// +kubebuilder:validation:MinLength=1
	OP string `json:"op"`

	// TimeConstantSeconds is the closed-loop settling time T; it sets the prediction horizon
	// b = ceil(T / t_s).
	// +kubebuilder:validation:Minimum=1
	TimeConstantSeconds int32 `json:"timeConstantSeconds"`

	// MinPredictability is the threshold PI_L: below it the loop is considered poorly tuned.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	MinPredictability float64 `json:"minPredictability"`

	// MinOutputStd is the variability gate σ̄_y: the loop is only judged when the standard
	// deviation of its controller output is above it (a saturated, idle or manual loop is not).
	// +kubebuilder:validation:Minimum=0
	// +optional
	MinOutputStd float64 `json:"minOutputStd,omitempty"`
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

	// ControlLoops are judged separately from the economic verdict (condition ControlLoopsHealthy,
	// never PolicyCompliant): the two observation levels stay side by side.
	// +optional
	// +listType=map
	// +listMapKey=name
	ControlLoops []ControlLoop `json:"controlLoops,omitempty"`

	// LoopWindowSeconds is the time window t_W over which each loop's index is computed.
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=1
	// +optional
	LoopWindowSeconds int32 `json:"loopWindowSeconds,omitempty"`

	// LoopSampleIntervalSeconds is the sampling time t_s the historian resamples the series to.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	LoopSampleIntervalSeconds int32 `json:"loopSampleIntervalSeconds,omitempty"`

	// LoopPersistenceEvaluations is how many consecutive evaluations with an unhealthy loop are
	// needed before ControlLoopsHealthy turns False (Bradu's N).
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +optional
	LoopPersistenceEvaluations int32 `json:"loopPersistenceEvaluations,omitempty"`
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
