package inplaceupdate

import (
	"errors"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/metrics/update"
	"github.com/openkruise/kruise/pkg/util/podadapter"
)

const AnnotationRestartCountInfoKey = "inplace.apps.kruise.io/restart-count-info"

type RestartCountInfo = map[string]int32

func (c *realControl) RefreshRestartCountBaseToPod(pod *v1.Pod) (gotPod *v1.Pod, err error) {
	if len(pod.Status.ContainerStatuses) == 0 {
		return pod, errors.New("no container status")
	}
	base := RestartCountInfo{}
	for _, cs := range pod.Status.ContainerStatuses {
		base[cs.Name] = cs.RestartCount
	}

	info := util.DumpJSON(base)
	podCopy := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}
	if adp, ok := c.podAdapter.(podadapter.AdapterWithPatch); ok {
		annoData := map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					AnnotationRestartCountInfoKey: info,
				},
			},
		}
		body := util.DumpJSON(annoData)
		gotPod, err = adp.PatchPod(podCopy, client.RawPatch(types.StrategicMergePatchType, []byte(body)))
	}
	return gotPod, err
}

func setPodRestartCountBase(pod *v1.Pod, base string) {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[AnnotationRestartCountInfoKey] = base
}

func CalcInplaceUpdateDuration(pod *v1.Pod) {
	calcInplaceUpdateDuration(pod, update.RecordUpdateDuration)
}

// why need this func? make ut easier
func calcInplaceUpdateDuration(pod *v1.Pod, fn func(updateType string, seconds int)) {
	if pod == nil || pod.Annotations == nil {
		return
	}
	state, err := appspub.ParseInPlaceUpdateState(pod)
	if state == nil || err != nil {
		return
	}
	inplaceType := update.GetInplaceUpdateType(state.UpdateImages, state.UpdateEnvFromMetadata, state.UpdateResources)
	fn(inplaceType, int(time.Since(state.UpdateTimestamp.Time).Seconds()))
}
