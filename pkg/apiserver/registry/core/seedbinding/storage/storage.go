// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/apiserver/registry/core/seedbinding"
)

// REST implements a RESTStorage for SecretBinding
type REST struct {
	*genericregistry.Store
}

// SeedBindingStorage implements the storage for SeedBindings.
type SeedBindingStorage struct {
	SeedBinding *REST
}

// NewStorage creates a new SeedBindingStorage object.
func NewStorage(optsGetter generic.RESTOptionsGetter) SeedBindingStorage {
	seedBindingRest := NewREST(optsGetter)

	return SeedBindingStorage{
		SeedBinding: seedBindingRest,
	}
}

// NewREST returns a RESTStorage object that will work with SecretBinding objects.
func NewREST(optsGetter generic.RESTOptionsGetter) *REST {
	store := &genericregistry.Store{
		NewFunc:                   func() runtime.Object { return &core.SeedBinding{} },
		NewListFunc:               func() runtime.Object { return &core.SeedBindingList{} },
		DefaultQualifiedResource:  core.Resource("seedbindings"),
		SingularQualifiedResource: core.Resource("seedbinding"),
		EnableGarbageCollection:   true,

		CreateStrategy: seedbinding.Strategy,
		UpdateStrategy: seedbinding.Strategy,
		DeleteStrategy: seedbinding.Strategy,

		TableConvertor: newTableConvertor(),
	}
	options := &generic.StoreOptions{RESTOptions: optsGetter}
	if err := store.CompleteWithOptions(options); err != nil {
		panic(err)
	}
	return &REST{store}
}

// Implement ShortNamesProvider
var _ rest.ShortNamesProvider = &REST{}

// ShortNames implements the ShortNamesProvider interface. Returns a list of short names for a resource.
func (r *REST) ShortNames() []string {
	return []string{}
}
