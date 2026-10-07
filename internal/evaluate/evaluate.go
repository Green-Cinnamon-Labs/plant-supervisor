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

// Package evaluate turns window means of plant signals into a verdict about an OperatingPolicy.
//
// Pure functions only: no Kubernetes client, no HTTP. The reconciler fetches the objects and the
// historian aggregates, and this package does the arithmetic, so it can be tested in isolation.
//
// Known approximation: a product term uses mean(a) × mean(b), not mean(a × b). The two agree when
// the signals are steady over the window, which is the regime an economic cost describes.
package evaluate

import (
	"math"
	"sort"

	"github.com/Green-Cinnamon-Labs/plant-supervisor/api/v1alpha1"
)

// Result is the instantaneous (non-persistence-filtered) evaluation of one window.
type Result struct {
	Cost        float64
	Terms       []v1alpha1.TermContribution
	Targets     []v1alpha1.TargetResult
	Constraints []v1alpha1.ConstraintResult

	CostWithinBudget     bool
	TargetsMet           bool
	ConstraintsSatisfied bool
}

// Violated reports whether this evaluation fails the policy.
func (r Result) Violated() bool {
	return !(r.CostWithinBudget && r.TargetsMet && r.ConstraintsSatisfied)
}

// RequiredSignals lists every signal the cost function and the policy read, sorted and unique.
func RequiredSignals(cf v1alpha1.CostFunctionSpec, policy v1alpha1.OperatingPolicySpec) []string {
	set := map[string]struct{}{}
	for _, term := range cf.Terms {
		for _, s := range term.Signals {
			set[s] = struct{}{}
		}
	}
	for _, t := range policy.Targets {
		set[t.Signal] = struct{}{}
	}
	for _, c := range policy.Constraints {
		set[c.Signal] = struct{}{}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// MissingSignals returns the required signals absent from means.
func MissingSignals(required []string, means map[string]float64) []string {
	var missing []string
	for _, k := range required {
		if _, ok := means[k]; !ok {
			missing = append(missing, k)
		}
	}
	return missing
}

// Evaluate computes J and checks the policy. Every required signal must be present in means
// (check with MissingSignals first); an absent signal counts as 0 in J and fails its target or
// constraint.
func Evaluate(cf v1alpha1.CostFunctionSpec, policy v1alpha1.OperatingPolicySpec, means map[string]float64) Result {
	r := Result{CostWithinBudget: true, TargetsMet: true, ConstraintsSatisfied: true}

	for _, term := range cf.Terms {
		value := term.Coefficient
		for _, s := range term.Signals {
			value *= means[s]
		}
		r.Cost += value
		r.Terms = append(r.Terms, v1alpha1.TermContribution{Name: term.Name, Value: value})
	}
	if policy.MaxCost != nil && r.Cost > *policy.MaxCost {
		r.CostWithinBudget = false
	}

	for _, t := range policy.Targets {
		res := v1alpha1.TargetResult{Signal: t.Signal, Target: t.Value}
		if v, ok := means[t.Signal]; ok {
			res.Observed = ptr(v)
			res.Met = math.Abs(v-t.Value) <= math.Abs(t.Value)*t.TolerancePercent/100
		}
		if !res.Met {
			r.TargetsMet = false
		}
		r.Targets = append(r.Targets, res)
	}

	for _, c := range policy.Constraints {
		res := v1alpha1.ConstraintResult{Signal: c.Signal}
		if v, ok := means[c.Signal]; ok {
			res.Observed = ptr(v)
			res.Satisfied = (c.Min == nil || v >= *c.Min) && (c.Max == nil || v <= *c.Max)
		}
		if !res.Satisfied {
			r.ConstraintsSatisfied = false
		}
		r.Constraints = append(r.Constraints, res)
	}

	return r
}

// NextViolations advances the consecutive-violation counter. The counter restarts when the
// active policy changed, since violations of a previous policy say nothing about the new one.
func NextViolations(previous int32, violated, policyChanged bool) int32 {
	if policyChanged {
		previous = 0
	}
	if !violated {
		return 0
	}
	return previous + 1
}

// NonCompliant applies the persistence rule: the verdict flips only after `persistence`
// consecutive failing evaluations.
func NonCompliant(violations, persistence int32) bool {
	if persistence < 1 {
		persistence = 1
	}
	return violations >= persistence
}

func ptr(v float64) *float64 { return &v }
