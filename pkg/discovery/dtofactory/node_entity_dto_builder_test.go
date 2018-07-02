package dtofactory

import (
	"fmt"
	"github.com/golang/glog"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api "k8s.io/client-go/pkg/api/v1"
)

func TestQuantity(t *testing.T) {
	q1 := resource.NewQuantity(1000, resource.DecimalSI)
	q2 := resource.NewQuantity(1024, resource.BinarySI)

	fmt.Printf("q1 = %++v\n", q1)
	fmt.Printf("q2 = %++v\n", q2)
}

func TestCPUQuantity(t *testing.T) {
	cpuTime1 := "1000m"
	cpuTime2 := "2500m"

	r1, err := resource.ParseQuantity(cpuTime1)
	if err != nil {
		t.Error(err)
		return
	}
	glog.V(1).Infof("cputime1(%s): %++v", cpuTime1, r1)

	r2, err := resource.ParseQuantity(cpuTime2)
	if err != nil {
		t.Error(err)
		return
	}
	glog.V(1).Infof("cputime2(%s): %++v", cpuTime2, r2)
}

func genMemQuantity(numbytes int64) resource.Quantity {
	numkb := int(numbytes / 1024.0)
	result, err := resource.ParseQuantity(fmt.Sprintf("%dKi", numkb))
	if err != nil {
		glog.Errorf("Failed to parse memory quantity: %v", err)
		result = *resource.NewQuantity(1024, resource.BinarySI)
	}
	glog.V(3).Infof("result = %+v", result)

	return result
}

// input: cores--cpu cores
func genCPUQuantity(cores float32) resource.Quantity {
	cpuTime := int(cores * 1000)
	result, err := resource.ParseQuantity(fmt.Sprintf("%dm", cpuTime))
	if err != nil {
		glog.Errorf("Failed to parse cpu quantity: %v", err)
		result = *resource.NewQuantity(1000, resource.DecimalSI)
	}

	glog.V(3).Infof("result = %+v", result)
	return result
}

func buildNodeResource() (api.ResourceList, api.ResourceList) {
	capacity := make(api.ResourceList)
	allocatable := make(api.ResourceList)

	//2 cpu cores, 1.5 are allocatable
	capacity[api.ResourceCPU] = genCPUQuantity(2.0)
	allocatable[api.ResourceCPU] = genCPUQuantity(1.5)

	// 8 GB memory, 6GB are allocatable
	capacity[api.ResourceMemory] = genMemQuantity(8 * 1024 * 1024 * 1024)
	allocatable[api.ResourceMemory] = genMemQuantity(6 * 1024 * 1024 * 1024)

	return capacity, allocatable
}

func genNodeInfo() api.NodeSystemInfo {
	nodeInfo := api.NodeSystemInfo{
		MachineID:        "e414b629ea12ffdaaa044b892dd35750",
		SystemUUID:       "E414B629-EA12-FFDA-AA04-4B892DD35750",
		OSImage:          "Ubuntu 16.04.3 LTS",
		KubeletVersion:   "v1.7.8",
		KubeProxyVersion: "v1.7.8",
		OperatingSystem:  "linux",
		Architecture:     "amd64",
	}
	return nodeInfo
}

func genAddresses() []api.NodeAddress {
	addresses := []api.NodeAddress{
		api.NodeAddress{
			Type:    api.NodeExternalIP,
			Address: "32.205.107.22",
		},
		api.NodeAddress{
			Type:    api.NodeInternalIP,
			Address: "10.10.172.235",
		},
	}

	return addresses
}

func mockNode() *api.Node {
	labels := make(map[string]string)
	labels["label1"] = "value1"
	labels["label2"] = "valuel2"

	resCapaicty, resAllocatable := buildNodeResource()
	addresses := genAddresses()
	nodeInfo := genNodeInfo()

	node := &api.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   "my-node-1",
			UID:    "my-node-1-UID",
			Labels: labels,
		},

		Spec: api.NodeSpec{
			ExternalID: "2272335446120646149",
			PodCIDR:    "10.4.1.0/24",
			ProviderID: "gce://turbonomic-eng/us-central1-a/gke-cluster-default-pool-b5fbbce4-1ckk",
		},

		Status: api.NodeStatus{
			Capacity:    resCapaicty,
			Allocatable: resAllocatable,
			Addresses:   addresses,
			NodeInfo:    nodeInfo,
		},
	}

	return node
}

func TestGetNodeIPs(t *testing.T) {
	node := mockNode()

	nodeIPs := getNodeIPs(node)
	filter := make(map[string]struct{})
	for _, ip := range nodeIPs {
		filter[ip] = struct{}{}
	}

	addresses := node.Status.Addresses
	if len(nodeIPs) != len(addresses) {
		t.Errorf("number of IPs are not equal: %d Vs. %d", len(nodeIPs), len(addresses))
		return
	}

	for _, addr := range addresses {
		if _, exist := filter[addr.Address]; !exist {
			t.Errorf("not found address: %+v", addr)
		}
	}
}

func TestGetTaintCollection(t *testing.T) {
	t1 := newTaint("k1", "v1", api.TaintEffectNoExecute)
	t2 := newTaint("k2", "v2", api.TaintEffectNoSchedule)
	t3 := newTaint("k3", "v3", api.TaintEffectPreferNoSchedule)
	n1 := newNodeWithTaints([]api.Taint{t1})
	n2 := newNodeWithTaints([]api.Taint{t2})
	n3 := newNodeWithTaints([]api.Taint{t3})
	nodes := []*api.Node{n1, n2, n3}

	taintCollection := getTaintCollection(nodes)

	fmt.Printf("taintCollection: %++v", taintCollection)

	if len(taintCollection) != 2 {
		t.Errorf("Expected 2 taints but got %d", len(taintCollection))
	}

	if value, ok := taintCollection[t1]; !ok {
		t.Errorf("Taint %+v not found", t1)
	} else if value != "k1=v1:NoExecute" {
		t.Errorf("Taint %+v has wrong key %s", t1, value)
	}

	if value, ok := taintCollection[t2]; !ok {
		t.Errorf("Taint %+v not found", t2)
	} else if value != "k2=v2:NoSchedule" {
		t.Errorf("Taint %+v has wrong key %s", t2, value)
	}
}

func TestCreateTaintAccessComms(t *testing.T) {
	t1 := newTaint("k1", "v1", api.TaintEffectNoExecute)
	t2 := newTaint("k2", "v2", api.TaintEffectNoSchedule)
	t3 := newTaint("k3", "v3", api.TaintEffectPreferNoSchedule)
	n1 := newNodeWithTaints([]api.Taint{t1})
	n2 := newNodeWithTaints([]api.Taint{t2})
	n3 := newNodeWithTaints([]api.Taint{t3})
	nodes := []*api.Node{n1, n2, n3}

	taintCollection := getTaintCollection(nodes)

	comms, err := createTaintAccessComms(n1, taintCollection)

	if err != nil {
		t.Errorf("Error: %v", err)
	}

	if len(comms) != 1 {
		t.Errorf("Expected 1 commodity but got %d", len(comms))
	}

	if *(comms[0].Key) != "k2=v2:NoSchedule" {
		t.Errorf("Comm %+v has wrong key %s", comms[0], *(comms[0].Key))
	}

	comms2, err := createTaintAccessComms(n2, taintCollection)

	if len(comms2) != 1 {
		t.Errorf("Expected 1 commodity but got %d", len(comms2))
	}

	if *(comms2[0].Key) != "k1=v1:NoExecute" {
		t.Errorf("Comm %+v has wrong key %s", comms2[0], *(comms2[0].Key))
	}

	comms3, err := createTaintAccessComms(n3, taintCollection)

	if len(comms3) != 2 {
		t.Errorf("Expected 2 commodities but got %d", len(comms3))
	}

	if *(comms3[0].Key) != "k1=v1:NoExecute" || *(comms3[1].Key) != "k2=v2:NoSchedule" {
		if *(comms3[1].Key) == "k1=v1:NoExecute" && *(comms3[0].Key) == "k2=v2:NoSchedule" {

		} else {
			t.Errorf("Wrong comms3 %+v", comms3)
		}
	}
}

func newNodeWithTaints(taints []api.Taint) *api.Node {
	node := &api.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-node-1",
			UID:  "my-node-1-UID",
		},

		Spec: api.NodeSpec{
			Taints: taints,
		},
	}

	return node
}

func newTaint(key, value string, effect api.TaintEffect) api.Taint {
	taint := api.Taint{
		Key:    key,
		Value:  value,
		Effect: effect,
	}

	return taint
}
