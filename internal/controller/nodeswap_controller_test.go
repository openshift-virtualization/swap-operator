/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nodeswapv1alpha1 "github.com/openshift-virtualization/swap-operator/api/v1alpha1"
)

var _ = Describe("NodeSwap Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		nodeswap := &nodeswapv1alpha1.NodeSwap{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind NodeSwap")
			err := k8sClient.Get(ctx, typeNamespacedName, nodeswap)
			if err != nil && errors.IsNotFound(err) {
				resource := &nodeswapv1alpha1.NodeSwap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: nodeswapv1alpha1.NodeSwapSpec{
						MachineConfigPoolSelector: "node-role.kubernetes.io/role:worker",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &nodeswapv1alpha1.NodeSwap{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance NodeSwap")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &NodeSwapReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
		It("should update status conditions when reconciliation fails", func() {
			By("Creating a NodeSwap with invalid configuration")
			invalidResourceName := "test-invalid-resource"
			invalidTypeNamespacedName := types.NamespacedName{
				Name:      invalidResourceName,
				Namespace: "default",
			}

			invalidResource := &nodeswapv1alpha1.NodeSwap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      invalidResourceName,
					Namespace: "default",
				},
				Spec: nodeswapv1alpha1.NodeSwapSpec{
					// Invalid selector format to trigger error
					MachineConfigPoolSelector: "invalid-selector-no-colon",
				},
			}
			Expect(k8sClient.Create(ctx, invalidResource)).To(Succeed())

			By("Reconciling the resource with invalid configuration")
			controllerReconciler := &NodeSwapReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				TemplateDir: "../../templates", // Adjust path as needed
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: invalidTypeNamespacedName,
			})

			// Reconcile should return an error
			Expect(err).To(HaveOccurred())

			By("Verifying that status conditions reflect the error")
			updatedResource := &nodeswapv1alpha1.NodeSwap{}
			Expect(k8sClient.Get(ctx, invalidTypeNamespacedName, updatedResource)).To(Succeed())

			// Check that Degraded condition is True
			degradedCondition := meta.FindStatusCondition(updatedResource.Status.Conditions, "Degraded")
			Expect(degradedCondition).NotTo(BeNil())
			Expect(degradedCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(degradedCondition.Reason).To(Equal("ReconciliationFailed"))
			Expect(degradedCondition.Message).To(ContainSubstring("Failed to reconcile"))

			// Check that Progressing condition is False
			progressingCondition := meta.FindStatusCondition(updatedResource.Status.Conditions, "Progressing")
			Expect(progressingCondition).NotTo(BeNil())
			Expect(progressingCondition.Status).To(Equal(metav1.ConditionFalse))

			// Check that Available condition is False
			availableCondition := meta.FindStatusCondition(updatedResource.Status.Conditions, "Availabe")
			Expect(availableCondition).NotTo(BeNil())
			Expect(availableCondition.Status).To(Equal(metav1.ConditionFalse))

			By("Cleaning up the invalid resource")
			Expect(k8sClient.Delete(ctx, invalidResource)).To(Succeed())
		})
	})
})
