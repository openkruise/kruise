package podreadiness

import (
	"sort"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/podadapter"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Interface interface {
	AddNotReadyKey(pod *v1.Pod, msg Message) error
	RemoveNotReadyKey(pod *v1.Pod, msg Message) error
}

type realCommonControl struct {
	adp podadapter.Adapter
}

func NewCommon(c client.Client) Interface {
	return &realCommonControl{adp: &podadapter.AdapterRuntimeClient{Client: c}}
}

func NewCommonForAdapter(adp podadapter.Adapter) Interface {
	return &realCommonControl{adp: adp}
}

func (c *realCommonControl) AddNotReadyKey(pod *v1.Pod, msg Message) error {
	_, err := addNotReadyKey(c.adp, pod, msg, appspub.KruisePodReadyConditionType)
	return err
}

func (c *realCommonControl) RemoveNotReadyKey(pod *v1.Pod, msg Message) error {
	_, err := removeNotReadyKey(c.adp, pod, msg, appspub.KruisePodReadyConditionType)
	return err
}

type Message struct {
	UserAgent string `json:"userAgent"`
	Key       string `json:"key"`
}

type messageList []Message

func (c messageList) Len() int      { return len(c) }
func (c messageList) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c messageList) Less(i, j int) bool {
	if c[i].UserAgent == c[j].UserAgent {
		return c[i].Key < c[j].Key
	}
	return c[i].UserAgent < c[j].UserAgent
}

func (c messageList) dump() string {
	sort.Sort(c)
	return util.DumpJSON(c)
}
