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
	"strings"

	"github.com/Green-Cinnamon-Labs/plant-supervisor/api/v1alpha1"
	"github.com/Green-Cinnamon-Labs/plant-supervisor/internal/historian"
)

// LoopsResult is the second observation level for one evaluation.
type LoopsResult struct {
	Loops []v1alpha1.LoopStatus

	// Evaluated counts loops that passed the variability gate and had an index.
	Evaluated int

	// Violated is true when at least one evaluated loop is below its threshold.
	Violated bool
}

// EvaluateLoops judges each declared loop with two independent tests:
//
// Tuning — the two rules of Bradu et al. (2017):
//  1. the loop is only judged if its controller output varies more than MinOutputStd (a saturated,
//     idle or manual loop says nothing about tuning);
//  2. a judged loop is unhealthy when its Predictability Index is below MinPredictability.
//
// Tracking — the deviation alarm the index assumes exists elsewhere: when MaxOffset is set, the
// loop is unhealthy if |offset| exceeds it, whatever the gate says (a loop that lost its setpoint
// can be quiet or saturated). Experiment 25 showed why: under a disturbance the error drifts
// smoothly, the index goes to ~1 and calls the loop healthy while it is far from the setpoint.
//
// A loop is unhealthy if it fails either test. A loop with neither test applicable (no index,
// below the gate, no MaxOffset) is reported as not evaluated, never as unhealthy.
func EvaluateLoops(loops []v1alpha1.ControlLoop, perf map[string]historian.LoopPerformance) LoopsResult {
	var r LoopsResult
	for _, loop := range loops {
		st := v1alpha1.LoopStatus{Name: loop.Name, Healthy: true}
		p, ok := perf[loop.Name]
		if !ok {
			st.Reason = "NoResult"
			r.Loops = append(r.Loops, st)
			continue
		}
		st.Predictability, st.Offset, st.OutputStd = p.PI, p.Offset, p.SigmaOP

		var failures, skipped []string

		// Tuning test (Bradu)
		switch {
		case p.PI == nil:
			skip := "NoIndex"
			if p.Reason != nil {
				skip = "NoIndex: " + *p.Reason
			}
			skipped = append(skipped, skip)
		case p.SigmaOP == nil || *p.SigmaOP <= loop.MinOutputStd:
			skipped = append(skipped, "OutputBelowGate")
		default:
			st.Evaluated = true
			if *p.PI < loop.MinPredictability {
				failures = append(failures, "BelowThreshold")
			}
		}

		// Tracking test (deviation alarm)
		if loop.MaxOffset != nil && p.Offset != nil {
			st.Evaluated = true
			if math.Abs(*p.Offset) > *loop.MaxOffset {
				failures = append(failures, "OffsetExceeded")
			}
		}

		if st.Evaluated {
			r.Evaluated++
		}
		if len(failures) > 0 {
			st.Healthy = false
			st.Reason = strings.Join(failures, ", ")
			r.Violated = true
		} else if !st.Evaluated {
			st.Reason = strings.Join(skipped, ", ")
		}
		r.Loops = append(r.Loops, st)
	}
	return r
}

// LoopSpecs converts the policy's loops into the historian request.
func LoopSpecs(loops []v1alpha1.ControlLoop) []historian.LoopSpec {
	specs := make([]historian.LoopSpec, 0, len(loops))
	for _, l := range loops {
		specs = append(specs, historian.LoopSpec{
			Name: l.Name, PV: l.PV, SP: l.Setpoint, OP: l.OP, TimeConstantS: float64(l.TimeConstantSeconds),
		})
	}
	return specs
}
