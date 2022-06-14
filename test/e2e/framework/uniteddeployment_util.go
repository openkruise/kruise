package framework

import (
	"context"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"time"
)

type UnitedDeploymentTester struct {
	C  clientset.Interface
	kc kruiseclientset.Interface
}

func NewUnitedDeploymentTester(c clientset.Interface, kc kruiseclientset.Interface) *UnitedDeploymentTester {
	return &UnitedDeploymentTester{
		C:  c,
		kc: kc,
	}
}

func (t *UnitedDeploymentTester) CreateUnitedDeployment(unitedDeployment *appsv1alpha1.UnitedDeployment) *appsv1alpha1.UnitedDeployment {
	Logf("create UnitedDeployment (%s/%s)", unitedDeployment.Namespace, unitedDeployment.Name)
	_, err := t.kc.AppsV1alpha1().UnitedDeployments(unitedDeployment.Namespace).Create(context.TODO(), unitedDeployment, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	t.WaitForUnitedDeploymentRunning(unitedDeployment)
	Logf("create unitedDeployment (%s/%s) success", unitedDeployment.Namespace, unitedDeployment.Name)
	unitedDeployment, _ = t.kc.AppsV1alpha1().UnitedDeployments(unitedDeployment.Namespace).Get(context.TODO(), unitedDeployment.Name, metav1.GetOptions{})
	return unitedDeployment
}

func (t *UnitedDeploymentTester) GetUnitedDeployment(namespace, name string) (*appsv1alpha1.UnitedDeployment, error) {
	Logf("Get UnitedDeployment (%s/%s)", namespace, name)
	return t.kc.AppsV1alpha1().UnitedDeployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *UnitedDeploymentTester) WaitForUnitedDeploymentRunning(ws *appsv1alpha1.UnitedDeployment) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := t.kc.AppsV1alpha1().UnitedDeployments(ws.Namespace).Get(context.TODO(), ws.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Generation == inner.Status.ObservedGeneration && *inner.Spec.Replicas == inner.Status.Replicas &&
				*inner.Spec.Replicas == inner.Status.UpdatedReplicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for unitedDeployment to enter running: %v", pollErr)
	}
}

func (t *UnitedDeploymentTester) NewBaseUnitedDeploymentSpec(workload string) *appsv1alpha1.UnitedDeploymentSpec {
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app": "nginx",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "nginx:1",
				},
			},
		},
	}
	var res *appsv1alpha1.UnitedDeploymentSpec
	switch workload {
	case "CloneSet":
		res = &appsv1alpha1.UnitedDeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"annotation-a": "value-a",
						},
						Labels: map[string]string{
							"app":     "nginx",
							"label-a": "value-a",
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "nginx",
							},
						},
						Template: podTemplate,
					},
				},
			},
		}
	case "Deployment":
		res = &appsv1alpha1.UnitedDeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"annotation-a": "value-a",
						},
						Labels: map[string]string{
							"app":     "nginx",
							"label-a": "value-a",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "nginx",
							},
						},
						Template: podTemplate,
					},
				},
			},
		}
	case "StatefulSet":
		res = &appsv1alpha1.UnitedDeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"annotation-a": "value-a",
						},
						Labels: map[string]string{
							"app":     "nginx",
							"label-a": "value-a",
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "nginx",
							},
						},
						Template: podTemplate,
					},
				},
			},
		}
	case "AdvancedStatefulSet":
		res = &appsv1alpha1.UnitedDeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				AdvancedStatefulSetTemplate: &appsv1alpha1.AdvancedStatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"annotation-a": "value-a",
						},
						Labels: map[string]string{
							"app":     "nginx",
							"label-a": "value-a",
						},
					},
					Spec: appsv1beta1.StatefulSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "nginx",
							},
						},
						Template: podTemplate,
					},
				},
			},
		}

	}
	return res
}

func (t *UnitedDeploymentTester) GetSelectorPods(namespace string, selector *metav1.LabelSelector) ([]corev1.Pod, error) {
	faster, err := util.GetFastLabelSelector(selector)
	if err != nil {
		return nil, err
	}
	podList, err := t.C.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: faster.String()})
	if err != nil {
		return nil, err
	}

	matchedPods := make([]corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		if kubecontroller.IsPodActive(&podList.Items[i]) {
			matchedPods = append(matchedPods, podList.Items[i])
		}
	}
	return matchedPods, nil
}

func (t *UnitedDeploymentTester) SetNodeLabel(c clientset.Interface, node *corev1.Node, key, value string) {
	labels := node.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[key] = value
	node.SetLabels(labels)
	_, err := c.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}
