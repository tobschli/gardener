// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package clusterautoscaler

import (
	"context"
	_ "embed"

	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/utils/managedresources"
)

var (
	//go:embed templates/crd-autoscaling.x-k8s.io_provisioningrequests.yaml
	provisioningRequestsCRD string

	crdResources map[string]string
)

func init() {
	crdResources = map[string]string{
		"crd-autoscaling.x-k8s.io_provisioningrequests.yaml": provisioningRequestsCRD,
	}
}

type prCRD struct {
	registry *managedresources.Registry
}

// NewCRD can be used to deploy the CRD definitions for the Kubernetes Vertical Pod Autoscaler.
func NewCRD(registry *managedresources.Registry) component.Deployer {
	return &prCRD{
		registry: registry,
	}
}

// Deploy creates and updates the CRD definitions for the Kubernetes Vertical Pod Autoscaler.
func (c *prCRD) Deploy(ctx context.Context) error {
	for filename, resource := range crdResources {
		c.registry.AddSerialized(filename, []byte(resource))
	}

	return nil
}

func (c *prCRD) Destroy(_ context.Context) error {
	return nil
}
