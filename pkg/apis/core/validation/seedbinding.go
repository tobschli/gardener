// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/gardener/gardener/pkg/apis/core"
)

// ValidateSeedBinding validates a SeedBinding object.
func ValidateSeedBinding(binding *core.SeedBinding) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, apivalidation.ValidateObjectMeta(&binding.ObjectMeta, true, ValidateName, field.NewPath("metadata"))...)
	if binding.SeedSelector != nil {
		allErrs = append(allErrs, metav1validation.ValidateLabelSelector(&binding.SeedSelector.LabelSelector, metav1validation.LabelSelectorValidationOptions{AllowInvalidLabelValueInSelector: true}, field.NewPath("seedSelector"))...)
	}

	return allErrs
}

// ValidateSecretBindingUpdate validates a SecretBinding object before an update.
func ValidateSeedBindingUpdate(newBinding, oldBinding *core.SeedBinding) field.ErrorList {
	allErrs := field.ErrorList{}

	//TODO: Update Validation

	return allErrs
}
