// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package seedbinding

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/controllermanager/apis/config"
	"github.com/gardener/gardener/pkg/controllerutils"
)

const (
	SeedBindingTolerationKey = "seedBinding"
)

// Reconciler reconciles SeedBindings.
type Reconciler struct {
	Client   client.Client
	Config   config.SeedBindingControllerConfiguration
	Recorder record.EventRecorder
}

// Reconcile reconciles SeedBindings.
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	ctx, cancel := controllerutils.GetMainReconciliationContext(ctx, controllerutils.DefaultReconciliationTimeout)
	defer cancel()

	seedBinding := &gardencorev1beta1.SeedBinding{}
	if err := r.Client.Get(ctx, request.NamespacedName, seedBinding); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	if seedBinding.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(seedBinding, gardencorev1beta1.GardenerName) {
			log.Info("Remove finalizer")
			if err := controllerutils.RemoveFinalizers(ctx, r.Client, seedBinding, gardencorev1beta1.GardenerName); err != nil {
				return reconcile.Result{}, fmt.Errorf("could not remove finalizer: %w", err)
			}
		}
	}

	if seedBinding.TaintSeed {
		// Get project from namespace
		// TODO: Not generally possible, because the namespace does not need to be prefixed with 'garden': Go through all Projects and search the one where this is the namespace

		projectName := strings.Replace(seedBinding.Namespace, "garden-", "", -1)

		project := &gardencorev1beta1.Project{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: projectName}, project); err != nil {
			return reconcile.Result{}, fmt.Errorf("error retrieving project from store: %w", err)
		}

		if err := r.ensureProjectTolerations(ctx, project, seedBinding); err != nil {
			return reconcile.Result{}, err
		}
	}

	if !controllerutil.ContainsFinalizer(seedBinding, gardencorev1beta1.GardenerName) {
		log.Info("Adding finalizer")
		if err := controllerutils.AddFinalizers(ctx, r.Client, seedBinding, gardencorev1beta1.GardenerName); err != nil {
			return reconcile.Result{}, fmt.Errorf("could not add finalizer: %w", err)
		}
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) ensureProjectTolerations(ctx context.Context, project *gardencorev1beta1.Project, seedBinding *gardencorev1beta1.SeedBinding) error {
	bindingToleration := seedBindingToleration(seedBinding)

	// Check if project has the toleration in the WhiteList

	if project.Spec.Tolerations == nil {
		project.Spec.Tolerations = &gardencorev1beta1.ProjectTolerations{}
	}

	if project.Spec.Tolerations.Whitelist == nil {
		project.Spec.Tolerations.Whitelist = []gardencorev1beta1.Toleration{}
	}

	if project.Spec.Tolerations.Defaults == nil {
		project.Spec.Tolerations.Defaults = []gardencorev1beta1.Toleration{}
	}

	if !tolerationInList(bindingToleration, project.Spec.Tolerations.Whitelist) {
		project.Spec.Tolerations.Whitelist = append(project.Spec.Tolerations.Whitelist, bindingToleration)
	}

	if !tolerationInList(bindingToleration, project.Spec.Tolerations.Defaults) {
		project.Spec.Tolerations.Defaults = append(project.Spec.Tolerations.Defaults, bindingToleration)
	}

	if err := r.Client.Update(ctx, project); err != nil {
		return fmt.Errorf("failed to update Project Tolerations: %w", err)
	}

	return nil
}

func seedBindingToleration(seedBinding *gardencorev1beta1.SeedBinding) gardencorev1beta1.Toleration {
	// must be globally unique, name alone does not suffice
	// TODO: Change this, because this does not suffice / there is no real mechanism for that
	return gardencorev1beta1.Toleration{
		Key:   SeedBindingTolerationKey,
		Value: &seedBinding.Name,
	}
}

func tolerationInList(targetToleration gardencorev1beta1.Toleration, tolerations []gardencorev1beta1.Toleration) bool {
	for _, toleration := range tolerations {
		// TODO: Pointer not equal obviously
		if toleration.Key == targetToleration.Key && ptr.Deref[string](toleration.Value, "") == ptr.Deref[string](targetToleration.Value, "") {
			return true
		}
	}
	return false
}
