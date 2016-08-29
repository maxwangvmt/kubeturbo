package probe

import (
	"fmt"
	"strconv"

	"k8s.io/kubernetes/pkg/api"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"

	"github.com/vmturbo/kubeturbo/pkg/helper"

	"github.com/golang/glog"
	"github.com/vmturbo/vmturbo-go-sdk/sdk"
)

// Pods Getter is such func that gets all the pods match the provided namespace, labels and fiels.
type ServiceGetter func(namespace string, selector labels.Selector) ([]*api.Service, error)

type EndpointGetter func(namespace string, selector labels.Selector) ([]*api.Endpoints, error)

type ServiceProbe struct {
	serviceGetter  ServiceGetter
	endpointGetter EndpointGetter
}

func NewServiceProbe(serviceGetter ServiceGetter, epGetter EndpointGetter) *ServiceProbe {
	return &ServiceProbe{
		serviceGetter:  serviceGetter,
		endpointGetter: epGetter,
	}
}

type VMTServiceGetter struct {
	kubeClient *client.Client
}

func NewVMTServiceGetter(kubeClient *client.Client) *VMTServiceGetter {
	return &VMTServiceGetter{
		kubeClient: kubeClient,
	}
}

// Get service match specified namesapce and label.
func (this *VMTServiceGetter) GetService(namespace string, selector labels.Selector) ([]*api.Service, error) {
	listOption := &api.ListOptions{
		LabelSelector: selector,
	}
	serviceList, err := this.kubeClient.Services(namespace).List(*listOption)
	if err != nil {
		return nil, fmt.Errorf("Error listing services: %s", err)
	}

	var serviceItems []*api.Service
	for _, service := range serviceList.Items {
		s := service
		serviceItems = append(serviceItems, &s)
	}

	glog.V(2).Infof("Discovering Services, now the cluster has " + strconv.Itoa(len(serviceItems)) + " services")

	return serviceItems, nil
}

type VMTEndpointGetter struct {
	kubeClient *client.Client
}

func NewVMTEndpointGetter(kubeClient *client.Client) *VMTEndpointGetter {
	return &VMTEndpointGetter{
		kubeClient: kubeClient,
	}
}

// Get endpoints match specified namesapce and label.
func (this *VMTEndpointGetter) GetEndpoints(namespace string, selector labels.Selector) ([]*api.Endpoints, error) {
	listOption := &api.ListOptions{
		LabelSelector: selector,
	}
	epList, err := this.kubeClient.Endpoints(namespace).List(*listOption)
	if err != nil {
		return nil, fmt.Errorf("Error listing endpoints: %s", err)
	}

	var epItems []*api.Endpoints
	for _, endpoint := range epList.Items {
		ep := endpoint
		epItems = append(epItems, &ep)
	}

	return epItems, nil
}

func (this *ServiceProbe) GetService(namespace string, selector labels.Selector) ([]*api.Service, error) {
	if this.serviceGetter == nil {
		return nil, fmt.Errorf("Service getter is not set")
	}
	return this.serviceGetter(namespace, selector)
}

func (this *ServiceProbe) GetEndpoints(namespace string, selector labels.Selector) ([]*api.Endpoints, error) {
	if this.endpointGetter == nil {
		return nil, fmt.Errorf("Endpoint getter is not set")
	}
	return this.endpointGetter(namespace, selector)
}

// Parse Services inside Kubernetes and build entityDTO as VApp.
func (this *ServiceProbe) ParseService(serviceList []*api.Service, endpointList []*api.Endpoints) (result []*sdk.EntityDTO, err error) {
	// first make a endpoint map, key is endpoints label, value is endoint object
	endpointMap := make(map[string]*api.Endpoints)
	for _, endpoint := range endpointList {
		nameWithNamespace := endpoint.Namespace + "/" + endpoint.Name
		endpointMap[nameWithNamespace] = endpoint
	}

	for _, service := range serviceList {
		serviceEndpointMap := this.findBackendPodPerService(service, endpointMap)

		// Now build entityDTO
		for serviceID, podIDList := range serviceEndpointMap {
			glog.V(4).Infof("service %s has the following pod as endpoints %s", serviceID, podIDList)

			if len(podIDList) < 1 {
				continue
			}

			// processMap := pod2AppMap[podIDList[0]]

			commoditiesBoughtMap := this.getCommoditiesBought(podIDList)

			entityDto := this.buildEntityDTO(serviceID, commoditiesBoughtMap)

			result = append(result, entityDto)

		}
	}

	return
}

func (this *ServiceProbe) findBackendPodPerService(service *api.Service, endpointMap map[string]*api.Endpoints) map[string][]string {
	// key is service identifier, value is the string list of the pod name with namespace
	serviceEndpointMap := make(map[string][]string)

	serviceNameWithNamespace := service.Namespace + "/" + service.Name
	serviceEndpoint := endpointMap[serviceNameWithNamespace]
	if serviceEndpoint == nil {
		return nil
	}
	subsets := serviceEndpoint.Subsets
	for _, endpointSubset := range subsets {
		addresses := endpointSubset.Addresses
		for _, address := range addresses {
			target := address.TargetRef
			if target == nil {
				continue
			}
			podName := target.Name
			podNamespace := target.Namespace
			podNameWithNamespace := podNamespace + "/" + podName
			// get the pod name and the service name
			var podIDList []string
			if pList, exists := serviceEndpointMap[serviceNameWithNamespace]; exists {
				podIDList = pList
			}
			podIDList = append(podIDList, podNameWithNamespace)
			serviceEndpointMap[serviceNameWithNamespace] = podIDList
		}
	}
	return serviceEndpointMap
}

func (this *ServiceProbe) buildEntityDTO(serviceName string, commoditiesBoughtMap map[*sdk.ProviderDTO][]*sdk.CommodityDTO) *sdk.EntityDTO {
	serviceEntityType := sdk.EntityDTO_VIRTUAL_APPLICATION
	id := "vApp-" + serviceName + "-" + ClusterID
	dispName := id
	entityDTOBuilder := sdk.NewEntityDTOBuilder(serviceEntityType, id)

	entityDTOBuilder = entityDTOBuilder.DisplayName(dispName)
	for provider, commodities := range commoditiesBoughtMap {
		entityDTOBuilder.SetProvider(provider)
		entityDTOBuilder.BuysCommodities(commodities)
	}
	entityDto := entityDTOBuilder.Create()

	glog.V(4).Infof("created a service entityDTO %v", entityDto)
	return entityDto
}

func (this *ServiceProbe) getCommoditiesBought(podIDList []string) map[*sdk.ProviderDTO][]*sdk.CommodityDTO {
	commoditiesBoughtMap := make(map[*sdk.ProviderDTO][]*sdk.CommodityDTO)

	for _, podID := range podIDList {
		serviceResourceStat := getServiceResourceStat(podTransactionCountMap, podID)
		appName := podID
		appID := appPrefix + appName
		// We might want to check here if the appID exist.
		appProvider := sdk.CreateProvider(sdk.EntityDTO_APPLICATION, appID)
		var commoditiesBoughtFromApp []*sdk.CommodityDTO
		transactionCommBought := sdk.NewCommodityDTOBuilder(sdk.CommodityDTO_TRANSACTION).
			Key(appName).
			Used(serviceResourceStat.transactionBought).
			Create()
		commoditiesBoughtFromApp = append(commoditiesBoughtFromApp, transactionCommBought)

		commoditiesBoughtMap[appProvider] = commoditiesBoughtFromApp

	}
	return commoditiesBoughtMap
}

func getServiceResourceStat(transactionCountMap map[string]float64, podID string) *ServiceResourceStat {
	transactionBought := float64(0)

	count, ok := transactionCountMap[podID]
	if ok {
		transactionBought = count
		glog.V(4).Infof("Transaction bought from pod %s is %d", podID, count)
	} else {
		glog.V(4).Infof("No transaction value for applications on pod %s", podID)
	}

	flag, err := helper.LoadTestingFlag()
	if err == nil {
		if flag.ProvisionTestingFlag || flag.DeprovisionTestingFlag {
			if fakeUtil := flag.FakeTransactionUtil; fakeUtil != 0 {
				transactionBought = fakeUtil * 1000
			}
		}
	}

	return &ServiceResourceStat{
		transactionBought: transactionBought,
	}
}
