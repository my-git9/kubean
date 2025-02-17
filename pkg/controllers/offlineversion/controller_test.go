// Copyright 2023 Authors of kubean-io
// SPDX-License-Identifier: Apache-2.0

package offlineversion

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	localartifactsetv1alpha1 "github.com/kubean-io/kubean-api/apis/localartifactset/v1alpha1"
	manifestv1alpha1 "github.com/kubean-io/kubean-api/apis/manifest/v1alpha1"
	"github.com/kubean-io/kubean-api/constants"
	localartifactsetv1alpha1fake "github.com/kubean-io/kubean-api/generated/localartifactset/clientset/versioned/fake"
	manifestv1alpha1fake "github.com/kubean-io/kubean-api/generated/manifest/clientset/versioned/fake"
)

func newFakeClient() client.Client {
	sch := scheme.Scheme
	if err := manifestv1alpha1.AddToScheme(sch); err != nil {
		panic(err)
	}
	if err := localartifactsetv1alpha1.AddToScheme(sch); err != nil {
		panic(err)
	}

	client := fake.NewClientBuilder().WithScheme(sch).WithRuntimeObjects(&manifestv1alpha1.Manifest{}).WithRuntimeObjects(&localartifactsetv1alpha1.LocalArtifactSet{}).Build()
	return client
}

func TestMergeOfflineVersion(t *testing.T) {
	controller := &Controller{
		Client:                    newFakeClient(),
		ClientSet:                 clientsetfake.NewSimpleClientset(),
		LocalArtifactSetClientSet: localartifactsetv1alpha1fake.NewSimpleClientset(),
		InfoManifestClientSet:     manifestv1alpha1fake.NewSimpleClientset(),
	}
	tests := []struct {
		name string
		args struct {
			OfflineVersion    localartifactsetv1alpha1.LocalArtifactSet
			ComponentsVersion manifestv1alpha1.Manifest
		}
		updated bool
	}{
		{
			name: "nothing update",
			args: struct {
				OfflineVersion    localartifactsetv1alpha1.LocalArtifactSet
				ComponentsVersion manifestv1alpha1.Manifest
			}{
				OfflineVersion: localartifactsetv1alpha1.LocalArtifactSet{
					Spec: localartifactsetv1alpha1.Spec{},
				},
				ComponentsVersion: manifestv1alpha1.Manifest{
					Status: manifestv1alpha1.Status{
						LocalAvailable: manifestv1alpha1.LocalAvailable{},
					},
				},
			},
			updated: false,
		},
		{
			name: "update software info",
			args: struct {
				OfflineVersion    localartifactsetv1alpha1.LocalArtifactSet
				ComponentsVersion manifestv1alpha1.Manifest
			}{
				OfflineVersion: localartifactsetv1alpha1.LocalArtifactSet{
					Spec: localartifactsetv1alpha1.Spec{
						Items: []*localartifactsetv1alpha1.SoftwareInfo{
							{
								Name:         "etcd-1",
								VersionRange: []string{"1.1", "1.2"},
							},
						},
					},
				},
				ComponentsVersion: manifestv1alpha1.Manifest{
					Status: manifestv1alpha1.Status{
						LocalAvailable: manifestv1alpha1.LocalAvailable{},
					},
				},
			},
			updated: true,
		},
		{
			name: "update software info",
			args: struct {
				OfflineVersion    localartifactsetv1alpha1.LocalArtifactSet
				ComponentsVersion manifestv1alpha1.Manifest
			}{
				OfflineVersion: localartifactsetv1alpha1.LocalArtifactSet{
					Spec: localartifactsetv1alpha1.Spec{
						Items: []*localartifactsetv1alpha1.SoftwareInfo{
							{
								Name:         "etcd-1",
								VersionRange: []string{"1.1", "1.2"},
							},
						},
					},
				},
				ComponentsVersion: manifestv1alpha1.Manifest{
					Status: manifestv1alpha1.Status{
						LocalAvailable: manifestv1alpha1.LocalAvailable{
							Components: []*manifestv1alpha1.SoftwareInfoStatus{
								{
									Name:         "etcd-1",
									VersionRange: []string{"1.2", "1.3"},
								},
							},
						},
					},
				},
			},
			updated: true,
		},
		{
			name: "add software info but nothing updated",
			args: struct {
				OfflineVersion    localartifactsetv1alpha1.LocalArtifactSet
				ComponentsVersion manifestv1alpha1.Manifest
			}{
				OfflineVersion: localartifactsetv1alpha1.LocalArtifactSet{
					Spec: localartifactsetv1alpha1.Spec{
						Items: []*localartifactsetv1alpha1.SoftwareInfo{
							{
								Name:         "etcd-1",
								VersionRange: []string{"1.1", "1.2"},
							},
						},
					},
				},
				ComponentsVersion: manifestv1alpha1.Manifest{
					Status: manifestv1alpha1.Status{
						LocalAvailable: manifestv1alpha1.LocalAvailable{
							Components: []*manifestv1alpha1.SoftwareInfoStatus{
								{
									Name:         "etcd-1",
									VersionRange: []string{"1.1", "1.2", "1.3"},
								},
							},
						},
					},
				},
			},
			updated: false,
		},
		{
			name: "update docker-ce info",
			args: struct {
				OfflineVersion    localartifactsetv1alpha1.LocalArtifactSet
				ComponentsVersion manifestv1alpha1.Manifest
			}{
				OfflineVersion: localartifactsetv1alpha1.LocalArtifactSet{
					Spec: localartifactsetv1alpha1.Spec{
						Docker: []*localartifactsetv1alpha1.DockerInfo{
							{
								OS:           "redhat-7",
								VersionRange: []string{"20.01", "20.02"},
							},
						},
						Items: []*localartifactsetv1alpha1.SoftwareInfo{
							{
								Name:         "etcd-1",
								VersionRange: []string{"1.1", "1.2"},
							},
						},
					},
				},
				ComponentsVersion: manifestv1alpha1.Manifest{
					Status: manifestv1alpha1.Status{
						LocalAvailable: manifestv1alpha1.LocalAvailable{
							Docker: []*manifestv1alpha1.DockerInfoStatus{
								{
									OS:           "redhat-8",
									VersionRange: []string{},
								},
							},
							Components: []*manifestv1alpha1.SoftwareInfoStatus{
								{
									Name:         "etcd-1",
									VersionRange: []string{"1.1", "1.2"},
								},
							},
						},
					},
				},
			},
			updated: true,
		},
		{
			name: "nothing updated",
			args: struct {
				OfflineVersion    localartifactsetv1alpha1.LocalArtifactSet
				ComponentsVersion manifestv1alpha1.Manifest
			}{
				OfflineVersion: localartifactsetv1alpha1.LocalArtifactSet{
					Spec: localartifactsetv1alpha1.Spec{
						Docker: []*localartifactsetv1alpha1.DockerInfo{
							{
								OS:           "redhat-7",
								VersionRange: []string{"20.01", "20.02"},
							},
						},
						Items: []*localartifactsetv1alpha1.SoftwareInfo{
							{
								Name:         "etcd-1",
								VersionRange: []string{"1.1", "1.2"},
							},
						},
					},
				},
				ComponentsVersion: manifestv1alpha1.Manifest{
					Status: manifestv1alpha1.Status{
						LocalAvailable: manifestv1alpha1.LocalAvailable{
							Docker: []*manifestv1alpha1.DockerInfoStatus{
								{
									OS:           "redhat-7",
									VersionRange: []string{"20.02", "20.01"},
								},
								{
									OS:           "redhat-8",
									VersionRange: []string{"21.02", "21.01"},
								},
							},
							Components: []*manifestv1alpha1.SoftwareInfoStatus{
								{
									Name:         "etcd-1",
									VersionRange: []string{"1.2", "1.1"},
								},
							},
						},
					},
				},
			},
			updated: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if updated, _ := controller.MergeOfflineVersionStatus(&test.args.OfflineVersion, &test.args.ComponentsVersion); updated != test.updated {
				t.Fatal()
			}
		})
	}
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name string
		args func() bool
		want bool
	}{
		{
			name: "merge successfully",
			args: func() bool {
				controller := &Controller{
					Client:                    newFakeClient(),
					ClientSet:                 clientsetfake.NewSimpleClientset(),
					LocalArtifactSetClientSet: localartifactsetv1alpha1fake.NewSimpleClientset(),
					InfoManifestClientSet:     manifestv1alpha1fake.NewSimpleClientset(),
				}
				offlineVersionData := localartifactsetv1alpha1.LocalArtifactSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "LocalArtifactSet",
						APIVersion: "kubean.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "offlineversion-1",
					},
					Spec: localartifactsetv1alpha1.Spec{
						Docker: []*localartifactsetv1alpha1.DockerInfo{
							{
								OS:           "redhat-7",
								VersionRange: []string{"20.1", "20.2"},
							},
						},
					},
				}

				globalComponentsVersion := manifestv1alpha1.Manifest{
					TypeMeta: metav1.TypeMeta{
						Kind:       "kubeanclusterconfig",
						APIVersion: "kubean.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.InfoManifestGlobal,
					},
				}

				controller.Client.Create(context.Background(), &offlineVersionData)
				controller.LocalArtifactSetClientSet.KubeanV1alpha1().LocalArtifactSets().Create(context.Background(), &offlineVersionData, metav1.CreateOptions{})
				controller.InfoManifestClientSet.KubeanV1alpha1().Manifests().Create(context.Background(), &globalComponentsVersion, metav1.CreateOptions{})

				result, err := controller.Reconcile(context.Background(), controllerruntime.Request{NamespacedName: types.NamespacedName{Name: offlineVersionData.Name}})
				newGlobalComponentsVersion, _ := controller.InfoManifestClientSet.KubeanV1alpha1().Manifests().Get(context.Background(), constants.InfoManifestGlobal, metav1.GetOptions{})
				return err == nil && result.RequeueAfter == Loop && len(newGlobalComponentsVersion.Status.LocalAvailable.Docker) == 1 && len(newGlobalComponentsVersion.Status.LocalAvailable.Docker[0].VersionRange) == 2
			},
			want: true,
		},
		{
			name: "ComponentsversionGlobal not exist",
			args: func() bool {
				controller := &Controller{
					Client:                    newFakeClient(),
					ClientSet:                 clientsetfake.NewSimpleClientset(),
					LocalArtifactSetClientSet: localartifactsetv1alpha1fake.NewSimpleClientset(),
					InfoManifestClientSet:     manifestv1alpha1fake.NewSimpleClientset(),
				}
				offlineVersionData := localartifactsetv1alpha1.LocalArtifactSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "LocalArtifactSet",
						APIVersion: "kubean.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "offlineversion-1",
					},
					Spec: localartifactsetv1alpha1.Spec{
						Docker: []*localartifactsetv1alpha1.DockerInfo{
							{
								OS:           "redhat-7",
								VersionRange: []string{"20.1", "20.2"},
							},
						},
					},
				}

				controller.Client.Create(context.Background(), &offlineVersionData)
				controller.LocalArtifactSetClientSet.KubeanV1alpha1().LocalArtifactSets().Create(context.Background(), &offlineVersionData, metav1.CreateOptions{})

				result, _ := controller.Reconcile(context.Background(), controllerruntime.Request{NamespacedName: types.NamespacedName{Name: offlineVersionData.Name}})
				return result.RequeueAfter == Loop
			},
			want: true,
		},
		{
			name: "offlineVersion not found",
			args: func() bool {
				controller := &Controller{
					Client:                    newFakeClient(),
					ClientSet:                 clientsetfake.NewSimpleClientset(),
					LocalArtifactSetClientSet: localartifactsetv1alpha1fake.NewSimpleClientset(),
					InfoManifestClientSet:     manifestv1alpha1fake.NewSimpleClientset(),
				}
				result, _ := controller.Reconcile(context.Background(), controllerruntime.Request{NamespacedName: types.NamespacedName{Name: "offlineversion-1"}})
				return result.Requeue == false && result.RequeueAfter == 0
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.args() != test.want {
				t.Fatal()
			}
		})
	}
}

func TestStart(t *testing.T) {
	controller := &Controller{
		Client:                    newFakeClient(),
		ClientSet:                 clientsetfake.NewSimpleClientset(),
		LocalArtifactSetClientSet: localartifactsetv1alpha1fake.NewSimpleClientset(),
		InfoManifestClientSet:     manifestv1alpha1fake.NewSimpleClientset(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	controller.Start(ctx)
}

func TestSetupWithManager(t *testing.T) {
	controller := &Controller{
		Client:                    newFakeClient(),
		ClientSet:                 clientsetfake.NewSimpleClientset(),
		LocalArtifactSetClientSet: localartifactsetv1alpha1fake.NewSimpleClientset(),
		InfoManifestClientSet:     manifestv1alpha1fake.NewSimpleClientset(),
	}
	if controller.SetupWithManager(MockManager{}) != nil {
		t.Fatal()
	}
}

type MockClusterForManager struct {
	_ string
}

func (MockClusterForManager) SetFields(interface{}) error { return nil }

func (MockClusterForManager) GetConfig() *rest.Config { return &rest.Config{} }

func (MockClusterForManager) GetScheme() *runtime.Scheme {
	sch := scheme.Scheme
	if err := manifestv1alpha1.AddToScheme(sch); err != nil {
		panic(err)
	}
	if err := localartifactsetv1alpha1.AddToScheme(sch); err != nil {
		panic(err)
	}
	return sch
}

func (MockClusterForManager) GetClient() client.Client { return nil }

func (MockClusterForManager) GetFieldIndexer() client.FieldIndexer { return nil }

func (MockClusterForManager) GetCache() cache.Cache { return nil }

func (MockClusterForManager) GetEventRecorderFor(name string) record.EventRecorder { return nil }

func (MockClusterForManager) GetRESTMapper() meta.RESTMapper { return nil }

func (MockClusterForManager) GetAPIReader() client.Reader { return nil }

func (MockClusterForManager) Start(ctx context.Context) error { return nil }

type MockManager struct {
	MockClusterForManager
}

func (MockManager) Add(manager.Runnable) error { return nil }

func (MockManager) Elected() <-chan struct{} { return nil }

func (MockManager) AddMetricsExtraHandler(path string, handler http.Handler) error { return nil }

func (MockManager) AddHealthzCheck(name string, check healthz.Checker) error { return nil }

func (MockManager) AddReadyzCheck(name string, check healthz.Checker) error { return nil }

func (MockManager) Start(ctx context.Context) error { return nil }

func (MockManager) GetWebhookServer() *webhook.Server { return nil }

func (MockManager) GetLogger() logr.Logger { return logr.Logger{} }

func (MockManager) GetControllerOptions() v1alpha1.ControllerConfigurationSpec {
	return v1alpha1.ControllerConfigurationSpec{}
}
