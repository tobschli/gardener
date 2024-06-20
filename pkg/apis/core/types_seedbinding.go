// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package core

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SeedBinding represents a binding to a seed.
type SeedBinding struct {
	metav1.TypeMeta
	// Standard object metadata.
	metav1.ObjectMeta

	// TaintSeed determines if the SeedBinding should
	// add a taint to the targeted seed(s)
	TaintSeed bool

	// SeedSelector is a selector for
	// one or more seeds that the scheduling should be restricted to.
	// If it is used, it is not possible to use the SeedRef.
	SeedSelector *SeedSelector
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SecretBindingList is a collection of SecretBindings.
type SeedBindingList struct {
	metav1.TypeMeta
	// Standard list object metadata.
	metav1.ListMeta
	// Items is the list of SecretBindings.
	Items []SeedBinding
}
