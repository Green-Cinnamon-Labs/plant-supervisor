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
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Green-Cinnamon-Labs/tep-operator/api/v1alpha1"
	"github.com/Green-Cinnamon-Labs/tep-operator/internal/evaluate"
	"github.com/Green-Cinnamon-Labs/tep-operator/internal/historian"
)

// Historian is what the reconciler needs from a historian; tests substitute a fake.
type Historian interface {
	Aggregate(ctx context.Context, keys []string, window time.Duration) (*historian.AggregateResponse, error)
}

// PlantReconciler evaluates each Plant against its active OperatingPolicy.
//
// One evaluation: Plant → OperatingPolicy → CostFunction → historian window means → J, targets,
// constraints → persistence rule → status. It never writes to the plant; the only output is the
// Plant's status, which kubectl and tep-ihm read through the Kubernetes API.
type PlantReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// NewHistorian builds a historian client for a URL. Defaults to historian.New.
	NewHistorian func(url string) Historian
}

// +kubebuilder:rbac:groups=supervision.greenlabs.io,resources=plants,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=supervision.greenlabs.io,resources=plants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=supervision.greenlabs.io,resources=operatingpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=supervision.greenlabs.io,resources=costfunctions,verbs=get;list;watch

func (r *PlantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var plant v1alpha1.Plant
	if err := r.Get(ctx, req.NamespacedName, &plant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	interval := time.Duration(plant.Spec.EvaluationIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	requeue := ctrl.Result{RequeueAfter: interval}
	status := &plant.Status

	var policy v1alpha1.OperatingPolicy
	if err := r.Get(ctx, types.NamespacedName{Namespace: plant.Namespace, Name: plant.Spec.PolicyRef}, &policy); err != nil {
		return requeue, r.pending(ctx, &plant, "PolicyNotFound", fmt.Sprintf("OperatingPolicy %q: %v", plant.Spec.PolicyRef, err))
	}
	var cf v1alpha1.CostFunction
	if err := r.Get(ctx, types.NamespacedName{Namespace: plant.Namespace, Name: policy.Spec.CostFunctionRef}, &cf); err != nil {
		return requeue, r.pending(ctx, &plant, "CostFunctionNotFound", fmt.Sprintf("CostFunction %q: %v", policy.Spec.CostFunctionRef, err))
	}

	policyChanged := status.ActivePolicy != policy.Name
	status.ActivePolicy = policy.Name

	window := time.Duration(policy.Spec.WindowSeconds) * time.Second
	if window <= 0 {
		window = 60 * time.Second
	}
	keys := evaluate.RequiredSignals(cf.Spec, policy.Spec)
	agg, err := r.historian(plant.Spec.HistorianURL).Aggregate(ctx, keys, window)
	if err != nil {
		return requeue, r.pending(ctx, &plant, "HistorianUnreachable", err.Error())
	}
	if !agg.Connected {
		return requeue, r.pending(ctx, &plant, "PlantDisconnected", "historian is not connected to the plant")
	}
	means := agg.Means()
	if missing := evaluate.MissingSignals(keys, means); len(missing) > 0 {
		return requeue, r.pending(ctx, &plant, "MissingSignals", "no samples for: "+strings.Join(missing, ", "))
	}

	result := evaluate.Evaluate(cf.Spec, policy.Spec, means)
	status.ConsecutiveViolations = evaluate.NextViolations(status.ConsecutiveViolations, result.Violated(), policyChanged)
	nonCompliant := evaluate.NonCompliant(status.ConsecutiveViolations, policy.Spec.PersistenceEvaluations)

	now := metav1.Now()
	status.LastEvaluationTime = &now
	status.Cost = &v1alpha1.CostStatus{
		Value:     result.Cost,
		Unit:      cf.Spec.Unit,
		Reference: cf.Spec.Reference,
		MaxCost:   policy.Spec.MaxCost,
	}
	status.Terms = result.Terms
	status.Targets = result.Targets
	status.Constraints = result.Constraints

	setCondition(&plant, v1alpha1.ConditionDataAvailable, true, "SignalsAvailable",
		fmt.Sprintf("%d signals averaged over %s", len(keys), window))
	setCondition(&plant, v1alpha1.ConditionCostWithinBudget, result.CostWithinBudget, reason(result.CostWithinBudget, "WithinBudget", "OverBudget"),
		costMessage(result.Cost, cf.Spec.Unit, policy.Spec.MaxCost))
	setCondition(&plant, v1alpha1.ConditionTargetsMet, result.TargetsMet, reason(result.TargetsMet, "TargetsMet", "TargetMissed"),
		failedTargets(result.Targets))
	setCondition(&plant, v1alpha1.ConditionConstraintsSatisfied, result.ConstraintsSatisfied, reason(result.ConstraintsSatisfied, "ConstraintsSatisfied", "ConstraintViolated"),
		failedConstraints(result.Constraints))
	if nonCompliant {
		status.Phase = v1alpha1.PhaseNonCompliant
		setCondition(&plant, v1alpha1.ConditionPolicyCompliant, false, "PolicyViolated",
			fmt.Sprintf("%d consecutive failing evaluations (persistence %d)", status.ConsecutiveViolations, policy.Spec.PersistenceEvaluations))
	} else {
		status.Phase = v1alpha1.PhaseCompliant
		msg := "all checks pass"
		if status.ConsecutiveViolations > 0 {
			msg = fmt.Sprintf("%d failing evaluation(s), below persistence %d", status.ConsecutiveViolations, policy.Spec.PersistenceEvaluations)
		}
		setCondition(&plant, v1alpha1.ConditionPolicyCompliant, true, "PolicyCompliant", msg)
	}

	if err := r.Status().Update(ctx, &plant); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("plant evaluated", "policy", policy.Name, "cost", result.Cost, "phase", status.Phase,
		"violations", status.ConsecutiveViolations)
	return requeue, nil
}

// pending records that no verdict could be produced and why. The verdict conditions become
// Unknown rather than keeping a stale True/False.
func (r *PlantReconciler) pending(ctx context.Context, plant *v1alpha1.Plant, reason, message string) error {
	logf.FromContext(ctx).Info("plant pending", "reason", reason, "message", message)
	plant.Status.Phase = v1alpha1.PhasePending
	meta.SetStatusCondition(&plant.Status.Conditions, metav1.Condition{
		Type: v1alpha1.ConditionDataAvailable, Status: metav1.ConditionFalse, Reason: reason, Message: message,
		ObservedGeneration: plant.Generation,
	})
	for _, t := range []string{
		v1alpha1.ConditionCostWithinBudget, v1alpha1.ConditionTargetsMet,
		v1alpha1.ConditionConstraintsSatisfied, v1alpha1.ConditionPolicyCompliant,
	} {
		meta.SetStatusCondition(&plant.Status.Conditions, metav1.Condition{
			Type: t, Status: metav1.ConditionUnknown, Reason: reason, Message: "no evaluation",
			ObservedGeneration: plant.Generation,
		})
	}
	return r.Status().Update(ctx, plant)
}

func (r *PlantReconciler) historian(url string) Historian {
	if r.NewHistorian != nil {
		return r.NewHistorian(url)
	}
	return historian.New(url)
}

func setCondition(plant *v1alpha1.Plant, condType string, ok bool, reason, message string) {
	status := metav1.ConditionFalse
	if ok {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&plant.Status.Conditions, metav1.Condition{
		Type: condType, Status: status, Reason: reason, Message: message, ObservedGeneration: plant.Generation,
	})
}

func reason(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func costMessage(cost float64, unit string, maxCost *float64) string {
	if maxCost == nil {
		return fmt.Sprintf("J = %.2f %s (no budget declared)", cost, unit)
	}
	return fmt.Sprintf("J = %.2f %s, budget %.2f", cost, unit, *maxCost)
}

func failedTargets(results []v1alpha1.TargetResult) string {
	var failed []string
	for _, t := range results {
		if !t.Met {
			failed = append(failed, fmt.Sprintf("%s=%s (target %.4g)", t.Signal, observed(t.Observed), t.Target))
		}
	}
	return joinOr(failed, fmt.Sprintf("%d target(s) met", len(results)))
}

func failedConstraints(results []v1alpha1.ConstraintResult) string {
	var failed []string
	for _, c := range results {
		if !c.Satisfied {
			failed = append(failed, fmt.Sprintf("%s=%s", c.Signal, observed(c.Observed)))
		}
	}
	return joinOr(failed, fmt.Sprintf("%d constraint(s) satisfied", len(results)))
}

func observed(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.4g", *v)
}

func joinOr(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	return strings.Join(items, "; ")
}

// SetupWithManager watches Plants (spec changes only — status updates must not retrigger) and
// re-evaluates every Plant in a namespace when a policy or cost function there changes, so an
// edited policy takes effect immediately instead of at the next interval.
func (r *PlantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	plantsInNamespace := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		var plants v1alpha1.PlantList
		if err := r.List(ctx, &plants, client.InNamespace(obj.GetNamespace())); err != nil {
			return nil
		}
		reqs := make([]reconcile.Request, 0, len(plants.Items))
		for _, p := range plants.Items {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: p.Namespace, Name: p.Name}})
		}
		return reqs
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Plant{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&v1alpha1.OperatingPolicy{}, plantsInNamespace).
		Watches(&v1alpha1.CostFunction{}, plantsInNamespace).
		Named("plant").
		Complete(r)
}
