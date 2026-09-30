// Copyright 2026 The Kstack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package clustercard

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kstackhq/kstack/sidecar/internal/clustersvc"
)

var errRead = errors.New("boom")

// fakeService is the cluster service as the card reads it: one reading handed to
// read, canned answers for the cache reads, one knob for a read that fails, and
// every other method left to panic since nothing calls it.
type fakeService struct {
	clustersvc.Service
	// active is what ReadActive hands read; readErr, when set, is its answer instead.
	active  clustersvc.ActiveCluster
	readErr error
	health  clustersvc.ClusterCacheHealth
	sync    clustersvc.ClusterCacheSyncStatus
	kinds   []clustersvc.ClusterCachedDataKind
	names   []string
	// fail names the cache read that errors: health, sync, kinds or objects.
	fail string
	// gone names the cache read — health or sync — that finds no row, as for a
	// cache that does not exist; the list read then answers no file either.
	gone string
	// asked is every cache id the cache reads were given.
	asked []clustersvc.ClusterCacheID
}

// The three families the card reads, each over the one fake.
type (
	fakeClusters struct {
		clustersvc.Clusters
		*fakeService
	}
	fakeCaches struct {
		clustersvc.Caches
		*fakeService
	}
	fakeCachedData struct {
		clustersvc.CachedData
		*fakeService
	}
)

func (f *fakeService) Clusters() clustersvc.Clusters     { return fakeClusters{fakeService: f} }
func (f *fakeService) Caches() clustersvc.Caches         { return fakeCaches{fakeService: f} }
func (f *fakeService) CachedData() clustersvc.CachedData { return fakeCachedData{fakeService: f} }

func (f fakeClusters) ReadActive(ctx context.Context, _ clustersvc.ClusterID, read func(context.Context, clustersvc.ActiveCluster) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if f.readErr != nil {
		return f.readErr
	}
	return read(ctx, f.active)
}

func (f fakeCaches) Health(_ context.Context, _ clustersvc.ClusterID, cacheID clustersvc.ClusterCacheID) (clustersvc.ClusterCacheHealth, bool, error) {
	f.asked = append(f.asked, cacheID)
	if f.fail == "health" {
		return clustersvc.ClusterCacheHealth{}, false, errRead
	}
	return f.health, f.gone != "health", nil
}

func (f fakeCaches) SyncStatus(context.Context, clustersvc.ClusterID, clustersvc.ClusterCacheID) (clustersvc.ClusterCacheSyncStatus, bool, error) {
	if f.fail == "sync" {
		return clustersvc.ClusterCacheSyncStatus{}, false, errRead
	}
	return f.sync, f.gone != "sync", nil
}

func (f fakeCachedData) ListKinds(context.Context, clustersvc.ClusterID, clustersvc.ClusterCacheID) ([]clustersvc.ClusterCachedDataKind, error) {
	if f.fail == "kinds" {
		return nil, errRead
	}
	return f.kinds, nil
}

func (f fakeCachedData) ListObjects(context.Context, clustersvc.ClusterID, clustersvc.ClusterCacheID, string, string) ([]clustersvc.ClusterCachedDataObject, bool, error) {
	if f.fail == "objects" {
		return nil, false, errRead
	}
	var out []clustersvc.ClusterCachedDataObject
	for _, n := range f.names {
		out = append(out, clustersvc.ClusterCachedDataObject{Name: n})
	}
	return out, f.gone == "", nil
}

func strp(s string) *string { return &s }

// cluster is a reachable, identified record whose server UID is uid ("" for a
// cluster never identified).
func cluster(uid string) *clustersvc.Cluster {
	c := &clustersvc.Cluster{
		ID: "7", Conditions: []clustersvc.Condition{
			{Type: "Connected", Status: clustersvc.ConditionTrue, Reason: "Connected"},
			{Type: "Identified", Status: clustersvc.ConditionTrue, Reason: "Identified"},
		},
		Spec: clustersvc.ClusterSpec{
			Name:   strp("prod"),
			Source: clustersvc.ClusterSpecSource{Kubeconfig: &clustersvc.ClusterSpecSourceKubeconfig{Context: "prod-admin"}},
		},
		Status: clustersvc.ClusterStatus{
			Server: clustersvc.ClusterServer{Version: strp("v1.31.2")},
			Source: clustersvc.ClusterStatusSource{Kubeconfig: &clustersvc.ClusterStatusSourceKubeconfig{
				Cluster: clustersvc.ClusterStatusSourceKubeconfigCluster{Entry: &clustersvc.ClusterStatusSourceKubeconfigClusterEntry{Server: "https://10.0.0.1:6443"}},
			}},
		},
	}
	if uid != "" {
		c.Status.Server.UID = &uid
	}
	return c
}

// healthy is a reachable, identified cluster read with its active cache.
func healthy() *fakeService {
	return &fakeService{
		active: clustersvc.ActiveCluster{
			Cluster: cluster("uid-2"),
			Cache:   &clustersvc.ClusterCache{RecordMeta: clustersvc.RecordMeta{ID: 11}, Spec: clustersvc.ClusterCacheSpec{ServerUID: "uid-2"}},
		},
		health: clustersvc.ClusterCacheHealth{Reason: "Watching", TotalKinds: 2},
		sync: clustersvc.ClusterCacheSyncStatus{
			Discovery: clustersvc.ClusterCacheDiscoveryStatus{Reason: "Discovered"},
			Kinds: []clustersvc.ClusterCacheKindSyncStatus{
				{APIVersion: "v1", Resource: "namespaces", Reason: "Watching"},
				{APIVersion: "v1", Resource: "nodes", Reason: "Watching", ObjectCount: 3},
			},
		},
		kinds: []clustersvc.ClusterCachedDataKind{
			{APIVersion: "v1", Kind: "Namespace", Resource: "namespaces", Scope: "Cluster"},
			{APIVersion: "v1", Kind: "Node", Resource: "nodes", Scope: "Cluster", Count: 3},
			{APIVersion: "v1", Kind: "Pod", Resource: "pods", Scope: "Namespaced"},
		},
		names: []string{"default", "kube-system"},
	}
}

func cardOf(f *fakeService) string { return New(f).ClusterCard(context.Background(), "7") }

func TestCardReadsTheActiveCache(t *testing.T) {
	f := healthy()
	got := cardOf(f)

	assert.Equal(t, []clustersvc.ClusterCacheID{11}, f.asked, "the cache the reading handed")
	assert.Contains(t, got, `{"cluster":{"name":"prod","context":"prod-admin","kubernetes":"v1.31.2"},"connection":{"status":"Connected","tls":"verified"},"freshness":{"status":"watching"},"inventory":{"namespaces":{"status":"watching","names":["default","kube-system"]},"apiGroups":`)
	assert.Contains(t, got, `{"name":"core","kinds":["Namespace","Node","Pod"]}`)
}

// The posture is read the way client-go will dial: an explicit scheme is taken as
// given, and a schemeless server is https only when the kubeconfig names a CA or a
// client certificate or skips verifying — else plain http. A server with no TLS is
// "none", not one that skipped verifying it. The server URL stays behind — the
// endpoint the card withholds.
func TestCardReadsTheTLSPostureOffTheKubeconfigEntry(t *testing.T) {
	type entry = clustersvc.ClusterStatusSourceKubeconfigClusterEntry
	tests := map[string]struct {
		entry      *entry
		clientCert bool
		want       string
	}{
		"verified":                        {entry: &entry{Server: "https://10.0.0.1:6443"}, want: `"tls":"verified"}`},
		"skip verify":                     {entry: &entry{Server: "https://10.0.0.1:6443", InsecureSkipTLSVerify: true}, want: `"tls":"unverified"}`},
		"plain http":                      {entry: &entry{Server: "HTTP://10.0.0.1:8080", InsecureSkipTLSVerify: true}, want: `"tls":"none"}`},
		"schemeless with nothing":         {entry: &entry{Server: "10.0.0.1:8080"}, want: `"tls":"none"}`},
		"schemeless with a CA":            {entry: &entry{Server: "10.0.0.1:6443", HasCertificateAuthority: true}, want: `"tls":"verified"}`},
		"schemeless with a client cert":   {entry: &entry{Server: "10.0.0.1:6443"}, clientCert: true, want: `"tls":"verified"}`},
		"schemeless skipping verify":      {entry: &entry{Server: "10.0.0.1:6443", InsecureSkipTLSVerify: true}, want: `"tls":"unverified"}`},
		"a server client-go cannot parse": {entry: &entry{Server: "10.0.0.1:6443/with/a/path"}, want: `"connection":{"status":"Connected"}`},
		"no entry":                        {entry: nil, want: `"connection":{"status":"Connected"}`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := healthy()
			f.active.Cluster.Status.Source.Kubeconfig.Cluster.Entry = tc.entry
			f.active.Cluster.Status.Source.Kubeconfig.User.HasClientCertificate = tc.clientCert
			got := cardOf(f)
			assert.Contains(t, got, tc.want)
			assert.NotContains(t, got, "10.0.0.1")
		})
	}
	t.Run("a record of another source", func(t *testing.T) {
		f := healthy()
		f.active.Cluster.Status.Source.Kubeconfig = nil
		assert.Contains(t, cardOf(f), `"connection":{"status":"Connected"}`)
	})
}

// The card's cluster.context is ContextName, the one spelling a sandboxed run's
// kubeconfig names its context by: the record's kube-context cut as the card
// cuts it, and none for a record that names none, since no such context exists
// outside a sandboxed run.
func TestTheCardRendersContextName(t *testing.T) {
	long := strings.Repeat("c", 100)
	for name, context := range map[string]string{"short": "prod-admin", "100 bytes": long, "none": ""} {
		t.Run(name, func(t *testing.T) {
			f := healthy()
			f.active.Cluster.Spec.Source.Kubeconfig.Context = context
			want := ContextName(f.active.Cluster)

			var card struct {
				Cluster struct {
					Context string `json:"context"`
				} `json:"cluster"`
			}
			got := cardOf(f)
			body := strings.TrimSuffix(strings.TrimPrefix(got, cardHead), fenceClose)
			require.NoError(t, json.Unmarshal([]byte(body), &card))
			assert.Equal(t, want, card.Cluster.Context)
		})
	}
	assert.Equal(t, "prod-admin", ContextName(cluster("")))
	c := cluster("")
	c.Spec.Source.Kubeconfig.Context = long
	assert.Equal(t, cut(long, contextMax), ContextName(c))
	c.Spec.Source.Kubeconfig = nil
	assert.Empty(t, ContextName(c))
}

// The longest context a cloud names, an EKS ARN of a cluster name at its limit,
// comes through whole: a command outside the sandbox names it in the user's
// own kubeconfig, where a cut one names nothing.
func TestAnEKSContextIsNotCut(t *testing.T) {
	arn := "arn:aws:eks:ap-southeast-2:123456789012:cluster/" + strings.Repeat("c", 100)
	c := cluster("")
	c.Spec.Source.Kubeconfig.Context = arn

	assert.Equal(t, arn, ContextName(c))
}

func TestCardOfAClusterTheUserNeverNamed(t *testing.T) {
	f := healthy()
	f.active.Cluster.Spec.Name = nil
	assert.Contains(t, cardOf(f), `{"cluster":{"context":"prod-admin","kubernetes"`)
}

func TestCardSaysWhenTheIdentityCannotBeRead(t *testing.T) {
	f := healthy()
	f.active.Cluster.Conditions[1] = clustersvc.Condition{Type: "Identified", Status: clustersvc.ConditionFalse, Reason: "UIDUnreadable"}
	assert.Contains(t, cardOf(f), `"connection":{"status":"UIDUnreadable","tls":"verified"}`)
}

func TestCardOfANeverIdentifiedCluster(t *testing.T) {
	f := healthy()
	f.active = clustersvc.ActiveCluster{Cluster: cluster("")}
	f.active.Cluster.Conditions = []clustersvc.Condition{{Type: "Connected", Status: clustersvc.ConditionFalse, Reason: "Connecting"}}
	assert.Equal(t, cardHead+`{"cluster":{"name":"prod","context":"prod-admin","kubernetes":"v1.31.2"},"connection":{"status":"Connecting","tls":"verified"},"freshness":{"status":"syncing"}}`+fenceClose, cardOf(f))
	assert.Empty(t, f.asked)
}

func TestCardOfAClusterWithNoVerdictYet(t *testing.T) {
	f := healthy()
	f.active = clustersvc.ActiveCluster{Cluster: cluster("")}
	f.active.Cluster.Conditions = nil
	assert.Contains(t, cardOf(f), `"connection":{"status":"Connecting","tls":"verified"}`)
}

func TestCardOfAnIdentifiedClusterWhoseCacheIsNotYetCreated(t *testing.T) {
	f := healthy()
	f.active.Cache = nil
	assert.Contains(t, cardOf(f), `"freshness":{"status":"syncing"}}`)
}

// Resolving the cluster and guarding its identity are ReadActive's; whatever it
// answers instead of a reading, the card is unavailable.
func TestCardIsUnavailableWhenTheReadingFails(t *testing.T) {
	for _, err := range []error{clustersvc.ErrNotFound, clustersvc.ErrIdentityMoved, errRead} {
		f := healthy()
		f.readErr = err
		assert.Equal(t, Unavailable, cardOf(f), err.Error())
	}
}

func TestCardIsUnavailableWhenAnyReadFails(t *testing.T) {
	for _, read := range []string{"health", "sync", "kinds", "objects"} {
		t.Run(read, func(t *testing.T) {
			f := healthy()
			f.fail = read
			assert.Equal(t, Unavailable, cardOf(f))
		})
	}
	for _, read := range []string{"health", "sync"} {
		t.Run("the "+read+" read finds no cache", func(t *testing.T) {
			f := healthy()
			f.gone = read
			assert.Equal(t, Unavailable, cardOf(f))
		})
	}
}

func TestCardIsUnavailableWhenItsBoundExpires(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Equal(t, Unavailable, New(healthy()).ClusterCard(ctx, "7"))
}

func TestCardFoldsTheSyncRowsOntoTheCatalog(t *testing.T) {
	f := healthy()
	at := time.Date(2026, 9, 14, 7, 30, 15, 0, time.UTC)
	f.health = clustersvc.ClusterCacheHealth{Reason: "SyncFailed", LastLiveAt: &at, TotalKinds: 2, PausedKinds: 1,
		UnhealthyKindRefs: []clustersvc.SyncedKindRef{{APIVersion: "v1", Resource: "nodes"}}}
	f.sync.Kinds[0].Reason = "Paused"
	f.sync.Kinds[1].Reason = "SyncFailed"
	got := cardOf(f)
	require.Contains(t, got, `"freshness":{"status":"last-known","reason":"SyncFailed","since":"2026-09-14T07:30:15Z"}`, "one of two kinds paused, the other behind")
	assert.Contains(t, got, `"namespaces":{"status":"paused","names":["default","kube-system"]}`)
}

// A cache whose cluster has sync switched off is reported Paused; its rows are
// still there, and the card lists them as paused.
func TestCardOfAPausedCacheListsWhatItHolds(t *testing.T) {
	f := healthy()
	f.health = clustersvc.ClusterCacheHealth{Reason: "Paused"}
	f.sync.Kinds[0].Reason = "Paused"
	f.sync.Kinds[1].Reason = ""
	got := cardOf(f)
	assert.Contains(t, got, `"freshness":{"status":"paused"}`)
	assert.Contains(t, got, `"namespaces":{"status":"paused","names":["default","kube-system"]}`)
}

// Read gathers the facts of the reading it is handed and resolves nothing: the
// card is Render of them.
func TestReadGathersTheFactsOfTheReadingItIsHanded(t *testing.T) {
	f := healthy()
	facts, err := Read(context.Background(), f, f.active)

	require.NoError(t, err)
	assert.Equal(t, cardOf(healthy()), Render(facts))
	assert.Equal(t, "Namespaced", facts.Cache.FindKind("v1", "pods").Scope)
}

func TestReadOfAClusterWithNoCacheReadsNoCache(t *testing.T) {
	f := healthy()
	f.active.Cache = nil

	facts, err := Read(context.Background(), f, f.active)

	require.NoError(t, err)
	assert.Nil(t, facts.Cache)
	assert.Empty(t, f.asked)
}
