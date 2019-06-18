/*
Copyright 2019 the Velero contributors.

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

package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"io/ioutil"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"

	velerov1 "github.com/heptio/velero/pkg/apis/velero/v1"
	"github.com/heptio/velero/pkg/client"
	"github.com/heptio/velero/pkg/discovery"
	"github.com/heptio/velero/pkg/generated/clientset/versioned/fake"
	"github.com/heptio/velero/pkg/kuberesource"
	"github.com/heptio/velero/pkg/plugin/velero"
	"github.com/heptio/velero/pkg/test"
	kubeutil "github.com/heptio/velero/pkg/util/kube"
	"github.com/heptio/velero/pkg/volume"
)

// TestBackupResourceFiltering runs backups with different combinations
// of resource filters (included/excluded resources, included/excluded
// namespaces, label selectors, "include cluster resources" flag), and
// verifies that the set of items written to the backup tarball are
// correct. Validation is done by looking at the names of the files in
// the backup tarball; the contents of the files are not checked.
func TestBackupResourceFiltering(t *testing.T) {
	tests := []struct {
		name         string
		backup       *velerov1.Backup
		apiResources []*apiResource
		want         []string
	}{
		{
			name:   "no filters backs up everything",
			backup: defaultBackup().Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/pods/namespaces/zoo/raz.json",
				"resources/deployments.apps/namespaces/foo/bar.json",
				"resources/deployments.apps/namespaces/zoo/raz.json",
			},
		},
		{
			name: "included resources filter only backs up resources of those types",
			backup: defaultBackup().
				IncludedResources("pods").
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/pods/namespaces/zoo/raz.json",
			},
		},
		{
			name: "excluded resources filter only backs up resources not of those types",
			backup: defaultBackup().
				ExcludedResources("deployments").
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/pods/namespaces/zoo/raz.json",
			},
		},
		{
			name: "included namespaces filter only backs up resources in those namespaces",
			backup: defaultBackup().
				IncludedNamespaces("foo").
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/deployments.apps/namespaces/foo/bar.json",
			},
		},
		{
			name: "excluded namespaces filter only backs up resources not in those namespaces",
			backup: defaultBackup().
				ExcludedNamespaces("zoo").
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/deployments.apps/namespaces/foo/bar.json",
			},
		},
		{
			name: "IncludeClusterResources=false only backs up namespaced resources",
			backup: defaultBackup().
				IncludeClusterResources(false).
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
				pvs(
					newPV("bar"),
					newPV("baz"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/pods/namespaces/zoo/raz.json",
				"resources/deployments.apps/namespaces/foo/bar.json",
				"resources/deployments.apps/namespaces/zoo/raz.json",
			},
		},
		{
			name: "label selector only backs up matching resources",
			backup: defaultBackup().
				LabelSelector(&metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}}).
				Backup(),
			apiResources: []*apiResource{
				pods(
					withLabel(newPod("foo", "bar"), "a", "b"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					withLabel(newDeployment("zoo", "raz"), "a", "b"),
				),
				pvs(
					withLabel(newPV("bar"), "a", "b"),
					withLabel(newPV("baz"), "a", "c"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/deployments.apps/namespaces/zoo/raz.json",
				"resources/persistentvolumes/cluster/bar.json",
			},
		},
		{
			name: "should include cluster-scoped resources if backing up subset of namespaces and IncludeClusterResources=true",
			backup: defaultBackup().
				IncludedNamespaces("ns-1", "ns-2").
				IncludeClusterResources(true).
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-1"),
					newPod("ns-3", "pod-1"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
				"resources/pods/namespaces/ns-2/pod-1.json",
				"resources/persistentvolumes/cluster/pv-1.json",
				"resources/persistentvolumes/cluster/pv-2.json",
			},
		},
		{
			name: "should not include cluster-scoped resource if backing up subset of namespaces and IncludeClusterResources=false",
			backup: defaultBackup().
				IncludedNamespaces("ns-1", "ns-2").
				IncludeClusterResources(false).
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-1"),
					newPod("ns-3", "pod-1"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
				"resources/pods/namespaces/ns-2/pod-1.json",
			},
		},
		{
			name: "should not include cluster-scoped resource if backing up subset of namespaces and IncludeClusterResources=nil",
			backup: defaultBackup().
				IncludedNamespaces("ns-1", "ns-2").
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-1"),
					newPod("ns-3", "pod-1"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
				"resources/pods/namespaces/ns-2/pod-1.json",
			},
		},
		{
			name: "should include cluster-scoped resources if backing up all namespaces and IncludeClusterResources=true",
			backup: defaultBackup().
				IncludeClusterResources(true).
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-1"),
					newPod("ns-3", "pod-1"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
				"resources/pods/namespaces/ns-2/pod-1.json",
				"resources/pods/namespaces/ns-3/pod-1.json",
				"resources/persistentvolumes/cluster/pv-1.json",
				"resources/persistentvolumes/cluster/pv-2.json",
			},
		},
		{
			name: "should not include cluster-scoped resources if backing up all namespaces and IncludeClusterResources=false",
			backup: defaultBackup().
				IncludeClusterResources(false).
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-1"),
					newPod("ns-3", "pod-1"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
				"resources/pods/namespaces/ns-2/pod-1.json",
				"resources/pods/namespaces/ns-3/pod-1.json",
			},
		},
		{
			name: "should include cluster-scoped resources if backing up all namespaces and IncludeClusterResources=nil",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-1"),
					newPod("ns-3", "pod-1"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
				"resources/pods/namespaces/ns-2/pod-1.json",
				"resources/pods/namespaces/ns-3/pod-1.json",
				"resources/persistentvolumes/cluster/pv-1.json",
				"resources/persistentvolumes/cluster/pv-2.json",
			},
		},
		{
			name: "when a wildcard and a specific resource are included, the wildcard takes precedence",
			backup: defaultBackup().
				IncludedResources("*", "pods").
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/pods/namespaces/zoo/raz.json",
				"resources/deployments.apps/namespaces/foo/bar.json",
				"resources/deployments.apps/namespaces/zoo/raz.json",
			},
		},
		{
			name: "wildcard excludes are ignored",
			backup: defaultBackup().
				ExcludedResources("*").
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/pods/namespaces/zoo/raz.json",
				"resources/deployments.apps/namespaces/foo/bar.json",
				"resources/deployments.apps/namespaces/zoo/raz.json",
			},
		},
		{
			name: "unresolvable included resources are ignored",
			backup: defaultBackup().
				IncludedResources("pods", "unresolvable").
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/pods/namespaces/zoo/raz.json",
			},
		},
		{
			name: "unresolvable excluded resources are ignored",
			backup: defaultBackup().
				ExcludedResources("deployments", "unresolvable").
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/pods/namespaces/foo/bar.json",
				"resources/pods/namespaces/zoo/raz.json",
			},
		},
		{
			name:   "terminating resources are not backed up",
			backup: defaultBackup().Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-2", Name: "pod-2", DeletionTimestamp: &metav1.Time{Time: time.Now()}}},
				),
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				h          = newHarness(t)
				req        = &Request{Backup: tc.backup}
				backupFile = bytes.NewBuffer([]byte{})
			)

			for _, resource := range tc.apiResources {
				h.addItems(t, resource.group, resource.version, resource.name, resource.shortName, resource.namespaced, resource.items...)
			}

			h.backupper.Backup(h.log, req, backupFile, nil, nil)

			assertTarballContents(t, backupFile, append(tc.want, "metadata/version")...)
		})
	}
}

// TestBackupResourceCohabitation runs backups for resources that "cohabitate",
// meaning they exist in multiple API groups (e.g. deployments.extensions and
// deployments.apps), and verifies that only one copy of each resource is backed
// up, with preference for the non-"extensions" API group.
func TestBackupResourceCohabitation(t *testing.T) {
	tests := []struct {
		name         string
		backup       *velerov1.Backup
		apiResources []*apiResource
		want         []string
	}{
		{
			name:   "when deployments exist only in extensions, they're backed up",
			backup: defaultBackup().Backup(),
			apiResources: []*apiResource{
				extensionsDeployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/deployments.extensions/namespaces/foo/bar.json",
				"resources/deployments.extensions/namespaces/zoo/raz.json",
			},
		},
		{
			name:   "when deployments exist in both apps and extensions, only apps/deployments are backed up",
			backup: defaultBackup().Backup(),
			apiResources: []*apiResource{
				extensionsDeployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
				deployments(
					newDeployment("foo", "bar"),
					newDeployment("zoo", "raz"),
				),
			},
			want: []string{
				"resources/deployments.apps/namespaces/foo/bar.json",
				"resources/deployments.apps/namespaces/zoo/raz.json",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				h          = newHarness(t)
				req        = &Request{Backup: tc.backup}
				backupFile = bytes.NewBuffer([]byte{})
			)

			for _, resource := range tc.apiResources {
				h.addItems(t, resource.group, resource.version, resource.name, resource.shortName, resource.namespaced, resource.items...)
			}

			h.backupper.Backup(h.log, req, backupFile, nil, nil)

			assertTarballContents(t, backupFile, append(tc.want, "metadata/version")...)
		})
	}
}

// TestBackupUsesNewCohabitatingResourcesForEachBackup ensures that when two backups are
// run that each include cohabitating resources, one copy of the relevant resources is
// backed up in each backup. Verification is done by looking at the contents of the backup
// tarball. This covers a specific issue that was fixed by https://github.com/heptio/velero/pull/485.
func TestBackupUsesNewCohabitatingResourcesForEachBackup(t *testing.T) {
	h := newHarness(t)

	// run and verify backup 1
	backup1 := &Request{
		Backup: defaultBackup().Backup(),
	}
	backup1File := bytes.NewBuffer([]byte{})

	h.addItems(t, "apps", "v1", "deployments", "deploys", true, newDeployment("ns-1", "deploy-1"))
	h.addItems(t, "extensions", "v1", "deployments", "deploys", true, newDeployment("ns-1", "deploy-1"))

	h.backupper.Backup(h.log, backup1, backup1File, nil, nil)

	assertTarballContents(t, backup1File, "metadata/version", "resources/deployments.apps/namespaces/ns-1/deploy-1.json")

	// run and verify backup 2
	backup2 := &Request{
		Backup: defaultBackup().Backup(),
	}
	backup2File := bytes.NewBuffer([]byte{})

	h.backupper.Backup(h.log, backup2, backup2File, nil, nil)

	assertTarballContents(t, backup2File, "metadata/version", "resources/deployments.apps/namespaces/ns-1/deploy-1.json")
}

// TestBackupResourceOrdering runs backups of the core API group and ensures that items are backed
// up in the expected order (pods, PVCs, PVs, everything else). Verification is done by looking
// at the order of files written to the backup tarball.
func TestBackupResourceOrdering(t *testing.T) {
	tests := []struct {
		name         string
		backup       *velerov1.Backup
		apiResources []*apiResource
	}{
		{
			name: "core API group: pods come before pvcs, pvcs come before pvs, pvs come before anything else",
			backup: defaultBackup().
				SnapshotVolumes(false).
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				pvcs(
					newPVC("foo", "bar"),
					newPVC("zoo", "raz"),
				),
				pvs(
					newPV("bar"),
					newPV("baz"),
				),
				secrets(
					newSecret("foo", "bar"),
					newSecret("zoo", "raz"),
				),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				h          = newHarness(t)
				req        = &Request{Backup: tc.backup}
				backupFile = bytes.NewBuffer([]byte{})
			)

			for _, resource := range tc.apiResources {
				h.addItems(t, resource.group, resource.version, resource.name, resource.shortName, resource.namespaced, resource.items...)
			}

			h.backupper.Backup(h.log, req, backupFile, nil, nil)

			assertTarballOrdering(t, backupFile, "pods", "persistentvolumeclaims", "persistentvolumes")
		})
	}
}

// recordResourcesAction is a backup item action that can be configured
// to run for specific resources/namespaces and simply records the items
// that it is executed for.
type recordResourcesAction struct {
	selector        velero.ResourceSelector
	ids             []string
	backups         []velerov1.Backup
	additionalItems []velero.ResourceIdentifier
}

func (a *recordResourcesAction) Execute(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	metadata, err := meta.Accessor(item)
	if err != nil {
		return item, a.additionalItems, err
	}
	a.ids = append(a.ids, kubeutil.NamespaceAndName(metadata))
	a.backups = append(a.backups, *backup)

	return item, a.additionalItems, nil
}

func (a *recordResourcesAction) AppliesTo() (velero.ResourceSelector, error) {
	return a.selector, nil
}

func (a *recordResourcesAction) ForResource(resource string) *recordResourcesAction {
	a.selector.IncludedResources = append(a.selector.IncludedResources, resource)
	return a
}

func (a *recordResourcesAction) ForNamespace(namespace string) *recordResourcesAction {
	a.selector.IncludedNamespaces = append(a.selector.IncludedNamespaces, namespace)
	return a
}

func (a *recordResourcesAction) ForLabelSelector(selector string) *recordResourcesAction {
	a.selector.LabelSelector = selector
	return a
}

func (a *recordResourcesAction) WithAdditionalItems(items []velero.ResourceIdentifier) *recordResourcesAction {
	a.additionalItems = items
	return a
}

// TestBackupActionsRunsForCorrectItems runs backups with backup item actions, and
// verifies that each backup item action is run for the correct set of resources based on its
// AppliesTo() resource selector. Verification is done by using the recordResourcesAction struct,
// which records which resources it's executed for.
func TestBackupActionsRunForCorrectItems(t *testing.T) {
	tests := []struct {
		name         string
		backup       *velerov1.Backup
		apiResources []*apiResource

		// actions is a map from a recordResourcesAction (which will record the items it was called for)
		// to a slice of expected items, formatted as {namespace}/{name}.
		actions map[*recordResourcesAction][]string
	}{
		{
			name: "single action with no selector runs for all items",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			actions: map[*recordResourcesAction][]string{
				new(recordResourcesAction): {"ns-1/pod-1", "ns-2/pod-2", "pv-1", "pv-2"},
			},
		},
		{
			name: "single action with a resource selector for namespaced resources runs only for matching resources",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			actions: map[*recordResourcesAction][]string{
				new(recordResourcesAction).ForResource("pods"): {"ns-1/pod-1", "ns-2/pod-2"},
			},
		},
		{
			name: "single action with a resource selector for cluster-scoped resources runs only for matching resources",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			actions: map[*recordResourcesAction][]string{
				new(recordResourcesAction).ForResource("persistentvolumes"): {"pv-1", "pv-2"},
			},
		},
		{
			// TODO this seems like a bug
			name: "single action with a namespace selector runs for resources in that namespace plus cluster-scoped resources",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
				),
				pvcs(
					newPVC("ns-1", "pvc-1"),
					newPVC("ns-2", "pvc-2"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			actions: map[*recordResourcesAction][]string{
				new(recordResourcesAction).ForNamespace("ns-1"): {"ns-1/pod-1", "ns-1/pvc-1", "pv-1", "pv-2"},
			},
		},
		{
			name: "single action with a resource and namespace selector runs only for matching resources",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			actions: map[*recordResourcesAction][]string{
				new(recordResourcesAction).ForResource("pods").ForNamespace("ns-1"): {"ns-1/pod-1"},
			},
		},
		{
			name: "multiple actions, each with a different resource selector using short name, run for matching resources",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			actions: map[*recordResourcesAction][]string{
				new(recordResourcesAction).ForResource("po"): {"ns-1/pod-1", "ns-2/pod-2"},
				new(recordResourcesAction).ForResource("pv"): {"pv-1", "pv-2"},
			},
		},
		{
			name: "actions with selectors that don't match anything don't run for any resources",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
				),
				pvcs(
					newPVC("ns-2", "pvc-2"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			actions: map[*recordResourcesAction][]string{
				new(recordResourcesAction).ForNamespace("ns-1").ForResource("persistentvolumeclaims"): nil,
				new(recordResourcesAction).ForNamespace("ns-2").ForResource("pods"):                   nil,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				h          = newHarness(t)
				req        = &Request{Backup: tc.backup}
				backupFile = bytes.NewBuffer([]byte{})
			)

			for _, resource := range tc.apiResources {
				h.addItems(t, resource.group, resource.version, resource.name, resource.shortName, resource.namespaced, resource.items...)
			}

			actions := []velero.BackupItemAction{}
			for action := range tc.actions {
				actions = append(actions, action)
			}

			err := h.backupper.Backup(h.log, req, backupFile, actions, nil)
			assert.NoError(t, err)

			for action, want := range tc.actions {
				assert.Equal(t, want, action.ids)
			}
		})
	}
}

// TestBackupWithInvalidActions runs backups with backup item actions that are invalid
// in some way (e.g. an invalid label selector returned from AppliesTo(), an error returned
// from AppliesTo()) and verifies that this causes the backupper.Backup(...) method to
// return an error.
func TestBackupWithInvalidActions(t *testing.T) {
	// all test cases in this function are expected to cause the method under test
	// to return an error, so no expected results need to be set up.
	tests := []struct {
		name         string
		backup       *velerov1.Backup
		apiResources []*apiResource
		actions      []velero.BackupItemAction
	}{
		{
			name: "action with invalid label selector results in an error",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				pvs(
					newPV("bar"),
					newPV("baz"),
				),
			},
			actions: []velero.BackupItemAction{
				new(recordResourcesAction).ForLabelSelector("=invalid-selector"),
			},
		},
		{
			name: "action returning an error from AppliesTo results in an error",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("foo", "bar"),
					newPod("zoo", "raz"),
				),
				pvs(
					newPV("bar"),
					newPV("baz"),
				),
			},
			actions: []velero.BackupItemAction{
				&appliesToErrorAction{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				h          = newHarness(t)
				req        = &Request{Backup: tc.backup}
				backupFile = bytes.NewBuffer([]byte{})
			)

			for _, resource := range tc.apiResources {
				h.addItems(t, resource.group, resource.version, resource.name, resource.shortName, resource.namespaced, resource.items...)
			}

			assert.Error(t, h.backupper.Backup(h.log, req, backupFile, tc.actions, nil))
		})
	}
}

// appliesToErrorAction is a backup item action that always returns
// an error when AppliesTo() is called.
type appliesToErrorAction struct{}

func (a *appliesToErrorAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{}, errors.New("error calling AppliesTo")
}

func (a *appliesToErrorAction) Execute(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	panic("not implemented")
}

// TestBackupActionModifications runs backups with backup item actions that make modifications
// to items in their Execute(...) methods and verifies that these modifications are
// persisted to the backup tarball. Verification is done by inspecting the file contents
// of the tarball.
func TestBackupActionModifications(t *testing.T) {
	// modifyingActionGetter is a helper function that returns a *pluggableAction, whose Execute(...)
	// method modifies the item being passed in by calling the 'modify' function on it.
	modifyingActionGetter := func(modify func(*unstructured.Unstructured)) *pluggableAction {
		return &pluggableAction{
			executeFunc: func(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
				obj, ok := item.(*unstructured.Unstructured)
				if !ok {
					return nil, nil, errors.Errorf("unexpected type %T", item)
				}

				res := obj.DeepCopy()
				modify(res)

				return res, nil, nil
			},
		}
	}

	tests := []struct {
		name         string
		backup       *velerov1.Backup
		apiResources []*apiResource
		actions      []velero.BackupItemAction
		want         map[string]unstructuredObject
	}{
		{
			name:   "action that adds a label to item gets persisted",
			backup: defaultBackup().Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
				),
			},
			actions: []velero.BackupItemAction{
				modifyingActionGetter(func(item *unstructured.Unstructured) {
					item.SetLabels(map[string]string{"updated": "true"})
				}),
			},
			want: map[string]unstructuredObject{
				"resources/pods/namespaces/ns-1/pod-1.json": toUnstructuredOrFail(t, withLabel(newPod("ns-1", "pod-1"), "updated", "true")),
			},
		},
		{
			name:   "action that removes labels from item gets persisted",
			backup: defaultBackup().Backup(),
			apiResources: []*apiResource{
				pods(
					withLabel(newPod("ns-1", "pod-1"), "should-be-removed", "true"),
				),
			},
			actions: []velero.BackupItemAction{
				modifyingActionGetter(func(item *unstructured.Unstructured) {
					item.SetLabels(nil)
				}),
			},
			want: map[string]unstructuredObject{
				"resources/pods/namespaces/ns-1/pod-1.json": toUnstructuredOrFail(t, newPod("ns-1", "pod-1")),
			},
		},
		{
			name:   "action that sets a spec field on item gets persisted",
			backup: defaultBackup().Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
				),
			},
			actions: []velero.BackupItemAction{
				modifyingActionGetter(func(item *unstructured.Unstructured) {
					item.Object["spec"].(map[string]interface{})["nodeName"] = "foo"
				}),
			},
			want: map[string]unstructuredObject{
				"resources/pods/namespaces/ns-1/pod-1.json": toUnstructuredOrFail(t, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-1", Name: "pod-1"}, Spec: corev1.PodSpec{NodeName: "foo"}}),
			},
		},
		{
			// TODO this seems like a bug
			name: "modifications to name and namespace in an action are persisted in JSON but not in filename",
			backup: defaultBackup().
				Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
				),
			},
			actions: []velero.BackupItemAction{
				modifyingActionGetter(func(item *unstructured.Unstructured) {
					item.SetName(item.GetName() + "-updated")
					item.SetNamespace(item.GetNamespace() + "-updated")
				}),
			},
			want: map[string]unstructuredObject{
				"resources/pods/namespaces/ns-1/pod-1.json": toUnstructuredOrFail(t, newPod("ns-1-updated", "pod-1-updated")),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				h          = newHarness(t)
				req        = &Request{Backup: tc.backup}
				backupFile = bytes.NewBuffer([]byte{})
			)

			for _, resource := range tc.apiResources {
				h.addItems(t, resource.group, resource.version, resource.name, resource.shortName, resource.namespaced, resource.items...)
			}

			err := h.backupper.Backup(h.log, req, backupFile, tc.actions, nil)
			assert.NoError(t, err)

			assertTarballFileContents(t, backupFile, tc.want)
		})
	}

}

// TestBackupActionAdditionalItems runs backups with backup item actions that return
// additional items to be backed up, and verifies that those items are included in the
// backup tarball as appropriate. Verification is done by looking at the files that exist
// in the backup tarball.
func TestBackupActionAdditionalItems(t *testing.T) {
	tests := []struct {
		name         string
		backup       *velerov1.Backup
		apiResources []*apiResource
		actions      []velero.BackupItemAction
		want         []string
	}{
		{
			name:   "additional items that are already being backed up are not backed up twice",
			backup: defaultBackup().Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
					newPod("ns-3", "pod-3"),
				),
			},
			actions: []velero.BackupItemAction{
				&pluggableAction{
					selector: velero.ResourceSelector{IncludedNamespaces: []string{"ns-1"}},
					executeFunc: func(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
						additionalItems := []velero.ResourceIdentifier{
							{GroupResource: kuberesource.Pods, Namespace: "ns-2", Name: "pod-2"},
							{GroupResource: kuberesource.Pods, Namespace: "ns-3", Name: "pod-3"},
						}

						return item, additionalItems, nil
					},
				},
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
				"resources/pods/namespaces/ns-2/pod-2.json",
				"resources/pods/namespaces/ns-3/pod-3.json",
			},
		},
		{
			name:   "when using a backup namespace filter, additional items that are in a non-included namespace are not backed up",
			backup: defaultBackup().IncludedNamespaces("ns-1").Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
					newPod("ns-3", "pod-3"),
				),
			},
			actions: []velero.BackupItemAction{
				&pluggableAction{
					executeFunc: func(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
						additionalItems := []velero.ResourceIdentifier{
							{GroupResource: kuberesource.Pods, Namespace: "ns-2", Name: "pod-2"},
							{GroupResource: kuberesource.Pods, Namespace: "ns-3", Name: "pod-3"},
						}

						return item, additionalItems, nil
					},
				},
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
			},
		},
		{
			name:   "when using a backup namespace filter, additional items that are cluster-scoped are backed up",
			backup: defaultBackup().IncludedNamespaces("ns-1").Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			actions: []velero.BackupItemAction{
				&pluggableAction{
					executeFunc: func(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
						additionalItems := []velero.ResourceIdentifier{
							{GroupResource: kuberesource.PersistentVolumes, Name: "pv-1"},
							{GroupResource: kuberesource.PersistentVolumes, Name: "pv-2"},
						}

						return item, additionalItems, nil
					},
				},
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
				"resources/persistentvolumes/cluster/pv-1.json",
				"resources/persistentvolumes/cluster/pv-2.json",
			},
		},
		{
			name:   "when using a backup resource filter, additional items that are non-included resources are not backed up",
			backup: defaultBackup().IncludedResources("pods").Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			actions: []velero.BackupItemAction{
				&pluggableAction{
					executeFunc: func(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
						additionalItems := []velero.ResourceIdentifier{
							{GroupResource: kuberesource.PersistentVolumes, Name: "pv-1"},
							{GroupResource: kuberesource.PersistentVolumes, Name: "pv-2"},
						}

						return item, additionalItems, nil
					},
				},
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
			},
		},
		{
			name:   "when IncludeClusterResources=false, additional items that are cluster-scoped are not backed up",
			backup: defaultBackup().IncludeClusterResources(false).Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
				),
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			actions: []velero.BackupItemAction{
				&pluggableAction{
					executeFunc: func(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
						additionalItems := []velero.ResourceIdentifier{
							{GroupResource: kuberesource.PersistentVolumes, Name: "pv-1"},
							{GroupResource: kuberesource.PersistentVolumes, Name: "pv-2"},
						}

						return item, additionalItems, nil
					},
				},
			},
			want: []string{
				"resources/pods/namespaces/ns-1/pod-1.json",
				"resources/pods/namespaces/ns-2/pod-2.json",
			},
		},
		{
			name:   "if there's an error backing up additional items, the item the action was run for isn't backed up",
			backup: defaultBackup().Backup(),
			apiResources: []*apiResource{
				pods(
					newPod("ns-1", "pod-1"),
					newPod("ns-2", "pod-2"),
					newPod("ns-3", "pod-3"),
				),
			},
			actions: []velero.BackupItemAction{
				&pluggableAction{
					selector: velero.ResourceSelector{IncludedNamespaces: []string{"ns-1"}},
					executeFunc: func(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
						additionalItems := []velero.ResourceIdentifier{
							{GroupResource: kuberesource.Pods, Namespace: "ns-4", Name: "pod-4"},
							{GroupResource: kuberesource.Pods, Namespace: "ns-5", Name: "pod-5"},
						}

						return item, additionalItems, nil
					},
				},
			},
			want: []string{
				"resources/pods/namespaces/ns-2/pod-2.json",
				"resources/pods/namespaces/ns-3/pod-3.json",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				h          = newHarness(t)
				req        = &Request{Backup: tc.backup}
				backupFile = bytes.NewBuffer([]byte{})
			)

			for _, resource := range tc.apiResources {
				h.addItems(t, resource.group, resource.version, resource.name, resource.shortName, resource.namespaced, resource.items...)
			}

			err := h.backupper.Backup(h.log, req, backupFile, tc.actions, nil)
			assert.NoError(t, err)

			assertTarballContents(t, backupFile, append(tc.want, "metadata/version")...)
		})
	}
}

// volumeSnapshotterGetter is a simple implementation of the VolumeSnapshotterGetter
// interface that returns velero.VolumeSnapshotters from a map if they exist.
type volumeSnapshotterGetter map[string]velero.VolumeSnapshotter

func (vsg volumeSnapshotterGetter) GetVolumeSnapshotter(name string) (velero.VolumeSnapshotter, error) {
	snapshotter, ok := vsg[name]
	if !ok {
		return nil, errors.New("volume snapshotter not found")
	}

	return snapshotter, nil
}

func int64Ptr(val int) *int64 {
	i := int64(val)
	return &i
}

type volumeIdentifier struct {
	volumeID string
	volumeAZ string
}

type volumeInfo struct {
	volumeType  string
	iops        *int64
	snapshotErr bool
}

// fakeVolumeSnapshotter is a test fake for the velero.VolumeSnapshotter interface.
type fakeVolumeSnapshotter struct {
	// PVVolumeNames is a map from PV name to volume ID, used as the basis
	// for the GetVolumeID method.
	PVVolumeNames map[string]string

	// Volumes is a map from volume identifier (volume ID + AZ) to a struct
	// of volume info, used for the GetVolumeInfo and CreateSnapshot methods.
	Volumes map[volumeIdentifier]*volumeInfo
}

// WithVolume is a test helper for registering persistent volumes that the
// fakeVolumeSnapshotter should handle.
func (vs *fakeVolumeSnapshotter) WithVolume(pvName, id, az, volumeType string, iops int, snapshotErr bool) *fakeVolumeSnapshotter {
	if vs.PVVolumeNames == nil {
		vs.PVVolumeNames = make(map[string]string)
	}
	vs.PVVolumeNames[pvName] = id

	if vs.Volumes == nil {
		vs.Volumes = make(map[volumeIdentifier]*volumeInfo)
	}

	identifier := volumeIdentifier{
		volumeID: id,
		volumeAZ: az,
	}

	vs.Volumes[identifier] = &volumeInfo{
		volumeType:  volumeType,
		iops:        int64Ptr(iops),
		snapshotErr: snapshotErr,
	}

	return vs
}

// Init is a no-op.
func (*fakeVolumeSnapshotter) Init(config map[string]string) error {
	return nil
}

// GetVolumeID looks up the PV name in the PVVolumeNames map and returns the result
// if found, or an error otherwise.
func (vs *fakeVolumeSnapshotter) GetVolumeID(pv runtime.Unstructured) (string, error) {
	obj := pv.(*unstructured.Unstructured)

	volumeID, ok := vs.PVVolumeNames[obj.GetName()]
	if !ok {
		return "", errors.New("unsupported volume type")
	}

	return volumeID, nil
}

// CreateSnapshot looks up the volume in the Volume map. If it's not found, an error is
// returned; if snapshotErr is true on the result, an error is returned; otherwise,
// a snapshotID of "<volumeID>-snapshot" is returned.
func (vs *fakeVolumeSnapshotter) CreateSnapshot(volumeID, volumeAZ string, tags map[string]string) (snapshotID string, err error) {
	vi, ok := vs.Volumes[volumeIdentifier{volumeID: volumeID, volumeAZ: volumeAZ}]
	if !ok {
		return "", errors.New("volume not found")
	}

	if vi.snapshotErr {
		return "", errors.New("error calling CreateSnapshot")
	}

	return volumeID + "-snapshot", nil
}

// GetVolumeInfo returns volume info if it exists in the Volumes map
// for the specified volume ID and AZ, or an error otherwise.
func (vs *fakeVolumeSnapshotter) GetVolumeInfo(volumeID, volumeAZ string) (string, *int64, error) {
	vi, ok := vs.Volumes[volumeIdentifier{volumeID: volumeID, volumeAZ: volumeAZ}]
	if !ok {
		return "", nil, errors.New("volume not found")
	}

	return vi.volumeType, vi.iops, nil
}

// CreateVolumeFromSnapshot panics because it's not expected to be used for backups.
func (*fakeVolumeSnapshotter) CreateVolumeFromSnapshot(snapshotID, volumeType, volumeAZ string, iops *int64) (volumeID string, err error) {
	panic("CreateVolumeFromSnapshot should not be used for backups")
}

// SetVolumeID panics because it's not expected to be used for backups.
func (*fakeVolumeSnapshotter) SetVolumeID(pv runtime.Unstructured, volumeID string) (runtime.Unstructured, error) {
	panic("SetVolumeID should not be used for backups")
}

// DeleteSnapshot panics because it's not expected to be used for backups.
func (*fakeVolumeSnapshotter) DeleteSnapshot(snapshotID string) error {
	panic("DeleteSnapshot should not be used for backups")
}

// TestBackupWithSnapshots runs backups with volume snapshot locations and volume snapshotters
// configured and verifies that snapshots are created as appropriate. Verification is done by
// looking at the backup request's VolumeSnapshots field. This test uses the fakeVolumeSnapshotter
// struct in place of real volume snapshotters.
func TestBackupWithSnapshots(t *testing.T) {
	tests := []struct {
		name              string
		req               *Request
		vsls              []*velerov1.VolumeSnapshotLocation
		apiResources      []*apiResource
		snapshotterGetter volumeSnapshotterGetter
		want              []*volume.Snapshot
	}{
		{
			name: "persistent volume with no zone annotation creates a snapshot",
			req: &Request{
				Backup: defaultBackup().Backup(),
				SnapshotLocations: []*velerov1.VolumeSnapshotLocation{
					newSnapshotLocation("velero", "default", "default"),
				},
			},
			apiResources: []*apiResource{
				pvs(
					newPV("pv-1"),
				),
			},
			snapshotterGetter: map[string]velero.VolumeSnapshotter{
				"default": new(fakeVolumeSnapshotter).WithVolume("pv-1", "vol-1", "", "type-1", 100, false),
			},
			want: []*volume.Snapshot{
				{
					Spec: volume.SnapshotSpec{
						BackupName:           "backup-1",
						Location:             "default",
						PersistentVolumeName: "pv-1",
						ProviderVolumeID:     "vol-1",
						VolumeType:           "type-1",
						VolumeIOPS:           int64Ptr(100),
					},
					Status: volume.SnapshotStatus{
						Phase:              volume.SnapshotPhaseCompleted,
						ProviderSnapshotID: "vol-1-snapshot",
					},
				},
			},
		},
		{
			name: "persistent volume with zone annotation creates a snapshot",
			req: &Request{
				Backup: defaultBackup().Backup(),
				SnapshotLocations: []*velerov1.VolumeSnapshotLocation{
					newSnapshotLocation("velero", "default", "default"),
				},
			},
			apiResources: []*apiResource{
				pvs(
					withLabel(newPV("pv-1"), "failure-domain.beta.kubernetes.io/zone", "zone-1"),
				),
			},
			snapshotterGetter: map[string]velero.VolumeSnapshotter{
				"default": new(fakeVolumeSnapshotter).WithVolume("pv-1", "vol-1", "zone-1", "type-1", 100, false),
			},
			want: []*volume.Snapshot{
				{
					Spec: volume.SnapshotSpec{
						BackupName:           "backup-1",
						Location:             "default",
						PersistentVolumeName: "pv-1",
						ProviderVolumeID:     "vol-1",
						VolumeAZ:             "zone-1",
						VolumeType:           "type-1",
						VolumeIOPS:           int64Ptr(100),
					},
					Status: volume.SnapshotStatus{
						Phase:              volume.SnapshotPhaseCompleted,
						ProviderSnapshotID: "vol-1-snapshot",
					},
				},
			},
		},
		{
			name: "error returned from CreateSnapshot results in a failed snapshot",
			req: &Request{
				Backup: defaultBackup().Backup(),
				SnapshotLocations: []*velerov1.VolumeSnapshotLocation{
					newSnapshotLocation("velero", "default", "default"),
				},
			},
			apiResources: []*apiResource{
				pvs(
					newPV("pv-1"),
				),
			},
			snapshotterGetter: map[string]velero.VolumeSnapshotter{
				"default": new(fakeVolumeSnapshotter).WithVolume("pv-1", "vol-1", "", "type-1", 100, true),
			},
			want: []*volume.Snapshot{
				{
					Spec: volume.SnapshotSpec{
						BackupName:           "backup-1",
						Location:             "default",
						PersistentVolumeName: "pv-1",
						ProviderVolumeID:     "vol-1",
						VolumeType:           "type-1",
						VolumeIOPS:           int64Ptr(100),
					},
					Status: volume.SnapshotStatus{
						Phase: volume.SnapshotPhaseFailed,
					},
				},
			},
		},
		{
			name: "backup with SnapshotVolumes=false does not create any snapshots",
			req: &Request{
				Backup: defaultBackup().SnapshotVolumes(false).Backup(),
				SnapshotLocations: []*velerov1.VolumeSnapshotLocation{
					newSnapshotLocation("velero", "default", "default"),
				},
			},
			apiResources: []*apiResource{
				pvs(
					newPV("pv-1"),
				),
			},
			snapshotterGetter: map[string]velero.VolumeSnapshotter{
				"default": new(fakeVolumeSnapshotter).WithVolume("pv-1", "vol-1", "", "type-1", 100, false),
			},
			want: nil,
		},
		{
			name: "backup with no volume snapshot locations does not create any snapshots",
			req: &Request{
				Backup: defaultBackup().Backup(),
			},
			apiResources: []*apiResource{
				pvs(
					newPV("pv-1"),
				),
			},
			snapshotterGetter: map[string]velero.VolumeSnapshotter{
				"default": new(fakeVolumeSnapshotter).WithVolume("pv-1", "vol-1", "", "type-1", 100, false),
			},
			want: nil,
		},
		{
			name: "backup with no volume snapshotters does not create any snapshots",
			req: &Request{
				Backup: defaultBackup().Backup(),
				SnapshotLocations: []*velerov1.VolumeSnapshotLocation{
					newSnapshotLocation("velero", "default", "default"),
				},
			},
			apiResources: []*apiResource{
				pvs(
					newPV("pv-1"),
				),
			},
			snapshotterGetter: map[string]velero.VolumeSnapshotter{},
			want:              nil,
		},
		{
			name: "unsupported persistent volume type does not create any snapshots",
			req: &Request{
				Backup: defaultBackup().Backup(),
				SnapshotLocations: []*velerov1.VolumeSnapshotLocation{
					newSnapshotLocation("velero", "default", "default"),
				},
			},
			apiResources: []*apiResource{
				pvs(
					newPV("pv-1"),
				),
			},
			snapshotterGetter: map[string]velero.VolumeSnapshotter{
				"default": new(fakeVolumeSnapshotter),
			},
			want: nil,
		},
		{
			name: "when there are multiple volumes, snapshot locations, and snapshotters, volumes are matched to the right snapshotters",
			req: &Request{
				Backup: defaultBackup().Backup(),
				SnapshotLocations: []*velerov1.VolumeSnapshotLocation{
					newSnapshotLocation("velero", "default", "default"),
					newSnapshotLocation("velero", "another", "another"),
				},
			},
			apiResources: []*apiResource{
				pvs(
					newPV("pv-1"),
					newPV("pv-2"),
				),
			},
			snapshotterGetter: map[string]velero.VolumeSnapshotter{
				"default": new(fakeVolumeSnapshotter).WithVolume("pv-1", "vol-1", "", "type-1", 100, false),
				"another": new(fakeVolumeSnapshotter).WithVolume("pv-2", "vol-2", "", "type-2", 100, false),
			},
			want: []*volume.Snapshot{
				{
					Spec: volume.SnapshotSpec{
						BackupName:           "backup-1",
						Location:             "default",
						PersistentVolumeName: "pv-1",
						ProviderVolumeID:     "vol-1",
						VolumeType:           "type-1",
						VolumeIOPS:           int64Ptr(100),
					},
					Status: volume.SnapshotStatus{
						Phase:              volume.SnapshotPhaseCompleted,
						ProviderSnapshotID: "vol-1-snapshot",
					},
				},
				{
					Spec: volume.SnapshotSpec{
						BackupName:           "backup-1",
						Location:             "another",
						PersistentVolumeName: "pv-2",
						ProviderVolumeID:     "vol-2",
						VolumeType:           "type-2",
						VolumeIOPS:           int64Ptr(100),
					},
					Status: volume.SnapshotStatus{
						Phase:              volume.SnapshotPhaseCompleted,
						ProviderSnapshotID: "vol-2-snapshot",
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				h          = newHarness(t)
				backupFile = bytes.NewBuffer([]byte{})
			)

			for _, resource := range tc.apiResources {
				h.addItems(t, resource.group, resource.version, resource.name, resource.shortName, resource.namespaced, resource.items...)
			}

			err := h.backupper.Backup(h.log, tc.req, backupFile, nil, tc.snapshotterGetter)
			assert.NoError(t, err)

			assert.Equal(t, tc.want, tc.req.VolumeSnapshots)
		})
	}
}

// pluggableAction is a backup item action that can be plugged with an Execute
// function body at runtime.
type pluggableAction struct {
	selector    velero.ResourceSelector
	executeFunc func(runtime.Unstructured, *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error)
}

func (a *pluggableAction) Execute(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	if a.executeFunc == nil {
		return item, nil, nil
	}

	return a.executeFunc(item, backup)
}

func (a *pluggableAction) AppliesTo() (velero.ResourceSelector, error) {
	return a.selector, nil
}

type apiResource struct {
	group      string
	version    string
	name       string
	shortName  string
	namespaced bool
	items      []metav1.Object
}

func pods(items ...metav1.Object) *apiResource {
	return &apiResource{
		group:      "",
		version:    "v1",
		name:       "pods",
		shortName:  "po",
		namespaced: true,
		items:      items,
	}
}

func pvcs(items ...metav1.Object) *apiResource {
	return &apiResource{
		group:      "",
		version:    "v1",
		name:       "persistentvolumeclaims",
		shortName:  "pvc",
		namespaced: true,
		items:      items,
	}
}

func secrets(items ...metav1.Object) *apiResource {
	return &apiResource{
		group:      "",
		version:    "v1",
		name:       "secrets",
		shortName:  "secrets",
		namespaced: true,
		items:      items,
	}
}

func deployments(items ...metav1.Object) *apiResource {
	return &apiResource{
		group:      "apps",
		version:    "v1",
		name:       "deployments",
		shortName:  "deploy",
		namespaced: true,
		items:      items,
	}
}

func extensionsDeployments(items ...metav1.Object) *apiResource {
	return &apiResource{
		group:      "extensions",
		version:    "v1",
		name:       "deployments",
		shortName:  "deploy",
		namespaced: true,
		items:      items,
	}
}

func pvs(items ...metav1.Object) *apiResource {
	return &apiResource{
		group:      "",
		version:    "v1",
		name:       "persistentvolumes",
		shortName:  "pv",
		namespaced: false,
		items:      items,
	}
}

type harness struct {
	veleroClient    *fake.Clientset
	kubeClient      *kubefake.Clientset
	dynamicClient   *dynamicfake.FakeDynamicClient
	discoveryClient *test.DiscoveryClient
	backupper       *kubernetesBackupper
	log             logrus.FieldLogger
}

func (h *harness) addItems(t *testing.T, group, version, resource, shortName string, namespaced bool, items ...metav1.Object) {
	t.Helper()

	h.discoveryClient.WithResource(group, version, resource, namespaced, shortName)
	require.NoError(t, h.backupper.discoveryHelper.Refresh())

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	for _, item := range items {
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(item)
		require.NoError(t, err)

		unstructuredObj := &unstructured.Unstructured{Object: obj}

		if namespaced {
			_, err = h.dynamicClient.Resource(gvr).Namespace(item.GetNamespace()).Create(unstructuredObj, metav1.CreateOptions{})
		} else {
			_, err = h.dynamicClient.Resource(gvr).Create(unstructuredObj, metav1.CreateOptions{})
		}
		require.NoError(t, err)
	}
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	// API server fakes
	var (
		veleroClient    = fake.NewSimpleClientset()
		kubeClient      = kubefake.NewSimpleClientset()
		dynamicClient   = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		discoveryClient = &test.DiscoveryClient{FakeDiscovery: kubeClient.Discovery().(*discoveryfake.FakeDiscovery)}
	)

	log := logrus.StandardLogger()

	discoveryHelper, err := discovery.NewHelper(discoveryClient, log)
	require.NoError(t, err)

	return &harness{
		veleroClient:    veleroClient,
		kubeClient:      kubeClient,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		backupper: &kubernetesBackupper{
			dynamicFactory:        client.NewDynamicFactory(dynamicClient),
			discoveryHelper:       discoveryHelper,
			groupBackupperFactory: new(defaultGroupBackupperFactory),

			// unsupported
			podCommandExecutor:     nil,
			resticBackupperFactory: nil,
			resticTimeout:          0,
		},
		log: log,
	}
}

func withLabel(obj metav1.Object, key, val string) metav1.Object {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = val
	obj.SetLabels(labels)

	return obj
}

func newPod(ns, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
}

func newPVC(ns, name string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
}

func newSecret(ns, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
}

func newDeployment(ns, name string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
}

func newPV(name string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func newSnapshotLocation(ns, name, provider string) *velerov1.VolumeSnapshotLocation {
	return &velerov1.VolumeSnapshotLocation{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: velerov1.VolumeSnapshotLocationSpec{
			Provider: provider,
		},
	}
}

func defaultBackup() *Builder {
	return NewNamedBuilder(velerov1.DefaultNamespace, "backup-1")
}

func toUnstructuredOrFail(t *testing.T, obj interface{}) map[string]interface{} {
	t.Helper()

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	require.NoError(t, err)

	return res
}

// assertTarballContents verifies that the gzipped tarball stored in the provided
// backupFile contains exactly the file names specified.
func assertTarballContents(t *testing.T, backupFile io.Reader, items ...string) {
	t.Helper()

	gzr, err := gzip.NewReader(backupFile)
	require.NoError(t, err)

	r := tar.NewReader(gzr)

	var files []string
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		files = append(files, hdr.Name)
	}

	sort.Strings(files)
	sort.Strings(items)
	assert.Equal(t, items, files)
}

// unstructuredObject is a type alias to improve readability.
type unstructuredObject map[string]interface{}

// assertTarballFileContents verifies that the gzipped tarball stored in the provided
// backupFile contains the files specified as keys in 'want', and for each of those
// files verifies that the content of the file is JSON and is equivalent to the JSON
// content stored as values in 'want'.
func assertTarballFileContents(t *testing.T, backupFile io.Reader, want map[string]unstructuredObject) {
	t.Helper()

	gzr, err := gzip.NewReader(backupFile)
	require.NoError(t, err)

	r := tar.NewReader(gzr)
	items := make(map[string][]byte)

	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		bytes, err := ioutil.ReadAll(r)
		require.NoError(t, err)

		items[hdr.Name] = bytes
	}

	for name, wantItem := range want {
		gotData, ok := items[name]
		assert.True(t, ok, "did not find item %s in tarball", name)
		if !ok {
			continue
		}

		// json-unmarshal the data from the tarball
		var got unstructuredObject
		err := json.Unmarshal(gotData, &got)
		assert.NoError(t, err)
		if err != nil {
			continue
		}

		assert.Equal(t, wantItem, got)
	}
}

// assertTarballOrdering ensures that resources were written to the tarball in the expected
// order. Any resources *not* in orderedResources are required to come *after* all resources
// in orderedResources, in any order.
func assertTarballOrdering(t *testing.T, backupFile io.Reader, orderedResources ...string) {
	t.Helper()

	gzr, err := gzip.NewReader(backupFile)
	require.NoError(t, err)

	r := tar.NewReader(gzr)

	// lastSeen tracks the index in 'orderedResources' of the last resource type
	// we saw in the tarball. Once we've seen a resource in 'orderedResources',
	// we should never see another instance of a prior resource.
	lastSeen := 0

	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		// ignore files like metadata/version
		if !strings.HasPrefix(hdr.Name, "resources/") {
			continue
		}

		// get the resource name
		parts := strings.Split(hdr.Name, "/")
		require.True(t, len(parts) >= 2)
		resourceName := parts[1]

		// Find the index in 'orderedResources' of the resource type for
		// the current tar item, if it exists. This index ('current') *must*
		// be greater than or equal to 'lastSeen', which was the last resource
		// we saw, since otherwise the current resource would be out of order. By
		// initializing current to len(ordered), we're saying that if the resource
		// is not explicitly in orederedResources, then it must come *after*
		// all orderedResources.
		current := len(orderedResources)
		for i, item := range orderedResources {
			if item == resourceName {
				current = i
				break
			}
		}

		// the index of the current resource must be the same as or greater than the index of
		// the last resource we saw for the backed-up order to be correct.
		assert.True(t, current >= lastSeen, "%s was backed up out of order", resourceName)
		lastSeen = current
	}
}
