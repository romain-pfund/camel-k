//go:build integration
// +build integration

// To enable compilation of this file in Goland, go to "Settings -> Go -> Vendoring & Build Tags -> Custom Tags" and add "integration"

/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package advanced

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	. "github.com/apache/camel-k/v2/e2e/support"
	v1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1"
)

func TestOperatorIDCamelCatalogReconciliation(t *testing.T) {
	WithNewTestNamespace(t, func(ns string) {
		operatorID := fmt.Sprintf("camel-k-%s", ns)
		Expect(KamelInstallWithID(operatorID, ns, "--global", "--force").Execute()).To(Succeed())
		Eventually(PlatformPhase(ns), TestTimeoutMedium).Should(Equal(v1.IntegrationPlatformPhaseReady))
		Eventually(DefaultCamelCatalogPhase(ns), TestTimeoutMedium).Should(Equal(v1.CamelCatalogPhaseReady))
	})
}

func TestOperatorIDFiltering(t *testing.T) {
	RegisterTestingT(t)

	WithNewTestNamespace(t, func(ns string) {
		WithNewTestNamespace(t, func(nsop1 string) {
			operator1 := "operator-1"
			Expect(CopyCamelCatalog(nsop1, operator1)).To(Succeed())
			Expect(KamelInstallWithIDAndKameletCatalog(operator1, nsop1, "--global", "--force").Execute()).To(Succeed())
			Eventually(PlatformPhase(nsop1), TestTimeoutMedium).Should(Equal(v1.IntegrationPlatformPhaseReady))

			WithNewTestNamespace(t, func(nsop2 string) {
				operator2 := "operator-2"
				Expect(CopyCamelCatalog(nsop2, operator2)).To(Succeed())
				Expect(KamelInstallWithIDAndKameletCatalog(operator2, nsop2, "--global", "--force").Execute()).To(Succeed())
				Eventually(PlatformPhase(nsop2), TestTimeoutMedium).Should(Equal(v1.IntegrationPlatformPhaseReady))

				t.Run("Operators ignore non-scoped integrations", func(t *testing.T) {
					Expect(KamelRunWithID("operator-x", ns, "files/yaml.yaml", "--name", "untouched", "--force").Execute()).To(Succeed())
					Consistently(IntegrationPhase(ns, "untouched"), 10*time.Second).Should(BeEmpty())
				})

				t.Run("Operators run scoped integrations", func(t *testing.T) {
					Expect(KamelRunWithID("operator-x", ns, "files/yaml.yaml", "--name", "moving", "--force").Execute()).To(Succeed())
					Expect(AssignIntegrationToOperator(ns, "moving", operator1)).To(Succeed())
					Eventually(IntegrationPhase(ns, "moving"), TestTimeoutMedium).Should(Equal(v1.IntegrationPhaseRunning))
					Eventually(IntegrationPodPhase(ns, "moving"), TestTimeoutLong).Should(Equal(corev1.PodRunning))
					Eventually(IntegrationLogs(ns, "moving"), TestTimeoutShort).Should(ContainSubstring("Magicstring!"))
				})

				t.Run("Operators can handoff scoped integrations", func(t *testing.T) {
					Expect(AssignIntegrationToOperator(ns, "moving", operator2)).To(Succeed())
					Eventually(IntegrationPhase(ns, "moving"), TestTimeoutMedium).Should(Equal(v1.IntegrationPhaseBuildingKit))
					Eventually(IntegrationPhase(ns, "moving"), TestTimeoutMedium).Should(Equal(v1.IntegrationPhaseRunning))
					Eventually(IntegrationPodPhase(ns, "moving"), TestTimeoutLong).Should(Equal(corev1.PodRunning))
					Eventually(IntegrationLogs(ns, "moving"), TestTimeoutShort).Should(ContainSubstring("Magicstring!"))
				})

				t.Run("Operators can be deactivated after completely handing off scoped integrations", func(t *testing.T) {
					Expect(ScaleOperator(nsop1, 0)).To(Succeed())
					Expect(Kamel("rebuild", "-n", ns, "moving").Execute()).To(Succeed())
					Eventually(IntegrationPhase(ns, "moving"), TestTimeoutMedium).Should(Equal(v1.IntegrationPhaseRunning))
					Eventually(IntegrationPodPhase(ns, "moving"), TestTimeoutLong).Should(Equal(corev1.PodRunning))
					Eventually(IntegrationLogs(ns, "moving"), TestTimeoutShort).Should(ContainSubstring("Magicstring!"))
					Expect(ScaleOperator(nsop1, 1)).To(Succeed())
				})

				t.Run("Operators can run scoped integrations with fixed image", func(t *testing.T) {
					image := IntegrationPodImage(ns, "moving")()
					Expect(image).NotTo(BeEmpty())
					// Save resources by deleting "moving" integration
					Expect(Kamel("delete", "moving", "-n", ns).Execute()).To(Succeed())

					Expect(KamelRunWithID("operator-x", ns, "files/yaml.yaml", "--name", "pre-built", "--force",
						"-t", fmt.Sprintf("container.image=%s", image), "-t", "jvm.enabled=true").Execute()).To(Succeed())
					Consistently(IntegrationPhase(ns, "pre-built"), 10*time.Second).Should(BeEmpty())
					Expect(AssignIntegrationToOperator(ns, "pre-built", operator2)).To(Succeed())
					Eventually(IntegrationPhase(ns, "pre-built"), TestTimeoutShort).Should(Equal(v1.IntegrationPhaseRunning))
					Eventually(IntegrationPodPhase(ns, "pre-built"), TestTimeoutLong).Should(Equal(corev1.PodRunning))
					Eventually(IntegrationLogs(ns, "pre-built"), TestTimeoutShort).Should(ContainSubstring("Magicstring!"))
					Expect(Kamel("delete", "pre-built", "-n", ns).Execute()).To(Succeed())
				})

				t.Run("Operators can run scoped Pipes", func(t *testing.T) {
					Expect(KamelBindWithID("operator-x", ns, "timer-source?message=Hello", "log-sink",
						"--name", "klb", "--force").Execute()).To(Succeed())
					Consistently(Integration(ns, "klb"), 10*time.Second).Should(BeNil())

					Expect(AssignPipeToOperator(ns, "klb", operator1)).To(Succeed())
					Eventually(Integration(ns, "klb"), TestTimeoutShort).ShouldNot(BeNil())
					Eventually(IntegrationPhase(ns, "klb"), TestTimeoutMedium).Should(Equal(v1.IntegrationPhaseRunning))
					Eventually(IntegrationPodPhase(ns, "klb"), TestTimeoutLong).Should(Equal(corev1.PodRunning))
				})
			})
		})

		Expect(Kamel("delete", "--all", "-n", ns).Execute()).To(Succeed())
	})
}
