// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SeedBinding represents a binding to a seed.
type SeedBinding struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// TaintSeed determines if the SeedBinding should
	// add a taint to the targeted seed(s)
	TaintSeed bool `json:"taintSeed" protobuf:"varint,2,opt,name=taintSeed"`

	// SeedSelector is a selector for
	// one or more seeds that the scheduling should be restricted to.
	// If it is used, it is not possible to use the SeedRef.
	// +optional
	SeedSelector *SeedSelector `json:"seedSelector,omitempty" protobuf:"bytes,4,opt,name=seedSelector"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SecretBindingList is a collection of SecretBindings.
type SeedBindingList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list object metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	// Items is the list of SecretBindings.
	Items []SeedBinding `json:"items" protobuf:"bytes,2,rep,name=items"`
}
