// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package seedbinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
)

var _ = Describe("SecretBindingControl", func() {
	var (
	// fakeClient client.Client
	// ctx        = context.TODO()
	)

	BeforeEach(func() {
		testScheme := runtime.NewScheme()
		Expect(kubernetes.AddGardenSchemeToScheme(testScheme)).To(Succeed())

		// fakeClient = fakeclient.NewClientBuilder().WithScheme(testScheme).Build()
	})

	Describe("#addTolerations", func() {
		testToleration := gardencorev1beta1.Toleration{
			Key:   "test",
			Value: ptr.To("test"),
		}
		tolerationList := []gardencorev1beta1.Toleration{}

		It("should be false, when List is nil", func() {
			result := tolerationInList(testToleration, nil)
			Expect(result).To(Equal(false))
		})

		It("should be false, when tolerationList is empty", func() {
			result := tolerationInList(testToleration, tolerationList)
			Expect(result).To(Equal(false))
		})

		It("should be true if the toleration is already in the List", func() {
			tolerationList = append(tolerationList, testToleration)

			result := tolerationInList(testToleration, tolerationList)
			Expect(len(tolerationList)).To(Equal(1))
			Expect(result).To(Equal(true))

		})

	})

})
