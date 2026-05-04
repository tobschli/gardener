// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package managedseedset

import (
	"fmt"

	seedmanagementv1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
)

// GetMSSRevisions exposes the internal getMSSRevisions function for use in tests.
var GetMSSRevisions = getMSSRevisions

// GetMSSRevisionName computes the ControllerRevision name that getMSSRevisions would produce for
// the given ManagedSeedSet with a zero collision count. It mirrors the logic in newMSSRevision /
// createControllerRevision so that test assertions can reference the expected name at
// spec-registration time (before BeforeEach runs).
var GetMSSRevisionName = func(set *seedmanagementv1alpha1.ManagedSeedSet) string {
	patch, err := getMSSPatch(set)
	if err != nil {
		panic(fmt.Sprintf("GetMSSRevisionName: getMSSPatch failed: %v", err))
	}
	// Use a zero collision count pointer, matching the behaviour of getMSSRevisions which always
	// passes &collisionCount (where collisionCount == 0 initially) to newMSSRevision /
	// createControllerRevision.
	var collisionCount int32
	hash := hashRevisionData(patch, &collisionCount)
	return controllerRevisionName(set.Name, hash)
}
