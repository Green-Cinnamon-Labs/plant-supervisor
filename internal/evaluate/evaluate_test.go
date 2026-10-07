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

package evaluate

import (
	"math"
	"testing"

	"github.com/Green-Cinnamon-Labs/tep-operator/api/v1alpha1"
)

// downsVogel builds the Downs & Vogel (1993) Table 9 operating cost as declared terms. The
// operator knows nothing about TEP; this is only test data, mirroring the manifest that lives in
// tep-supervisor.
//
//	J = Σ cost_i · (x_purge_i/100) · 44.79 · F_purge      (purge, kscmh → kgmol/h)
//	  + Σ cost_i · (x_prod_i/100)  · 9.21  · F_product    (product D, E, F; m³/h → kgmol/h)
//	  + 0.0536 · W_compressor + 0.0318 · F_steam
func downsVogel() v1alpha1.CostFunctionSpec {
	const purgeKgmolPerKscmh, productKgmolPerM3 = 44.79, 9.21
	purge := map[string]float64{"a": 2.206, "c": 6.177, "d": 22.06, "e": 14.56, "f": 17.89, "g": 30.44, "h": 22.94}
	product := map[string]float64{"d": 22.06, "e": 14.56, "f": 17.89}

	var terms []v1alpha1.CostTerm
	for _, c := range []string{"a", "c", "d", "e", "f", "g", "h"} {
		terms = append(terms, v1alpha1.CostTerm{
			Name:        "purge." + c,
			Coefficient: purge[c] * purgeKgmolPerKscmh / 100,
			Signals:     []string{"xmeas.stream9.flow_rate", "xmeas.stream9.component." + c},
		})
	}
	for _, c := range []string{"d", "e", "f"} {
		terms = append(terms, v1alpha1.CostTerm{
			Name:        "product." + c,
			Coefficient: product[c] * productKgmolPerM3 / 100,
			Signals:     []string{"xmeas.stream11.flow_rate", "xmeas.stream11.component." + c},
		})
	}
	terms = append(terms,
		v1alpha1.CostTerm{Name: "compressor", Coefficient: 0.0536, Signals: []string{"xmeas.compressor.work"}},
		v1alpha1.CostTerm{Name: "steam", Coefficient: 0.0318, Signals: []string{"xmeas.stripper.steam_flow_rate"}},
	)
	return v1alpha1.CostFunctionSpec{Unit: "$/h", Terms: terms}
}

// baseCase holds the values Table 9 itself uses for its sample calculation (purge and product
// mole fractions in mol%, purge 0.3371 kscmh, product 22.95 m³/h, 341.4 kW, 230.3 kg/h).
func baseCase() map[string]float64 {
	return map[string]float64{
		"xmeas.stream9.flow_rate":        0.3371,
		"xmeas.stream9.component.a":      32.958,
		"xmeas.stream9.component.c":      23.978,
		"xmeas.stream9.component.d":      1.257,
		"xmeas.stream9.component.e":      18.579,
		"xmeas.stream9.component.f":      2.263,
		"xmeas.stream9.component.g":      4.844,
		"xmeas.stream9.component.h":      2.299,
		"xmeas.stream11.flow_rate":       22.95,
		"xmeas.stream11.component.d":     0.018,
		"xmeas.stream11.component.e":     0.836,
		"xmeas.stream11.component.f":     0.099,
		"xmeas.compressor.work":          341.4,
		"xmeas.stripper.steam_flow_rate": 230.3,
		"xmeas.reactor.pressure":         2705.0,
	}
}

func f(v float64) *float64 { return &v }

func TestDownsVogelBaseCaseCost(t *testing.T) {
	r := Evaluate(downsVogel(), v1alpha1.OperatingPolicySpec{}, baseCase())

	if math.Abs(r.Cost-170.6) > 0.1 {
		t.Fatalf("J = %.3f $/h, want 170.6 ± 0.1 (Downs & Vogel 1993, Table 9)", r.Cost)
	}
	if len(r.Terms) != 12 {
		t.Fatalf("got %d terms, want 12", len(r.Terms))
	}
	if r.Violated() {
		t.Fatal("empty policy must not be violated")
	}
}

func TestBudgetTargetsAndConstraints(t *testing.T) {
	means := baseCase()
	policy := v1alpha1.OperatingPolicySpec{
		MaxCost: f(180),
		Targets: []v1alpha1.SignalTarget{
			{Signal: "xmeas.stream11.flow_rate", Value: 22.949, TolerancePercent: 5},
		},
		Constraints: []v1alpha1.SignalConstraint{
			{Signal: "xmeas.reactor.pressure", Max: f(2895)},
		},
	}

	r := Evaluate(downsVogel(), policy, means)
	if r.Violated() {
		t.Fatalf("base case should comply: %+v", r)
	}

	means["xmeas.reactor.pressure"] = 2900
	means["xmeas.stream11.flow_rate"] = 20 // −13 %, outside ±5 %
	r = Evaluate(downsVogel(), policy, means)
	if r.ConstraintsSatisfied || r.TargetsMet {
		t.Fatalf("expected constraint and target failures: %+v", r)
	}

	policy.MaxCost = f(100)
	if Evaluate(downsVogel(), policy, baseCase()).CostWithinBudget {
		t.Fatal("170.6 must exceed a 100 budget")
	}
}

func TestConstraintBoundsAreOptional(t *testing.T) {
	policy := v1alpha1.OperatingPolicySpec{
		Constraints: []v1alpha1.SignalConstraint{{Signal: "x", Min: f(50)}},
	}
	cf := v1alpha1.CostFunctionSpec{Terms: []v1alpha1.CostTerm{{Name: "t", Coefficient: 1, Signals: []string{"x"}}}}

	if Evaluate(cf, policy, map[string]float64{"x": 1e9}).Violated() {
		t.Fatal("no max → any large value satisfies")
	}
	if !Evaluate(cf, policy, map[string]float64{"x": 49}).Violated() {
		t.Fatal("below min must violate")
	}
}

func TestRequiredAndMissingSignals(t *testing.T) {
	policy := v1alpha1.OperatingPolicySpec{
		Targets:     []v1alpha1.SignalTarget{{Signal: "xmeas.stream11.flow_rate"}},
		Constraints: []v1alpha1.SignalConstraint{{Signal: "xmeas.reactor.pressure"}},
	}
	req := RequiredSignals(downsVogel(), policy)
	if len(req) != 15 { // 2 flows + 7 purge + 3 product comps + compressor + steam + pressure
		t.Fatalf("got %d required signals: %v", len(req), req)
	}
	if m := MissingSignals(req, baseCase()); len(m) != 0 {
		t.Fatalf("unexpected missing: %v", m)
	}
	means := baseCase()
	delete(means, "xmeas.compressor.work")
	if m := MissingSignals(req, means); len(m) != 1 || m[0] != "xmeas.compressor.work" {
		t.Fatalf("missing = %v", m)
	}
}

func TestPersistence(t *testing.T) {
	var v int32
	for _, violated := range []bool{true, true} {
		v = NextViolations(v, violated, false)
	}
	if NonCompliant(v, 3) {
		t.Fatal("2 violations < persistence 3")
	}
	v = NextViolations(v, true, false)
	if !NonCompliant(v, 3) {
		t.Fatal("3 violations reach persistence 3")
	}
	if NextViolations(v, false, false) != 0 {
		t.Fatal("a passing evaluation resets the counter")
	}
	if NextViolations(v, true, true) != 1 {
		t.Fatal("a policy change restarts the counter")
	}
	if !NonCompliant(1, 0) {
		t.Fatal("persistence below 1 behaves as 1")
	}
}
