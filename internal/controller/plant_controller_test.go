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

package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Green-Cinnamon-Labs/plant-supervisor/api/v1alpha1"
)

// fakeHistorian is a real HTTP server speaking tep-historian's API, so the controller tests also
// exercise the real historian client (request encoding, response decoding, error handling).
type fakeHistorian struct {
	mu        sync.Mutex
	means     map[string]float64
	connected bool
	loops     map[string]map[string]any // historian "loops" entries, by loop name
	fail      bool                      // answer 500 to every request
	failLoops bool                      // answer 500 to /loop-performance only
}

func (f *fakeHistorian) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail || (f.failLoops && r.URL.Path == "/loop-performance") {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch r.URL.Path {
	case "/aggregate":
		var req struct {
			Keys []string `json:"keys"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		signals := map[string]any{}
		var missing []string
		for _, k := range req.Keys {
			if v, ok := f.means[k]; ok {
				signals[k] = map[string]any{"count": 10, "mean": v}
			} else {
				missing = append(missing, k)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"connected": f.connected, "signals": signals, "missing": missing})
	case "/loop-performance":
		_ = json.NewEncoder(w).Encode(map[string]any{"connected": f.connected, "loops": f.loops})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeHistorian) set(fn func(*fakeHistorian)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func fp(v float64) *float64 { return &v }

func loopPerf(pi, sigmaOP float64) map[string]any {
	return map[string]any{"pi": pi, "sigma_op": sigmaOP, "offset": 0.0, "n": 300, "b": 30, "m": 60}
}

var _ = Describe("Plant Controller", func() {
	const ns = "default"
	var (
		hist       *fakeHistorian
		server     *httptest.Server
		reconciler *PlantReconciler
		key        = types.NamespacedName{Namespace: ns, Name: "plant-under-test"}
	)

	reconcileOnce := func() v1alpha1.Plant {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		var p v1alpha1.Plant
		Expect(k8sClient.Get(ctx, key, &p)).To(Succeed())
		return p
	}
	condition := func(p v1alpha1.Plant, t string) metav1.ConditionStatus {
		c := meta.FindStatusCondition(p.Status.Conditions, t)
		Expect(c).NotTo(BeNil(), "condition %s", t)
		return c.Status
	}
	conditionReason := func(p v1alpha1.Plant, t string) string {
		c := meta.FindStatusCondition(p.Status.Conditions, t)
		Expect(c).NotTo(BeNil(), "condition %s", t)
		return c.Reason
	}
	withLoops := func() {
		var policy v1alpha1.OperatingPolicy
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "policy"}, &policy)).To(Succeed())
		policy.Spec.ControlLoops = []v1alpha1.ControlLoop{
			{Name: "level", PV: "level", Setpoint: 50, OP: "valve", TimeConstantSeconds: 30, MinPredictability: 0.4, MinOutputStd: 0.1},
		}
		policy.Spec.LoopPersistenceEvaluations = 2
		Expect(k8sClient.Update(ctx, &policy)).To(Succeed())
	}

	BeforeEach(func() {
		hist = &fakeHistorian{
			connected: true,
			means:     map[string]float64{"power": 100, "level": 50, "pressure": 200},
			loops:     map[string]map[string]any{"level": loopPerf(0.7, 0.5)},
		}
		server = httptest.NewServer(hist)
		reconciler = &PlantReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

		Expect(k8sClient.Create(ctx, &v1alpha1.CostFunction{
			ObjectMeta: metav1.ObjectMeta{Name: "cf", Namespace: ns},
			Spec: v1alpha1.CostFunctionSpec{Unit: "$/h", Terms: []v1alpha1.CostTerm{
				{Name: "power", Coefficient: 0.1, Signals: []string{"power"}},
			}},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &v1alpha1.OperatingPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "policy", Namespace: ns},
			Spec: v1alpha1.OperatingPolicySpec{
				CostFunctionRef:        "cf",
				MaxCost:                fp(15),
				PersistenceEvaluations: 2,
				Targets:                []v1alpha1.SignalTarget{{Signal: "level", Value: 50, TolerancePercent: 10}},
				Constraints:            []v1alpha1.SignalConstraint{{Signal: "pressure", Max: fp(250)}},
			},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &v1alpha1.Plant{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: ns},
			Spec:       v1alpha1.PlantSpec{HistorianURL: server.URL, PolicyRef: "policy"},
		})).To(Succeed())
	})

	AfterEach(func() {
		server.Close()
		Expect(k8sClient.Delete(ctx, &v1alpha1.Plant{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: ns}})).To(Succeed())
		Expect(k8sClient.Delete(ctx, &v1alpha1.OperatingPolicy{ObjectMeta: metav1.ObjectMeta{Name: "policy", Namespace: ns}})).To(Succeed())
		Expect(k8sClient.Delete(ctx, &v1alpha1.CostFunction{ObjectMeta: metav1.ObjectMeta{Name: "cf", Namespace: ns}})).To(Succeed())
	})

	Context("economic level", func() {
		It("reports J and a compliant verdict when every check passes", func() {
			p := reconcileOnce()

			Expect(p.Status.Phase).To(Equal(v1alpha1.PhaseCompliant))
			Expect(p.Status.ActivePolicy).To(Equal("policy"))
			Expect(p.Status.Cost.Value).To(BeNumerically("~", 10.0, 1e-9))
			Expect(p.Status.Terms).To(HaveLen(1))
			Expect(condition(p, v1alpha1.ConditionPolicyCompliant)).To(Equal(metav1.ConditionTrue))
			Expect(condition(p, v1alpha1.ConditionDataAvailable)).To(Equal(metav1.ConditionTrue))
			Expect(meta.FindStatusCondition(p.Status.Conditions, v1alpha1.ConditionControlLoopsHealthy)).To(BeNil(),
				"no loops declared → no loop condition")
		})

		It("flips to NonCompliant only after the persistence count", func() {
			hist.set(func(h *fakeHistorian) { h.means["pressure"] = 300 }) // violates max 250

			p := reconcileOnce()
			Expect(condition(p, v1alpha1.ConditionConstraintsSatisfied)).To(Equal(metav1.ConditionFalse))
			Expect(p.Status.Phase).To(Equal(v1alpha1.PhaseCompliant), "1 violation < persistence 2")
			Expect(p.Status.ConsecutiveViolations).To(Equal(int32(1)))

			p = reconcileOnce()
			Expect(p.Status.Phase).To(Equal(v1alpha1.PhaseNonCompliant))
			Expect(condition(p, v1alpha1.ConditionPolicyCompliant)).To(Equal(metav1.ConditionFalse))

			hist.set(func(h *fakeHistorian) { h.means["pressure"] = 200 })
			p = reconcileOnce()
			Expect(p.Status.Phase).To(Equal(v1alpha1.PhaseCompliant))
			Expect(p.Status.ConsecutiveViolations).To(BeZero())
		})

		It("flags cost over budget", func() {
			hist.set(func(h *fakeHistorian) { h.means["power"] = 200 }) // J = 20 > 15

			p := reconcileOnce()
			Expect(condition(p, v1alpha1.ConditionCostWithinBudget)).To(Equal(metav1.ConditionFalse))
		})

		It("stays Pending when the historian is unreachable", func() {
			hist.set(func(h *fakeHistorian) { h.fail = true })

			p := reconcileOnce()
			Expect(p.Status.Phase).To(Equal(v1alpha1.PhasePending))
			Expect(condition(p, v1alpha1.ConditionDataAvailable)).To(Equal(metav1.ConditionFalse))
			Expect(conditionReason(p, v1alpha1.ConditionDataAvailable)).To(Equal("HistorianUnreachable"))
			Expect(condition(p, v1alpha1.ConditionPolicyCompliant)).To(Equal(metav1.ConditionUnknown))
		})

		It("stays Pending when a required signal has no samples", func() {
			hist.set(func(h *fakeHistorian) { delete(h.means, "level") })

			p := reconcileOnce()
			Expect(p.Status.Phase).To(Equal(v1alpha1.PhasePending))
			c := meta.FindStatusCondition(p.Status.Conditions, v1alpha1.ConditionDataAvailable)
			Expect(c.Reason).To(Equal("MissingSignals"))
			Expect(c.Message).To(ContainSubstring("level"))
		})

		It("stays Pending when the policy does not exist", func() {
			var p v1alpha1.Plant
			Expect(k8sClient.Get(ctx, key, &p)).To(Succeed())
			p.Spec.PolicyRef = "nope"
			Expect(k8sClient.Update(ctx, &p)).To(Succeed())

			p = reconcileOnce()
			Expect(p.Status.Phase).To(Equal(v1alpha1.PhasePending))
			Expect(conditionReason(p, v1alpha1.ConditionDataAvailable)).To(Equal("PolicyNotFound"))
		})
	})

	Context("control-loop level", func() {
		BeforeEach(withLoops)

		It("reports each loop and a healthy condition", func() {
			p := reconcileOnce()

			Expect(condition(p, v1alpha1.ConditionControlLoopsHealthy)).To(Equal(metav1.ConditionTrue))
			Expect(p.Status.Loops).To(HaveLen(1))
			Expect(*p.Status.Loops[0].Predictability).To(BeNumerically("~", 0.7, 1e-9))
			Expect(p.Status.Loops[0].Evaluated).To(BeTrue())
		})

		It("turns ControlLoopsHealthy False after the loop persistence, without touching the phase", func() {
			hist.set(func(h *fakeHistorian) { h.loops["level"] = loopPerf(0.1, 0.5) })

			p := reconcileOnce()
			Expect(condition(p, v1alpha1.ConditionControlLoopsHealthy)).To(Equal(metav1.ConditionTrue), "1 < persistence 2")
			Expect(p.Status.ConsecutiveLoopViolations).To(Equal(int32(1)))

			p = reconcileOnce()
			Expect(condition(p, v1alpha1.ConditionControlLoopsHealthy)).To(Equal(metav1.ConditionFalse))
			Expect(conditionReason(p, v1alpha1.ConditionControlLoopsHealthy)).To(Equal("LoopsDegraded"))
			Expect(p.Status.Phase).To(Equal(v1alpha1.PhaseCompliant), "the two observation levels stay separate")
			Expect(condition(p, v1alpha1.ConditionPolicyCompliant)).To(Equal(metav1.ConditionTrue))
		})

		It("does not judge a loop whose output is below the variability gate", func() {
			hist.set(func(h *fakeHistorian) { h.loops["level"] = loopPerf(0.05, 0.01) })

			p := reconcileOnce()
			Expect(condition(p, v1alpha1.ConditionControlLoopsHealthy)).To(Equal(metav1.ConditionUnknown))
			Expect(conditionReason(p, v1alpha1.ConditionControlLoopsHealthy)).To(Equal("NoLoopEvaluated"))
			Expect(p.Status.Loops[0].Reason).To(Equal("OutputBelowGate"))
		})

		It("turns ControlLoopsHealthy False when a predictable loop drifts away from its setpoint", func() {
			// Experiment 25: PI ~1 (predictable drift) but far from the setpoint.
			var policy v1alpha1.OperatingPolicy
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "policy"}, &policy)).To(Succeed())
			policy.Spec.ControlLoops[0].MaxOffset = fp(1.0)
			Expect(k8sClient.Update(ctx, &policy)).To(Succeed())
			hist.set(func(h *fakeHistorian) {
				h.loops["level"] = map[string]any{"pi": 0.99, "sigma_op": 0.8, "offset": 3.5, "n": 300, "b": 30, "m": 60}
			})

			reconcileOnce()
			p := reconcileOnce() // persistence 2
			Expect(condition(p, v1alpha1.ConditionControlLoopsHealthy)).To(Equal(metav1.ConditionFalse))
			Expect(p.Status.Loops[0].Reason).To(Equal("OffsetExceeded"))
			Expect(meta.FindStatusCondition(p.Status.Conditions, v1alpha1.ConditionControlLoopsHealthy).Message).To(ContainSubstring("offset=3.5"))
		})

		It("keeps the economic verdict when only the loop endpoint fails", func() {
			hist.set(func(h *fakeHistorian) { h.failLoops = true })

			p := reconcileOnce()
			Expect(p.Status.Phase).To(Equal(v1alpha1.PhaseCompliant))
			Expect(condition(p, v1alpha1.ConditionPolicyCompliant)).To(Equal(metav1.ConditionTrue))
			Expect(condition(p, v1alpha1.ConditionControlLoopsHealthy)).To(Equal(metav1.ConditionUnknown))
			Expect(conditionReason(p, v1alpha1.ConditionControlLoopsHealthy)).To(Equal("HistorianUnreachable"))
		})
	})
})
