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
	"testing"

	"github.com/Green-Cinnamon-Labs/plant-supervisor/api/v1alpha1"
	"github.com/Green-Cinnamon-Labs/plant-supervisor/internal/historian"
)

func perf(pi, sigmaOP float64) historian.LoopPerformance {
	return historian.LoopPerformance{PI: &pi, SigmaOP: &sigmaOP, Offset: f(0)}
}

func loop(name string) v1alpha1.ControlLoop {
	return v1alpha1.ControlLoop{Name: name, MinPredictability: 0.4, MinOutputStd: 0.1}
}

func TestLoopAboveThresholdIsHealthy(t *testing.T) {
	r := EvaluateLoops([]v1alpha1.ControlLoop{loop("a")}, map[string]historian.LoopPerformance{"a": perf(0.7, 0.5)})
	if r.Violated || r.Evaluated != 1 || !r.Loops[0].Healthy || !r.Loops[0].Evaluated {
		t.Fatalf("%+v", r)
	}
}

func TestLoopBelowThresholdIsUnhealthy(t *testing.T) {
	r := EvaluateLoops([]v1alpha1.ControlLoop{loop("a")}, map[string]historian.LoopPerformance{"a": perf(0.2, 0.5)})
	if !r.Violated || r.Loops[0].Healthy || r.Loops[0].Reason != "BelowThreshold" {
		t.Fatalf("%+v", r)
	}
}

func TestOutputBelowGateIsNotJudged(t *testing.T) {
	// PI ruim, mas a saída do controlador quase não varia: Bradu não julga essa malha
	r := EvaluateLoops([]v1alpha1.ControlLoop{loop("a")}, map[string]historian.LoopPerformance{"a": perf(0.05, 0.01)})
	if r.Violated || r.Evaluated != 0 || r.Loops[0].Evaluated || !r.Loops[0].Healthy || r.Loops[0].Reason != "OutputBelowGate" {
		t.Fatalf("%+v", r)
	}
}

func TestMissingIndexIsNotJudged(t *testing.T) {
	reason := "too_few_samples"
	p := historian.LoopPerformance{Reason: &reason, SigmaOP: f(1)}
	r := EvaluateLoops([]v1alpha1.ControlLoop{loop("a"), loop("b")}, map[string]historian.LoopPerformance{"a": p})
	if r.Violated || r.Evaluated != 0 {
		t.Fatalf("%+v", r)
	}
	if r.Loops[0].Reason != "NoIndex: too_few_samples" || r.Loops[1].Reason != "NoResult" {
		t.Fatalf("reasons: %q, %q", r.Loops[0].Reason, r.Loops[1].Reason)
	}
}

func TestOneUnhealthyLoopViolatesTheLevel(t *testing.T) {
	r := EvaluateLoops(
		[]v1alpha1.ControlLoop{loop("good"), loop("bad")},
		map[string]historian.LoopPerformance{"good": perf(0.9, 0.5), "bad": perf(0.1, 0.5)},
	)
	if !r.Violated || r.Evaluated != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestLoopSpecsCarrySetpointAndTimeConstant(t *testing.T) {
	specs := LoopSpecs([]v1alpha1.ControlLoop{{Name: "p", PV: "pv", OP: "op", Setpoint: 2705, TimeConstantSeconds: 30}})
	if len(specs) != 1 || specs[0].SP != 2705 || specs[0].TimeConstantS != 30 || specs[0].PV != "pv" {
		t.Fatalf("%+v", specs)
	}
}
