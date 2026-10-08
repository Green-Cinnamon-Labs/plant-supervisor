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

// PlantSpec points the operator at a plant's historian and at the policy currently in force.
type PlantSpec struct {
	// HistorianURL is the base URL of the historian (e.g. "http://tep-historian:8090").
	// +kubebuilder:validation:MinLength=1
	HistorianURL string `json:"historianURL"`

	// PolicyRef names the active OperatingPolicy in the same namespace.
	// +kubebuilder:validation:MinLength=1
	PolicyRef string `json:"policyRef"`

	// EvaluationIntervalSeconds is how often the plant is evaluated.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +optional
	EvaluationIntervalSeconds int32 `json:"evaluationIntervalSeconds,omitempty"`
}

// PlantPhase summarizes the verdict.
// +kubebuilder:validation:Enum=Pending;Compliant;NonCompliant
type PlantPhase string

const (
	// PhasePending means there is no verdict yet: missing policy, unreachable historian or
	// missing signals.
	PhasePending PlantPhase = "Pending"
	// PhaseCompliant means the plant satisfies the active policy.
	PhaseCompliant PlantPhase = "Compliant"
	// PhaseNonCompliant means the plant has failed the policy for PersistenceEvaluations
	// consecutive evaluations.
	PhaseNonCompliant PlantPhase = "NonCompliant"
)

// Condition types reported in PlantStatus.Conditions.
const (
	ConditionDataAvailable        = "DataAvailable"
	ConditionCostWithinBudget     = "CostWithinBudget"
	ConditionTargetsMet           = "TargetsMet"
	ConditionConstraintsSatisfied = "ConstraintsSatisfied"
	ConditionPolicyCompliant      = "PolicyCompliant"
	// ConditionControlLoopsHealthy is the second observation level: control-loop quality. It is
	// set only when the active policy declares control loops, and never affects Phase.
	ConditionControlLoopsHealthy = "ControlLoopsHealthy"
)

// CostStatus is the evaluated value of J.
type CostStatus struct {
	Value float64 `json:"value"`

	Unit string `json:"unit"`

	// +optional
	Reference *float64 `json:"reference,omitempty"`

	// +optional
	MaxCost *float64 `json:"maxCost,omitempty"`
}

// TermContribution is one term's share of J.
type TermContribution struct {
	Name string `json:"name"`

	Value float64 `json:"value"`
}

// TargetResult is the outcome of one SignalTarget.
type TargetResult struct {
	Signal string `json:"signal"`

	Target float64 `json:"target"`

	// +optional
	Observed *float64 `json:"observed,omitempty"`

	Met bool `json:"met"`
}

// ConstraintResult is the outcome of one SignalConstraint.
type ConstraintResult struct {
	Signal string `json:"signal"`

	// +optional
	Observed *float64 `json:"observed,omitempty"`

	Satisfied bool `json:"satisfied"`
}

// LoopStatus is the quality of one control loop in the last evaluation.
type LoopStatus struct {
	Name string `json:"name"`

	// Predictability is the Predictability Index PI (0–1) of the loop error's fluctuation.
	// +optional
	Predictability *float64 `json:"predictability,omitempty"`

	// Offset is the mean loop error SP − PV (a proportional controller keeps one).
	// +optional
	Offset *float64 `json:"offset,omitempty"`

	// OutputStd is the standard deviation of the controller output over the window.
	// +optional
	OutputStd *float64 `json:"outputStd,omitempty"`

	// Evaluated is false when the loop could not be judged: no index (e.g. too few samples) or
	// controller output below the variability gate.
	Evaluated bool `json:"evaluated"`

	// Healthy is false only for an evaluated loop with Predictability below its threshold.
	Healthy bool `json:"healthy"`

	// Reason explains Evaluated=false or Healthy=false.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// PlantStatus is the supervisory layer's verdict about the plant.
type PlantStatus struct {
	// +optional
	Phase PlantPhase `json:"phase,omitempty"`

	// ActivePolicy is the policy the verdict refers to.
	// +optional
	ActivePolicy string `json:"activePolicy,omitempty"`

	// +optional
	Cost *CostStatus `json:"cost,omitempty"`

	// +optional
	Terms []TermContribution `json:"terms,omitempty"`

	// +optional
	Targets []TargetResult `json:"targets,omitempty"`

	// +optional
	Constraints []ConstraintResult `json:"constraints,omitempty"`

	// ConsecutiveViolations counts consecutive failing evaluations under ActivePolicy.
	// +optional
	ConsecutiveViolations int32 `json:"consecutiveViolations,omitempty"`

	// Loops is the per-loop result of the second observation level.
	// +optional
	Loops []LoopStatus `json:"loops,omitempty"`

	// ConsecutiveLoopViolations counts consecutive evaluations with at least one unhealthy loop.
	// +optional
	ConsecutiveLoopViolations int32 `json:"consecutiveLoopViolations,omitempty"`

	// +optional
	LastEvaluationTime *metav1.Time `json:"lastEvaluationTime,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Policy",type=string,JSONPath=`.status.activePolicy`
// +kubebuilder:printcolumn:name="Cost",type=number,JSONPath=`.status.cost.value`
// +kubebuilder:printcolumn:name="Unit",type=string,JSONPath=`.status.cost.unit`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Loops",type=string,JSONPath=`.status.conditions[?(@.type=="ControlLoopsHealthy")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Plant is an industrial plant observed by the supervisory layer.
type Plant struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PlantSpec `json:"spec"`

	// +optional
	Status PlantStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PlantList contains a list of Plant.
type PlantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Plant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Plant{}, &PlantList{})
}
