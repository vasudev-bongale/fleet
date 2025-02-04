/*
Copyright (c) Microsoft Corporation.
Licensed under the MIT license.
*/

package clusteraffinity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	clusterv1beta1 "go.goms.io/fleet/apis/cluster/v1beta1"
	placementv1beta1 "go.goms.io/fleet/apis/placement/v1beta1"
	"go.goms.io/fleet/pkg/scheduler/framework"
)

var (
	cmpStatusOptions = cmp.Options{
		cmpopts.IgnoreFields(framework.Status{}, "reasons", "err"),
		cmp.AllowUnexported(framework.Status{}),
	}
	cmpPluginStateOptions = cmp.Options{
		cmp.AllowUnexported(pluginState{}, affinityTerm{}, preferredAffinityTerm{}),
	}
	defaultPluginName = defaultPluginOptions.name
)

func TestPreFilter(t *testing.T) {
	tests := []struct {
		name            string
		policy          *placementv1beta1.PlacementPolicy
		want            *framework.Status
		wantPluginState *pluginState
	}{
		{
			name: "nil policy",
			want: framework.NewNonErrorStatus(framework.Skip, defaultPluginName),
		},
		{
			name:   "nil affinity",
			policy: &placementv1beta1.PlacementPolicy{},
			want:   framework.NewNonErrorStatus(framework.Skip, defaultPluginName),
		},
		{
			name: "nil cluster affinity",
			policy: &placementv1beta1.PlacementPolicy{
				Affinity: &placementv1beta1.Affinity{},
			},
			want: framework.NewNonErrorStatus(framework.Skip, defaultPluginName),
		},
		{
			name: "no cluster affinity",
			policy: &placementv1beta1.PlacementPolicy{
				Affinity: &placementv1beta1.Affinity{
					ClusterAffinity: &placementv1beta1.ClusterAffinity{},
				},
			},
			want: framework.NewNonErrorStatus(framework.Skip, defaultPluginName),
			wantPluginState: &pluginState{
				requiredAffinityTerms:  []affinityTerm{},
				preferredAffinityTerms: []preferredAffinityTerm{},
			},
		},
		{
			name: "no required terms and empty preferred terms",
			policy: &placementv1beta1.PlacementPolicy{
				Affinity: &placementv1beta1.Affinity{
					ClusterAffinity: &placementv1beta1.ClusterAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &placementv1beta1.ClusterSelector{},
						PreferredDuringSchedulingIgnoredDuringExecution: []placementv1beta1.PreferredClusterSelector{
							{
								Weight: 0,
								Preference: placementv1beta1.ClusterSelectorTerm{
									LabelSelector: metav1.LabelSelector{
										MatchLabels: map[string]string{
											"region": "us-west",
										},
									},
								},
							},
						},
					},
				},
			},
			want: framework.NewNonErrorStatus(framework.Skip, defaultPluginName),
			wantPluginState: &pluginState{
				requiredAffinityTerms:  []affinityTerm{},
				preferredAffinityTerms: []preferredAffinityTerm{},
			},
		},
		{
			name: "no required terms and multiple preferred terms",
			policy: &placementv1beta1.PlacementPolicy{
				Affinity: &placementv1beta1.Affinity{
					ClusterAffinity: &placementv1beta1.ClusterAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &placementv1beta1.ClusterSelector{},
						PreferredDuringSchedulingIgnoredDuringExecution: []placementv1beta1.PreferredClusterSelector{
							{
								Weight: 5,
								Preference: placementv1beta1.ClusterSelectorTerm{
									LabelSelector: metav1.LabelSelector{
										MatchLabels: map[string]string{
											"region": "us-west",
										},
									},
								},
							},
							{
								Weight: 1,
								Preference: placementv1beta1.ClusterSelectorTerm{
									LabelSelector: metav1.LabelSelector{
										MatchLabels: map[string]string{},
									},
								},
							},
						},
					},
				},
			},
			want: framework.NewNonErrorStatus(framework.Skip, defaultPluginName),
			wantPluginState: &pluginState{
				requiredAffinityTerms: []affinityTerm{},
				preferredAffinityTerms: []preferredAffinityTerm{
					{
						weight: 5,
						affinityTerm: affinityTerm{
							selector: labels.SelectorFromSet(map[string]string{"region": "us-west"}),
						},
					},
				},
			},
		},
		{
			name: "empty required terms and no preferred terms",
			policy: &placementv1beta1.PlacementPolicy{
				Affinity: &placementv1beta1.Affinity{
					ClusterAffinity: &placementv1beta1.ClusterAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &placementv1beta1.ClusterSelector{
							ClusterSelectorTerms: []placementv1beta1.ClusterSelectorTerm{
								{
									LabelSelector: metav1.LabelSelector{
										MatchLabels: map[string]string{},
									},
								},
							},
						},
					},
				},
			},
			want: framework.NewNonErrorStatus(framework.Skip, defaultPluginName),
			wantPluginState: &pluginState{
				requiredAffinityTerms:  []affinityTerm{},
				preferredAffinityTerms: []preferredAffinityTerm{},
			},
		},
		{
			name: "multiple required terms and no preferred terms",
			policy: &placementv1beta1.PlacementPolicy{
				Affinity: &placementv1beta1.Affinity{
					ClusterAffinity: &placementv1beta1.ClusterAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &placementv1beta1.ClusterSelector{
							ClusterSelectorTerms: []placementv1beta1.ClusterSelectorTerm{
								{
									LabelSelector: metav1.LabelSelector{
										MatchLabels: map[string]string{"region": "us-west"},
									},
								},
								{
									LabelSelector: metav1.LabelSelector{},
								},
							},
						},
					},
				},
			},
			want: nil, // not skip the filter stage
			wantPluginState: &pluginState{
				requiredAffinityTerms: []affinityTerm{
					{
						selector: labels.SelectorFromSet(map[string]string{"region": "us-west"}),
					},
				},
				preferredAffinityTerms: []preferredAffinityTerm{},
			},
		},
		{
			name: "multiple required terms and preferred terms",
			policy: &placementv1beta1.PlacementPolicy{
				Affinity: &placementv1beta1.Affinity{
					ClusterAffinity: &placementv1beta1.ClusterAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &placementv1beta1.ClusterSelector{
							ClusterSelectorTerms: []placementv1beta1.ClusterSelectorTerm{
								{
									LabelSelector: metav1.LabelSelector{
										MatchLabels: map[string]string{"region": "us-west"},
									},
								},
							},
						},
						PreferredDuringSchedulingIgnoredDuringExecution: []placementv1beta1.PreferredClusterSelector{
							{
								Weight: 5,
								Preference: placementv1beta1.ClusterSelectorTerm{
									LabelSelector: metav1.LabelSelector{
										MatchLabels: map[string]string{
											"region": "us-west",
										},
									},
								},
							},
							{
								Weight: 1,
								Preference: placementv1beta1.ClusterSelectorTerm{
									LabelSelector: metav1.LabelSelector{
										MatchLabels: map[string]string{},
									},
								},
							},
						},
					},
				},
			},
			want: nil, // not skip the filter stage
			wantPluginState: &pluginState{
				requiredAffinityTerms: []affinityTerm{
					{
						selector: labels.SelectorFromSet(map[string]string{"region": "us-west"}),
					},
				},
				preferredAffinityTerms: []preferredAffinityTerm{
					{
						weight: 5,
						affinityTerm: affinityTerm{
							selector: labels.SelectorFromSet(map[string]string{"region": "us-west"}),
						},
					},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := framework.NewCycleState(nil, nil)
			snapshot := &placementv1beta1.ClusterSchedulingPolicySnapshot{
				Spec: placementv1beta1.SchedulingPolicySnapshotSpec{
					Policy: tc.policy,
				},
			}
			p := New()
			got := p.PreFilter(context.Background(), state, snapshot)
			if diff := cmp.Diff(tc.want, got, cmpStatusOptions); diff != "" {
				t.Errorf("PreFilter() status mismatch (-want, +got):\n%s", diff)
			}
			if tc.wantPluginState == nil {
				return
			}
			gotPluginState, err := p.readPluginState(state)
			if err != nil {
				t.Fatalf("readPluginState() got err %v, want not nil", err)
			}
			if diff := cmp.Diff(tc.wantPluginState, gotPluginState, cmpPluginStateOptions); diff != "" {
				t.Errorf("readPluginState() pluginState mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name              string
		ps                *pluginState
		notSetPluginState bool
		cluster           *clusterv1beta1.MemberCluster
		want              *framework.Status
	}{
		{
			name:              "pluginState is not set",
			notSetPluginState: true,
			want:              framework.FromError(errors.New("invalid state"), defaultPluginName),
		},
		{
			name: "nil pluginState",
			want: framework.FromError(errors.New("invalid state"), defaultPluginName),
		},
		{
			name: "matched cluster",
			ps: &pluginState{
				requiredAffinityTerms: []affinityTerm{
					{
						selector: labels.SelectorFromSet(map[string]string{"region": "us-west"}),
					},
				},
			},
			cluster: &clusterv1beta1.MemberCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
					Labels: map[string]string{
						"region": "us-west",
						"zone":   "zone2",
					},
				},
			},
			want: nil,
		},
		{
			name: "not matched cluster",
			ps: &pluginState{
				requiredAffinityTerms: []affinityTerm{
					{
						selector: labels.SelectorFromSet(map[string]string{"region": "us-west"}),
					},
				},
			},
			cluster: &clusterv1beta1.MemberCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
					Labels: map[string]string{
						"region": "us-east",
						"zone":   "zone2",
					},
				},
			},
			want: framework.NewNonErrorStatus(framework.ClusterUnschedulable, defaultPluginName),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := New()
			state := framework.NewCycleState(nil, nil)
			if !tc.notSetPluginState {
				state.Write(framework.StateKey(p.Name()), tc.ps)
			}

			got := p.Filter(context.Background(), state, nil, tc.cluster)
			if diff := cmp.Diff(tc.want, got, cmpStatusOptions); diff != "" {
				t.Errorf("Filter() status mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
