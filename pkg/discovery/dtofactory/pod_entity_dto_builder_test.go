package dtofactory

import (
	"testing"

	"github.com/turbonomic/kubeturbo/pkg/discovery/metrics"
	"github.com/turbonomic/turbo-go-sdk/pkg/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api "k8s.io/client-go/pkg/api/v1"
	"reflect"
)

var builder = &podEntityDTOBuilder{
	generalBuilder: newGeneralBuilder(metrics.NewEntityMetricSink()),
}

func Test_podEntityDTOBuilder_getPodCommoditiesSold_Error(t *testing.T) {
	testGetCommoditiesWithError(t, builder.getPodCommoditiesSold)
}

func Test_podEntityDTOBuilder_getPodCommoditiesBought_Error(t *testing.T) {
	testGetCommoditiesWithError(t, builder.getPodCommoditiesBought)
}

func Test_podEntityDTOBuilder_getPodCommoditiesBoughtFromQuota_Error(t *testing.T) {
	if _, err := builder.getPodCommoditiesBoughtFromQuota("quota1", &api.Pod{}, 100.0); err == nil {
		t.Errorf("Error thrown expected")
	}
}

func Test_podEntityDTOBuilder_createContainerPodData(t *testing.T) {
	podIP := "1.1.1.1"
	hostIP := "2.2.2.2"
	namespace := "foo"
	podName := "bar"
	port := "not-set"

	tests := []struct {
		name string
		pod  *api.Pod
		want *proto.EntityDTO_ContainerPodData
	}{
		{
			name: "test-pod-with-empty-IP",
			pod:  createPodWithIPs("", hostIP),
			want: nil,
		},
		{
			name: "test-pod-with-same-host-IP",
			pod:  createPodWithIPs(podIP, podIP),
			want: nil,
		},
		{
			name: "test-pod-with-different-IP",
			pod:  createPodWithIPs(podIP, hostIP),
			want: &proto.EntityDTO_ContainerPodData{
				IpAddress: &podIP,
				FullName:  &podName,
				Namespace: &namespace,
				Port:      &port,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := builder.createContainerPodData(tt.pod); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got = %v while want = %v", got, tt.want)
			}

		})
	}
}

func TestCreateTolerationAccessComms(t *testing.T) {
	t1 := newTaint("k1", "v1", api.TaintEffectNoExecute)
	t2 := newTaint("k2", "v2", api.TaintEffectNoSchedule)
	t3 := newTaint("k3", "v3", api.TaintEffectPreferNoSchedule)
	key1 := "k1=v1:NoExecute"
	key2 := "k2=v2:NoSchedule"
	n1 := newNodeWithTaints([]api.Taint{t1})
	n2 := newNodeWithTaints([]api.Taint{t2})
	n3 := newNodeWithTaints([]api.Taint{t3})
	nodes := []*api.Node{n1, n2, n3}

	taintCollection := getTaintCollection(nodes)

	podNoTole := newPodWithTolerations([]api.Toleration{})

	testTolerationAccessComms(t, podNoTole, taintCollection, []string{key1, key2})

	tole1 := newToleration("k1", "v1", api.TaintEffectNoExecute, api.TolerationOpEqual)
	tole2 := newToleration("k2", "v2", api.TaintEffectNoSchedule, api.TolerationOpEqual)

	pod2 := newPodWithTolerations([]api.Toleration{tole1})
	pod3 := newPodWithTolerations([]api.Toleration{tole1, tole2})

	testTolerationAccessComms(t, pod2, taintCollection, []string{key2})

	testTolerationAccessComms(t, pod3, taintCollection, []string{})
}

func testGetCommoditiesWithError(t *testing.T, f func(pod *api.Pod, cpuFrequency float64) ([]*proto.CommodityDTO, error)) {
	if _, err := f(&api.Pod{}, 100.0); err == nil {
		t.Errorf("Error thrown expected")
	}
}

func createPodWithIPs(podIP, hostIP string) *api.Pod {
	status := api.PodStatus{
		PodIP:  podIP,
		HostIP: hostIP,
	}

	return &api.Pod{
		Status:     status,
		ObjectMeta: metav1.ObjectMeta{Namespace: "foo", Name: "bar"},
	}
}

func newPodWithTolerations(tolerations []api.Toleration) *api.Pod {
	return &api.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-foo",
			UID:  "uid-foo",
		},

		Spec: api.PodSpec{
			Tolerations: tolerations,
		},
	}
}

func newToleration(key, value string, effect api.TaintEffect, tolerationOp api.TolerationOperator) api.Toleration {
	toleration := api.Toleration{
		Key:      key,
		Value:    value,
		Effect:   effect,
		Operator: tolerationOp,
	}

	return toleration
}

func testTolerationAccessComms(t *testing.T, pod *api.Pod, taintCollection map[api.Taint]string, keys []string) {
	comms, err := createTolerationAccessComms(pod, taintCollection)

	if err != nil {
		t.Errorf("Error: %v", err)
	}

	if len(comms) != len(keys) {
		t.Errorf("Expected to get %d commodities but got %d", len(keys), len(comms))
	}

	// Don't care the order
	commsMap := make(map[string]struct{})
	for i := range comms {
		commsMap[comms[i].GetKey()] = struct{}{}
	}

	for _, key := range keys {
		if _, ok := commsMap[key]; !ok {
			t.Errorf("The commodity with key %s not found", key)
		}
	}
}
