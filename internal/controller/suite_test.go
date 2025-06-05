/*
Copyright 2023.

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
	coopv1 "github.com/williamnoble/coop/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	utilv1 "github.com/williamnoble/coop/api/v1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx    context.Context
	cancel context.CancelFunc
)
var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())
	var err error
	err = utilv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&CoopReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("COOP controller - ConfigMap", func() {

	const (
		SourceConfigMapName           = "test-configmap"
		SourceConfigMapNamespace      = "default"
		DestinationConfigMapNamespace = "fruits"
		timeout                       = time.Second * 10
		interval                      = time.Millisecond * 250
	)

	var (
		ConfigMapData = map[string]string{"fruit": "lemon"}
	)

	Context("Coop", func() {
		It("Config Map Test", func() {
			By("Create an example config map")
			ctx := context.Background()
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: SourceConfigMapName, Namespace: SourceConfigMapNamespace},
				Data:       ConfigMapData,
			}

			// Firstly, we create an example config map
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			// Create the destination namespace if it doesn't exist
			destNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: DestinationConfigMapNamespace,
				},
			}
			err := k8sClient.Create(ctx, destNamespace)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			// Create the COOP resource
			By("Create a new COOP Resource which references the example config map")
			coop := &coopv1.Coop{
				ObjectMeta: metav1.ObjectMeta{Name: "test-coop", Namespace: "default"},
				Spec: coopv1.CoopSpec{
					Source: coopv1.SourceSpec{
						Name:      SourceConfigMapName,
						Namespace: SourceConfigMapNamespace,
					},
					Destination: coopv1.DestinationSpec{
						Namespace: DestinationConfigMapNamespace,
					},
				},
			}
			Expect(k8sClient.Create(ctx, coop)).To(Succeed())

			time.Sleep(2 * time.Second)

			destinationConfigMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: DestinationConfigMapNamespace,
					Name:      SourceConfigMapName,
				}, destinationConfigMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// verify the data in the destination ConfigMap matches the source
			Expect(destinationConfigMap.Data).To(Equal(ConfigMapData))

			// check the finalizer works
			Expect(k8sClient.Delete(ctx, coop)).To(Succeed())
			Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())

		})
	})
})
