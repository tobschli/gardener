// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package shoot

import (
	"go.opentelemetry.io/otel"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	predicateutils "github.com/gardener/gardener/pkg/controllerutils/predicate"
)

// ControllerName is the name of this controller.
const ControllerName = "shoot"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor(ControllerName + "-scheduler")
	}
	if r.GardenNamespace == "" {
		r.GardenNamespace = v1beta1constants.GardenNamespace
	}
	if r.Tracer == nil {
		r.Tracer = otel.GetTracerProvider().Tracer("gardener-scheduler")
	}

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		For(&gardencorev1beta1.Shoot{}, builder.WithPredicates(
			r.ShootPredicate(),
			predicate.Not(predicateutils.IsDeleting()),
		)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Config.ConcurrentSyncs,
		}).
		Complete(r)
}

// ShootPredicate is a predicate that returns true if a shoot is not assigned to a seed
// and the default scheduler is configured.
func (r *Reconciler) ShootPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		if shoot, ok := obj.(*gardencorev1beta1.Shoot); ok {
			return shoot.Spec.SeedName == nil &&
				ptr.Deref(shoot.Spec.SchedulerName, v1beta1constants.DefaultSchedulerName) == v1beta1constants.DefaultSchedulerName
		}
		return false
	})
}
