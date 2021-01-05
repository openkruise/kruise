/*
Copyright 2019 The Kruise Authors.

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

package updatesort

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateRules(t *testing.T) {
	testCases := []struct {
		desc            string
		podLabels       []map[string]string
		scatterStrategy appsv1alpha1.UpdateScatterStrategy
		expectedResult  []appsv1alpha1.UpdateScatterTerm
	}{
		{
			desc:            "one pod one label",
			podLabels:       []map[string]string{{}, {}, {}, {}, {"labelA": "AAA"}, {"labelA": "AAA"}},
			scatterStrategy: []appsv1alpha1.UpdateScatterTerm{{Key: "labelA", Value: "AAA"}},
			expectedResult:  []appsv1alpha1.UpdateScatterTerm{{Key: "labelA", Value: "AAA"}},
		},
		{
			desc:            "same pods a label",
			podLabels:       []map[string]string{{}, {}, {"labelB": "B"}, {"labelB": "BBB"}, {"labelA": "AAA"}, {"labelA": "AAA"}},
			scatterStrategy: []appsv1alpha1.UpdateScatterTerm{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedResult:  []appsv1alpha1.UpdateScatterTerm{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
		},
		//{
		//	desc:            "test regular label",
		//	podLabels:       []map[string]string{{"mode": "AAA"}, {"mode": "AAA"}, {"mode": "AAA"}, {"mode": "BBB"}, {"mode": "BBB"}, {}, {}, {"mode": "CCC"}, {"mode": "CCC"}, {"mode": "CCC"}, {"mode": "CCC"}},
		//	scatterStrategy: []appsv1alpha1.UpdateScatterTerm{{Key: "mode", Value: "*"}},
		//	expectedResult:  []appsv1alpha1.UpdateScatterTerm{{Key: "mode", Value: "CCC"}, {Key: "mode", Value: "AAA"}, {Key: "mode", Value: "BBB"}},
		//},
		//{
		//	desc:            "test regular label + other label",
		//	podLabels:       []map[string]string{{"mode": "AAA"}, {"mode": "AAA", "labelB": "BBB"}, {"mode": "AAA"}, {"mode": "BBB"}, {"mode": "BBB"}, {}, {}, {"mode": "CCC"}, {"mode": "CCC"}, {"mode": "CCC", "labelB": "BBB"}, {"mode": "CCC"}},
		//	scatterStrategy: []appsv1alpha1.UpdateScatterTerm{{Key: "mode", Value: "*"}, {Key: "labelB", Value: "BBB"}},
		//	expectedResult:  []appsv1alpha1.UpdateScatterTerm{{Key: "mode", Value: "CCC"}, {Key: "mode", Value: "AAA"}, {Key: "mode", Value: "BBB"}, {Key: "labelB", Value: "BBB"}},
		//},
		//{
		//	desc:            "test more regular labels",
		//	podLabels:       []map[string]string{{"mode": "AAA"}, {"mode": "AAA", "labelB": "BBB"}, {"mode": "AAA"}, {"mode": "BBB"}, {"mode": "BBB"}, {"env": "AAA"}, {}, {"env": "AAA", "mode": "CCC"}, {"mode": "CCC"}, {"mode": "CCC", "labelB": "BBB", "env": "AAA"}, {"mode": "CCC"}, {"env": "CCC"}, {"env": "CCC"}},
		//	scatterStrategy: []appsv1alpha1.UpdateScatterTerm{{Key: "mode", Value: "*"}, {Key: "env", Value: "*"}},
		//	expectedResult:  []appsv1alpha1.UpdateScatterTerm{{Key: "mode", Value: "CCC"}, {Key: "mode", Value: "AAA"}, {Key: "env", Value: "AAA"}, {Key: "mode", Value: "BBB"}, {Key: "env", Value: "CCC"}},
		//},
	}

	for idx, tc := range testCases {
		var pods []*v1.Pod
		var indexes []int
		for i, labels := range tc.podLabels {
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Labels:    labels,
					Name:      fmt.Sprintf("%d", i),
				},
			}

			pods = append(pods, pod)
			indexes = append(indexes, i)
		}

		ss := &scatterSort{strategy: tc.scatterStrategy}
		terms := ss.getScatterTerms(pods, indexes)

		if !reflect.DeepEqual(terms, tc.expectedResult) {
			t.Errorf("[%d] wrong order. desired: \n%v\n, got: \n%v", idx, tc.expectedResult, terms)
		}
	}
}

func TestScatterPodsByRule(t *testing.T) {
	strategyTerm := appsv1alpha1.UpdateScatterTerm{Key: "labelA", Value: "AAA"}
	testCases := []struct {
		desc            string
		podLabels       []string
		expectedIndexes []int
	}{
		{
			desc:            "no label",
			podLabels:       []string{""},
			expectedIndexes: []int{0},
		},
		{
			desc:            "only one scattered pod",
			podLabels:       []string{"labelA=AAA"},
			expectedIndexes: []int{0},
		},
		{
			desc:            "odd scattered pod + odd ordinary pod",
			podLabels:       []string{"labelA=AAA", "bbb"},
			expectedIndexes: []int{0, 1},
		},
		{
			desc:            "even scattered pods + even ordinary pods",
			podLabels:       []string{"", "", "", "", "labelA=AAA", "labelA=AAA"},
			expectedIndexes: []int{4, 0, 1, 2, 3, 5},
		},
		{
			desc:            "odd scattered pods + odd ordinary pods",
			podLabels:       []string{"", "", "", "labelA=AAA", "labelA=AAA", "labelA=AAA"},
			expectedIndexes: []int{3, 0, 1, 4, 2, 5},
		},
		{
			desc:            "even scattered pods + odd ordinary pods",
			podLabels:       []string{"", "", "", "labelA=AAA", "", "", "labelA=AAA"},
			expectedIndexes: []int{3, 0, 1, 2, 4, 5, 6},
		},
		{
			desc:            "odd scattered pods + even ordinary pods",
			podLabels:       []string{"", "", "", "", "", "", "labelA=AAA", "labelA=AAA", "labelA=AAA"},
			expectedIndexes: []int{6, 0, 1, 2, 7, 3, 4, 5, 8},
		},
		{
			desc:            "even scattered pods + even ordinary pods + scattered pods more than ordinary pods",
			podLabels:       []string{"", "", "labelA=AAA", "labelA=AAA", "labelA=AAA", "labelA=AAA"},
			expectedIndexes: []int{2, 3, 0, 4, 1, 5},
		},
		{
			desc:            "odd scattered pods + odd ordinary pods + scattered pods more than ordinary pods",
			podLabels:       []string{"", "", "", "labelA=AAA", "labelA=AAA", "labelA=AAA", "labelA=AAA", "labelA=AAA"},
			expectedIndexes: []int{3, 0, 1, 4, 2, 5},
		},
		{
			desc:            "even scattered pods + odd ordinary pods + scattered pods more than ordinary pods",
			podLabels:       []string{"", "", "labelA=AAA", "", "labelA=AAA", "labelA=AAA", "labelA=AAA", "labelA=AAA", "labelA=AAA"},
			expectedIndexes: []int{2, 4, 0, 5, 6, 1, 7, 3, 8},
		},
		{
			desc:            "odd scattered pods + even ordinary pods + scattered pods more than ordinary pods",
			podLabels:       []string{"", "labelA=AAA", "", "labelA=AAA", "", "", "labelA=AAA", "labelA=AAA", "labelA=AAA"},
			expectedIndexes: []int{1, 0, 3, 2, 6, 4, 7, 5, 8},
		},
		{
			desc:            "invalid label",
			podLabels:       []string{"", "", "", "", "ee", "a", "", "labelA=AAA", "labelA=AAA", "labelA=AAA,labelB=BBB"},
			expectedIndexes: []int{7, 0, 1, 2, 3, 8, 4, 5, 6, 9},
		},
	}

	for idx, tc := range testCases {

		var pods []*v1.Pod
		var indexes []int
		for i := 0; i < len(tc.expectedIndexes); i++ {
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Labels:    map[string]string{},
					Name:      fmt.Sprintf("%d", i),
				},
			}
			for _, scatterLabel := range strings.Split(tc.podLabels[i], ",") {
				label := strings.SplitN(scatterLabel, "=", 2)
				if len(label) < 2 {
					continue
				}
				pod.Labels[label[0]] = label[1]
			}

			pods = append(pods, pod)
			indexes = append(indexes, i)
		}

		ss := &scatterSort{strategy: []appsv1alpha1.UpdateScatterTerm{strategyTerm}}
		gotIndexes := ss.scatterPodsByRule(strategyTerm, pods, indexes)

		// compare
		if !reflect.DeepEqual(gotIndexes, tc.expectedIndexes) {
			t.Errorf("[%d] wrong order. desired: %v, got: %v", idx, tc.expectedIndexes, gotIndexes)
		}
	}
}

func TestSort(t *testing.T) {
	testCases := []struct {
		desc            string
		podLabels       []map[string]string
		scatterStrategy appsv1alpha1.UpdateScatterStrategy
		expectedIndexes []int
	}{
		{
			desc:            "a scattered pod + a ordinary pod",
			podLabels:       []map[string]string{{"labelA": "AAA", "labelB": "BBB"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}},
			expectedIndexes: []int{0},
		},
		{
			desc:            "all ordinary pods",
			podLabels:       []map[string]string{{}, {}, {}, {}, {}, {}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{},
			expectedIndexes: []int{0, 1, 2, 3, 4, 5},
		},
		{
			desc:            "one pod one label",
			podLabels:       []map[string]string{{}, {}, {}, {}, {"labelA": "AAA"}, {"labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}},
			expectedIndexes: []int{4, 0, 1, 2, 3, 5},
		},
		{
			desc:            "one pod more labels",
			podLabels:       []map[string]string{{}, {}, {}, {}, {"sdfb": "eee"}, {"dsf": "same"}, {"labelB": "BBB"}, {"labelA": "AAA"}, {"labelA": "AAA"}, {"labelA": "AAA", "labelB": "BBB"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{6, 0, 1, 2, 3, 4, 5, 7, 8, 9},
		},
		{
			desc:            "2 dimensions + one pod one label",
			podLabels:       []map[string]string{{"labelB": "BBB"}, {"labelB": "BBB"}, {}, {}, {"labelA": "AAA"}, {"labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{0, 4, 2, 3, 5, 1},
		},
		{
			desc:            "2 dimensions + one pod more labels",
			podLabels:       []map[string]string{{}, {}, {}, {"labelB": "BBB"}, {"labelA": "AAA", "labelB": "BBB"}, {"labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{4, 0, 1, 2, 5, 3},
		},
		{
			desc:            "2 dimensions + same label + sequence: A + B",
			podLabels:       []map[string]string{{}, {}, {}, {}, {"sdfb": "eee"}, {"dsf": "same"}, {"labelB": "BBB"}, {"labelA": "AAA"}, {"labelA": "AAA"}, {"labelB": "BBB"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelB", Value: "BBB"}, {Key: "labelA", Value: "AAA"}},
			expectedIndexes: []int{7, 6, 0, 1, 2, 3, 4, 5, 9, 8},
		},
		{
			desc:            "2 dimensions + same label + sequence: B + A",
			podLabels:       []map[string]string{{}, {}, {}, {}, {"sdfb": "eee"}, {"dsf": "same"}, {"labelB": "BBB"}, {"labelA": "AAA"}, {"labelA": "AAA"}, {"labelB": "BBB"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{6, 7, 0, 1, 2, 3, 4, 5, 8, 9},
		},
		{
			desc:            "2 dimensions + same pod same label + even scatter pods + even ordinary pods",
			podLabels:       []map[string]string{{}, {}, {}, {}, {"sdfb": "eee"}, {"dsf": "same"}, {"labelA": "AAA", "labelB": "BBB"}, {}, {}, {"labelB": "BBB", "labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{6, 0, 1, 2, 3, 4, 5, 7, 8, 9},
		},
		{
			desc:            "2 dimensions + same pod same label + even scatter pods + odd ordinary pods",
			podLabels:       []map[string]string{{}, {}, {}, {"sdfb": "eee"}, {"dsf": "same"}, {"labelA": "AAA", "labelB": "BBB"}, {}, {}, {"labelB": "BBB", "labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{5, 0, 1, 2, 3, 4, 6, 7, 8},
		},
		{
			desc:            "2 dimensions + same pod same label + odd scatter pods + odd ordinary pods",
			podLabels:       []map[string]string{{}, {}, {}, {"labelA": "AAA"}, {"sdfb": "eee"}, {"dsf": "same"}, {"labelA": "AAA", "labelB": "BBB"}, {}, {}, {}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{3, 0, 1, 2, 4, 5, 7, 6},
		},
		{
			desc:            "2 dimensions + same pod same label + odd scatter pods + odd ordinary pods",
			podLabels:       []map[string]string{{}, {}, {}, {"labelA": "AAA"}, {}, {}, {"sdfb": "eee"}, {"dsf": "same"}, {"labelA": "AAA", "labelB": "BBB"}, {}, {}, {"labelB": "BBB", "labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{0, 1, 2, 4, 3, 5, 6, 7, 9, 10, 8},
		},
		{
			desc:            "2 dimensions + same pod same label + even scatter pods + even ordinary pods + scatter pods more than ordinary pods",
			podLabels:       []map[string]string{{}, {}, {}, {"labelA": "AAA"}, {"sdfb": "eee", "labelA": "AAA"}, {"dsf": "same"}, {"labelA": "AAA", "labelB": "BBB"}, {}, {}, {"labelB": "BBB", "labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{6, 3, 0, 1, 4, 2, 5, 7, 8, 9},
		},
		{
			desc:            "2 dimensions + same pod same label + even scatter pods + odd ordinary pods + scatter pods more than ordinary pods",
			podLabels:       []map[string]string{{}, {"labelA": "AAA"}, {"labelA": "AAA"}, {"sdfb": "eee"}, {"dsf": "same"}, {"labelA": "AAA", "labelB": "BBB"}, {}, {}, {"labelB": "BBB", "labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{5, 1, 0, 3, 2, 4, 6, 7, 8},
		},
		{
			desc:            "2 dimensions + same pod same label + odd scatter pods + odd ordinary pods + scatter pods more than ordinary pods",
			podLabels:       []map[string]string{{"labelA": "AAA"}, {}, {}, {"labelA": "AAA"}, {"sdfb": "eee", "labelA": "AAA"}, {"dsf": "same"}, {"labelA": "AAA", "labelB": "BBB"}, {}, {}, {"labelB": "BBB", "labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{1, 0, 2, 3, 5, 4, 7, 6},
		},
		{
			desc:            "2 dimensions + same pod same label + odd scatter pods + odd ordinary pods + scatter pods more than ordinary pods",
			podLabels:       []map[string]string{{}, {}, {"labelA": "AAA"}, {"labelA": "AAA"}, {}, {"labelA": "AAA"}, {"sdfb": "eee"}, {"dsf": "same"}, {"labelA": "AAA", "labelB": "BBB"}, {}, {}, {"labelB": "BBB", "labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}},
			expectedIndexes: []int{0, 2, 1, 4, 3, 6, 7, 5, 9, 10, 8},
		},
		{
			desc:            "3 dimensions + one pod one label",
			podLabels:       []map[string]string{{}, {"labelA": "AAA"}, {"labelA": "AAA"}, {"labelB": "BBB"}, {"labelB": "BBB"}, {"labelC": "CCC"}, {"labelC": "CCC"}, {"labelC": "CCC"}, {"labelC": "CCC"}, {"labelC": "CCC"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}, {Key: "labelC", Value: "CCC"}},
			expectedIndexes: []int{3, 1, 5, 0, 6, 7, 8, 9, 2, 4},
		},
		{
			desc:            "3 dimensions + one pod more label",
			podLabels:       []map[string]string{{}, {"labelA": "AAA"}, {"labelA": "AAA", "labelC": "CCC"}, {"labelB": "BBB", "labelC": "CCC"}, {"labelB": "BBB"}, {}, {}, {"labelC": "CCC"}, {"labelC": "CCC"}, {"labelC": "CCC"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}, {Key: "labelC", Value: "CCC"}},
			expectedIndexes: []int{3, 2, 0, 7, 8, 5, 6, 9, 1, 4},
		},
		//{
		//	desc:            "test regular label",
		//	podLabels:       []map[string]string{{"mode": "AAA"}, {"mode": "AAA"}, {"mode": "AAA"}, {"mode": "BBB"}, {"mode": "BBB"}, {}, {}, {"mode": "CCC"}, {"mode": "CCC"}, {"mode": "CCC"}, {"mode": "CCC"}},
		//	scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "mode", Value: "*"}},
		//	expectedIndexes: []int{3, 0, 7, 8, 9, 1, 5, 6, 10, 2, 4},
		//},
		//{
		//	desc:            "test regular label + other label",
		//	podLabels:       []map[string]string{{"mode": "AAA"}, {"mode": "AAA", "labelB": "BBB"}, {"mode": "AAA"}, {"mode": "BBB"}, {"mode": "BBB"}, {}, {}, {"mode": "CCC"}, {"mode": "CCC"}, {"mode": "CCC", "labelB": "BBB"}, {"mode": "CCC"}},
		//	scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "mode", Value: "*"}, {Key: "labelB", Value: "BBB"}},
		//	expectedIndexes: []int{9, 3, 0, 7, 8, 5, 6, 10, 2, 4, 1},
		//},
		{
			desc:            "continuous sort 1",
			podLabels:       []map[string]string{{}, {}, {}, {}, {"labelA": "AAA"}, {"labelA": "AAA"}, {"labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}},
			expectedIndexes: []int{0, 1, 4, 2, 3, 5},
		},
		{
			desc:            "continuous sort 2",
			podLabels:       []map[string]string{{"labelA": "AAA"}, {"labelA": "AAA"}, {}, {}, {}, {}, {}, {}, {}, {"labelA": "AAA"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}},
			expectedIndexes: []int{2, 0, 3, 4, 5, 6, 1},
		},
		{
			desc:            "reserveOrdinals nil in slice",
			podLabels:       []map[string]string{{}, {"labelA": "AAA"}, {"labelA": "AAA", "labelC": "CCC"}, {"labelB": "BBB", "labelC": "CCC"}, {"labelB": "BBB"}, nil, {}, {}, {"labelC": "CCC"}, {"labelC": "CCC"}, {"labelC": "CCC"}},
			scatterStrategy: appsv1alpha1.UpdateScatterStrategy{{Key: "labelA", Value: "AAA"}, {Key: "labelB", Value: "BBB"}, {Key: "labelC", Value: "CCC"}},
			expectedIndexes: []int{3, 2, 0, 8, 9, 6, 7, 10, 1, 4},
		},
	}

	for idx, tc := range testCases {
		var pods []*v1.Pod
		var indexes []int
		var j int
		for i := 0; i < len(tc.expectedIndexes)+j; i++ {
			var pod *v1.Pod
			if tc.podLabels[i] != nil {
				pod = &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Labels:    tc.podLabels[i],
						Name:      fmt.Sprintf("%d", i),
					},
				}
				indexes = append(indexes, i)
			} else {
				j++
			}
			pods = append(pods, pod)
		}
		for i := len(indexes) + j; i < len(tc.podLabels); i++ {
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Labels:    tc.podLabels[i],
					Name:      fmt.Sprintf("%d", i),
				},
			}
			pods = append(pods, pod)
		}

		ss := &scatterSort{strategy: tc.scatterStrategy}
		gotIndexes := ss.Sort(pods, indexes)

		// compare
		if !reflect.DeepEqual(gotIndexes, tc.expectedIndexes) {
			t.Errorf("[%d][%s] wrong order. desired: %v, got: %v", idx, tc.desc, tc.expectedIndexes, gotIndexes)
		}
	}
}
