package framework

import (
	"context"
	"time"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
)

type ELivenessProbeMapNodeProbeTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
	ns string
}

func NewELivenessProbeMapNodeProbeTester(c clientset.Interface, kc kruiseclientset.Interface, ns string) *ELivenessProbeMapNodeProbeTester {
	return &ELivenessProbeMapNodeProbeTester{
		c:  c,
		kc: kc,
		ns: ns,
	}
}

func (t *ELivenessProbeMapNodeProbeTester) CreateTestCloneSetAndGetPods(randStr string, replicas int32, annotations, labels map[string]string, containers []v1.Container) (pods []*v1.Pod) {
	set := &appsv1alpha1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: t.ns, Name: "clone-foo-" + randStr},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"rand": randStr}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"rand": randStr},
				},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAntiAffinity: &v1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
								{
									Weight:          100,
									PodAffinityTerm: v1.PodAffinityTerm{TopologyKey: v1.LabelHostname, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"rand": randStr}}},
								},
							},
						},
					},
					Containers: containers,
				},
			},
		},
	}
	if len(annotations) != 0 {
		set.Spec.Template.Annotations = annotations
	}
	if len(labels) != 0 {
		for lk, lv := range labels {
			set.Spec.Template.Labels[lk] = lv
		}
	}

	var err error
	if _, err = t.kc.AppsV1alpha1().CloneSets(t.ns).Create(context.TODO(), set, metav1.CreateOptions{}); err != nil {
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}

	// Wait for 60s
	gomega.Eventually(func() int32 {
		set, err = t.kc.AppsV1alpha1().CloneSets(t.ns).Get(context.TODO(), set.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		return set.Status.ReadyReplicas
	}, 120*time.Second, 3*time.Second).Should(gomega.Equal(replicas))

	podList, err := t.c.CoreV1().Pods(t.ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "rand=" + randStr})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	for i := range podList.Items {
		p := &podList.Items[i]
		pods = append(pods, p)
	}
	return
}

func (t *ELivenessProbeMapNodeProbeTester) CreateTestCloneSetAffinityNodeAndGetPods(randStr string, replicas int32,
	annotations, labels map[string]string, containers []v1.Container, affinityNodeName string) (pods []*v1.Pod) {
	set := &appsv1alpha1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: t.ns, Name: "clone-foo-" + randStr},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"rand": randStr}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"rand": randStr},
				},
				Spec: v1.PodSpec{
					NodeName:   affinityNodeName,
					Containers: containers,
				},
			},
		},
	}
	if len(annotations) != 0 {
		set.Spec.Template.Annotations = annotations
	}
	if len(labels) != 0 {
		for lk, lv := range labels {
			set.Spec.Template.Labels[lk] = lv
		}
	}

	var err error
	if _, err = t.kc.AppsV1alpha1().CloneSets(t.ns).Create(context.TODO(), set, metav1.CreateOptions{}); err != nil {
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}

	// Wait for 60s
	gomega.Eventually(func() int32 {
		set, err = t.kc.AppsV1alpha1().CloneSets(t.ns).Get(context.TODO(), set.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		return set.Status.ReadyReplicas
	}, 120*time.Second, 3*time.Second).Should(gomega.Equal(replicas))

	podList, err := t.c.CoreV1().Pods(t.ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "rand=" + randStr})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	for i := range podList.Items {
		p := &podList.Items[i]
		pods = append(pods, p)
	}
	return
}

func (t *ELivenessProbeMapNodeProbeTester) GetNodePodProbe(name string) (*appsv1alpha1.NodePodProbe, error) {
	return t.kc.AppsV1alpha1().NodePodProbes().Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *ELivenessProbeMapNodeProbeTester) CleanAllTestResources() error {
	if err := t.kc.AppsV1alpha1().NodePodProbes().DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{}); err != nil {
		return err
	}
	if err := t.kc.AppsV1alpha1().CloneSets(t.ns).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{}); err != nil {
		return err
	}
	return nil
}
