// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package seedbinding

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"

	"github.com/gardener/gardener/pkg/api"
	"github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/apis/core/validation"
)

type seedBindingStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

// Strategy defines the storage strategy for SecretBindings.
var Strategy = seedBindingStrategy{api.Scheme, names.SimpleNameGenerator}

func (seedBindingStrategy) NamespaceScoped() bool {
	return true
}

func (seedBindingStrategy) PrepareForCreate(_ context.Context, _ runtime.Object) {
}

func (seedBindingStrategy) Validate(_ context.Context, obj runtime.Object) field.ErrorList {
	binding := obj.(*core.SeedBinding)
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validation.ValidateSeedBinding(binding)...)
	return allErrs
}

func (seedBindingStrategy) Canonicalize(_ runtime.Object) {
}

func (seedBindingStrategy) AllowCreateOnUpdate() bool {
	return false
}

func (seedBindingStrategy) PrepareForUpdate(_ context.Context, newObj, oldObj runtime.Object) {
	_ = oldObj.(*core.SeedBinding)
	_ = newObj.(*core.SeedBinding)
}

func (seedBindingStrategy) AllowUnconditionalUpdate() bool {
	return true
}

func (seedBindingStrategy) ValidateUpdate(_ context.Context, newObj, oldObj runtime.Object) field.ErrorList {
	oldBinding, newBinding := oldObj.(*core.SeedBinding), newObj.(*core.SeedBinding)
	return validation.ValidateSeedBindingUpdate(newBinding, oldBinding)
}

// WarningsOnCreate returns warnings to the client performing a create.
func (seedBindingStrategy) WarningsOnCreate(_ context.Context, _ runtime.Object) []string {
	return nil
}

// WarningsOnUpdate returns warnings to the client performing the update.
func (seedBindingStrategy) WarningsOnUpdate(_ context.Context, _, _ runtime.Object) []string {
	return nil
}
