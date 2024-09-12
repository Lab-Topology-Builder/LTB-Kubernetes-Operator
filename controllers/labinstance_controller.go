//+kubebuilder:scaffold:scheme

package controllers

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	network "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	ltbv1alpha1 "github.com/Lab-Topology-Builder/LTB-K8s-Backend/api/v1alpha1"
)

type LabInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type ReturnToReconciler struct {
	shouldReturn bool
	result       ctrl.Result
	err          error
}

//+kubebuilder:rbac:groups=ltb-backend.ltb,resources=labinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ltb-backend.ltb,resources=labinstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ltb-backend.ltb,resources=labinstances/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines/finalizers,verbs=update
//+kubebuilder:rbac:groups=subresources.kubevirt.io,resources=virtualmachineinstances/console,verbs=get;list;create;update;delete
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/log,verbs=get;list
//+kubebuilder:rbac:groups="",resources=pods/exec,verbs=create;update;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

func (r *LabInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	var err error
	labInstance := &ltbv1alpha1.LabInstance{}
	err = r.Get(ctx, req.NamespacedName, labInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("LabInstance resource not found. Ignoring since object must be deleted", "IsNotFound", err)
			return ctrl.Result{Requeue: false}, client.IgnoreNotFound(err)
		}
		log.Error(err, "Failed to get LabInstance")
		return ctrl.Result{}, err
	}

	labTemplate := &ltbv1alpha1.LabTemplate{}
	retValue := r.GetLabTemplate(ctx, labInstance, labTemplate)
	if retValue.shouldReturn {
		return retValue.result, retValue.err
	}

	// Reconcile Network
	retValue = r.ReconcileNetwork(ctx, labInstance)
	if retValue.shouldReturn {
		return retValue.result, retValue.err
	}

	// Reconcile TTYD Service Account
	sa := &corev1.ServiceAccount{}
	sa.Name = labInstance.Name + "-ttyd-svcacc"
	retValue = r.ReconcileResource(labInstance, sa, nil, "")
	if retValue.shouldReturn {
		return retValue.result, retValue.err
	}

	// Reconcile TTYD Role
	role := &rbacv1.Role{}
	role.Name = labInstance.Name + "-ttyd-role"
	retValue = r.ReconcileResource(labInstance, role, nil, "")
	if retValue.shouldReturn {
		return retValue.result, retValue.err
	}

	// Reconcile TTYD Role Binding
	roleBinding := &rbacv1.RoleBinding{}
	roleBinding.Name = labInstance.Name + "-ttyd-rolebind"
	retValue = r.ReconcileResource(labInstance, roleBinding, nil, "")
	if retValue.shouldReturn {
		return retValue.result, retValue.err
	}

	// Reconcile TTYD Service
	ttydService := &corev1.Service{}
	ttydService.Name = labInstance.Name + "-ttyd-service"
	retValue = r.ReconcileResource(labInstance, ttydService, nil, "")
	if retValue.shouldReturn {
		return retValue.result, retValue.err
	}

	// Reconcile TTYD Pod
	ttydPod := &corev1.Pod{}
	ttydPod.Name = labInstance.Name + "-ttyd-pod"
	retValue = r.ReconcileResource(labInstance, ttydPod, nil, "")
	if retValue.shouldReturn {
		return retValue.result, retValue.err
	}

	nodes := labTemplate.Spec.Nodes
	pods := []*corev1.Pod{}
	vms := []*kubevirtv1.VirtualMachine{}
	for _, node := range nodes {
		nodeType := &ltbv1alpha1.NodeType{}
		retValue = r.GetNodeType(ctx, &node.NodeTypeRef, nodeType)
		if retValue.shouldReturn {
			return retValue.result, retValue.err
		}
		if nodeType.Spec.Kind == "vm" {
			virtualMachine := &kubevirtv1.VirtualMachine{}
			virtualMachine.Name = labInstance.Name + "-" + node.Name
			retValue := r.ReconcileResource(labInstance, virtualMachine, &node, "")
			if retValue.shouldReturn {
				return retValue.result, retValue.err
			}
			vms = append(vms, virtualMachine)
		} else {
			pod := &corev1.Pod{}
			pod.Name = labInstance.Name + "-" + node.Name
			retValue := r.ReconcileResource(labInstance, pod, &node, "")
			if retValue.shouldReturn {
				return retValue.result, retValue.err
			}
			pods = append(pods, pod)
		}

		// Reconcile Remote Access Service
		if len(node.Ports) > 0 {
			service := &corev1.Service{}
			service.Name = labInstance.Name + "-" + node.Name + "-remote-access"
			retValue = r.ReconcileResource(labInstance, service, &node, "")
			if retValue.shouldReturn {
				return retValue.result, retValue.err
			}
		}

		// Reconcile Ingress
		ingress := &networkingv1.Ingress{}
		ingress.Name = labInstance.Namespace + "-" + labInstance.Name + "-" + node.Name
		retValue = r.ReconcileResource(labInstance, ingress, &node, nodeType.Spec.Kind)
		if retValue.shouldReturn {
			return retValue.result, retValue.err
		}

	}

	err = UpdateLabInstanceStatus(pods, vms, labInstance)
	if err != nil {
		log.Error(err, "Failed set new status for LabInstance")
		return ctrl.Result{}, err
	}

	err = r.Status().Update(ctx, labInstance)
	if err != nil {
		log.Error(err, "Failed to update LabInstance status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *LabInstanceReconciler) ReconcileNetwork(ctx context.Context, labInstance *ltbv1alpha1.LabInstance) ReturnToReconciler {
	log := log.FromContext(ctx)
	retValue := ReturnToReconciler{shouldReturn: true, result: ctrl.Result{}, err: nil}
	if labInstance == nil {
		retValue.err = errors.NewBadRequest("labInstance is nil")
		return retValue
	}
	podNetworkDefinitionName := labInstance.Name + "-pod"
	vmNetworkDefinitionName := labInstance.Name + "-vm"
	networkdefinitionNames := []string{podNetworkDefinitionName, vmNetworkDefinitionName}
	for _, networkDefinitionName := range networkdefinitionNames {
		foundNetworkAttachmentDefinition := &network.NetworkAttachmentDefinition{}
		err := r.Get(ctx, types.NamespacedName{Name: networkDefinitionName, Namespace: labInstance.Namespace}, foundNetworkAttachmentDefinition)
		if errors.IsNotFound(err) {
			networkAttachmentDefinition := &network.NetworkAttachmentDefinition{}
			networkAttachmentDefinition.Name = networkDefinitionName
			networkAttachmentDefinition.Namespace = labInstance.Namespace
			if networkDefinitionName == podNetworkDefinitionName {
				// Don't change mode to "passthru" as it will takeover the kubernetes node interface and cause a network outage
				networkAttachmentDefinition.Spec.Config = `{
				"cniVersion": "0.3.1",
				"name": "mynet",
				"type": "bridge",
				"bridge": "mynet0",
				"ipam": {
					"type": "host-local",
					"ranges": [
						[ {
							"subnet": "10.10.0.0/24",
							"rangeStart": "10.10.0.10",
							"rangeEnd": "10.10.0.250"
						} ]
					]
				}
			}`
			} else {
				networkAttachmentDefinition.Spec.Config = `{
					"cniVersion": "0.3.1",
					"name": "mynet",
					"type": "bridge",
					"bridge": "mynet0",
					"ipam": {}
				}`
			}
			ctrl.SetControllerReference(labInstance, networkAttachmentDefinition, r.Scheme)
			log.Info("Creating a new NetworkAttachmentDefinition", "NetworkAttachmentDefinition.Namespace", networkAttachmentDefinition.Namespace, "NetworkAttachmentDefinition.Name", networkAttachmentDefinition.Name)

			err = r.Create(ctx, networkAttachmentDefinition)
			if err != nil {
				retValue.err = err
				log.Error(err, "Failed to create NetworkAttachmentDefinition")
				return retValue
			}
			retValue.result = ctrl.Result{Requeue: true}
			return retValue
		}
		if err != nil {
			retValue.err = err
			log.Error(err, "Failed to get NetworkAttachmentDefinition")
			return retValue
		}
	}
	retValue.shouldReturn = false
	return retValue
}

func (r *LabInstanceReconciler) ReconcileResource(labInstance *ltbv1alpha1.LabInstance, resource client.Object, node *ltbv1alpha1.LabInstanceNodes, nodeKind string) ReturnToReconciler {
	ctx := context.Context(context.Background())
	log := log.FromContext(ctx)
	retValue := ReturnToReconciler{shouldReturn: true, result: ctrl.Result{}, err: nil}
	if labInstance == nil {
		retValue.err = errors.NewBadRequest("labInstance is nil")
		return retValue
	}
	resource.SetNamespace(labInstance.Namespace)
	resourceExists, err := r.ResourceExists(resource)
	if err != nil && !resourceExists {
		resource, err := CreateResource(labInstance, node, resource, nodeKind)
		if err != nil {
			retValue.err = err
			log.Error(err, "Failed to create new resource", "resource.Namespace", labInstance.Namespace, "resource.Name", reflect.TypeOf(resource).Elem().Name())
			return retValue
		}
		log.Info("Creating a new resource", "resource.Namespace", labInstance.Namespace, "resource.Name", reflect.ValueOf(resource).Elem().FieldByName("Name"))
		ctrl.SetControllerReference(labInstance, resource, r.Scheme)

		err = r.Create(ctx, resource)
		if err != nil {
			retValue.err = err
			log.Error(err, "Failed to create new resource", "resource.Namespace", labInstance.Namespace, "resource.Name", reflect.TypeOf(resource).Elem().Name())
			return retValue
		}
		retValue.result = ctrl.Result{Requeue: true}
		return retValue
	} else if err != nil {
		retValue.err = err
		log.Error(err, "Failed to get resource")
		return retValue
	}
	retValue.shouldReturn = false
	return retValue
}

func CreateResource(labInstance *ltbv1alpha1.LabInstance, node *ltbv1alpha1.LabInstanceNodes, resource client.Object, kind string) (client.Object, error) {
	ctx := context.Context(context.Background())
	log := log.FromContext(ctx)
	var err error
	switch reflect.TypeOf(resource).Elem().Name() {
	case "Pod":
		resource, err = CreatePod(labInstance, node)
		if err != nil {
			log.Error(err, "Failed to create Pod")
			return nil, err
		}
	case "VirtualMachine":
		resource, err = MapTemplateToVM(labInstance, node)
		if err != nil {
			log.Error(err, "Failed to create VirtualMachine")
			return nil, err
		}
	case "Service":
		resource, err = CreateService(labInstance, node)
		if err != nil {
			log.Error(err, "Failed to create Service")
			return nil, err
		}
	case "Ingress":
		resource, err = CreateIngress(labInstance, node, kind)
		if err != nil {
			log.Error(err, "Failed to create Ingress")
			return nil, err
		}
	case "Role":
		_, role, _ := CreateSvcAccRoleRoleBind(labInstance)
		resource = role
	case "ServiceAccount":
		svcAcc, _, _ := CreateSvcAccRoleRoleBind(labInstance)
		resource = svcAcc
	case "RoleBinding":
		_, _, roleBind := CreateSvcAccRoleRoleBind(labInstance)
		resource = roleBind
	default:
		log.Error(fmt.Errorf("resource type not supported"), "Unsupported", "ResourceKind", reflect.TypeOf(resource).Elem().Name())
		return resource, errors.NewBadRequest(fmt.Sprintf("Resource type not supported: %s", reflect.TypeOf(resource).Elem().Name()))
	}
	return resource, nil

}

func (r *LabInstanceReconciler) ResourceExists(resource client.Object) (bool, error) {
	ctx := context.Context(context.Background())
	resourceName := reflect.ValueOf(resource).Elem().FieldByName("Name").String()
	nameSpace := reflect.ValueOf(resource).Elem().FieldByName("Namespace").String()
	err := r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: nameSpace}, resource)
	if errors.IsNotFound(err) {
		return false, err
	} else if err != nil {
		return true, err
	}
	return true, nil
}

func (r *LabInstanceReconciler) GetLabTemplate(ctx context.Context, labInstance *ltbv1alpha1.LabInstance, labTemplate *ltbv1alpha1.LabTemplate) ReturnToReconciler {
	log := log.FromContext(ctx)
	returnValue := ReturnToReconciler{shouldReturn: false, result: ctrl.Result{}, err: nil}
	err := r.Get(ctx, types.NamespacedName{Name: labInstance.Spec.LabTemplateReference, Namespace: labInstance.Namespace}, labTemplate)
	if err != nil && errors.IsNotFound(err) {
		log.Info("LabTemplate not found", "LabTemplate", labInstance.Spec.LabTemplateReference)
		returnValue.shouldReturn = true
		returnValue.err = err
		return returnValue
	} else if err != nil {
		returnValue.shouldReturn = true
		returnValue.err = err
		log.Error(err, "Failed to get LabTemplate")
		return returnValue
	}
	return returnValue
}

func (r *LabInstanceReconciler) GetNodeType(ctx context.Context, nodeTypeRef *ltbv1alpha1.NodeTypeRef, nodeType *ltbv1alpha1.NodeType) ReturnToReconciler {
	log := log.FromContext(ctx)
	returnValue := ReturnToReconciler{shouldReturn: false, result: ctrl.Result{}, err: nil}
	err := r.Get(ctx, types.NamespacedName{Name: nodeTypeRef.Type}, nodeType)
	if err != nil && errors.IsNotFound(err) {
		log.Info("NodeType not found", "NodeType", nodeTypeRef.Type)
		returnValue.shouldReturn = true
		returnValue.err = err
		return returnValue
	} else if err != nil {
		returnValue.shouldReturn = true
		returnValue.err = err
		log.Error(err, "Failed to get NodeType")
		return returnValue
	}
	return returnValue
}

func MapTemplateToPod(labInstance *ltbv1alpha1.LabInstance, node *ltbv1alpha1.LabInstanceNodes) (*corev1.Pod, error) {
	log := log.FromContext(context.Background())
	if node == nil {
		return nil, errors.NewBadRequest("Node is nil")
	}
	if labInstance == nil {
		return nil, errors.NewBadRequest("LabInstance is nil")
	}
	metadata := metav1.ObjectMeta{
		Name:      labInstance.Name + "-" + node.Name,
		Namespace: labInstance.Namespace,
		Annotations: map[string]string{
			"k8s.v1.cni.cncf.io/networks": labInstance.Name + "-pod",
		},
		Labels: map[string]string{
			"app": labInstance.Name + "-" + node.Name + "-remote-access",
		},
	}
	podSpec := &corev1.PodSpec{}
	err := yaml.Unmarshal([]byte(node.RenderedNodeSpec), podSpec)
	if err != nil {
		log.Error(err, "Failed to unmarshal node spec")
		return nil, err
	}
	log.Info("PodSpec", "Spec applied to Pod", podSpec)
	pod := &corev1.Pod{
		ObjectMeta: metadata,
		Spec:       *podSpec,
	}
	return pod, nil
}

func MapTemplateToVM(labInstance *ltbv1alpha1.LabInstance, node *ltbv1alpha1.LabInstanceNodes) (*kubevirtv1.VirtualMachine, error) {
	log := log.FromContext(context.Background())
	if node == nil {
		return nil, errors.NewBadRequest("Node is nil")
	}
	if labInstance == nil {
		return nil, errors.NewBadRequest("LabInstance is nil")
	}
	metadata := metav1.ObjectMeta{
		Name:      labInstance.Name + "-" + node.Name,
		Namespace: labInstance.Namespace,
	}
	vmSpec := &kubevirtv1.VirtualMachineSpec{}
	err := yaml.Unmarshal([]byte(node.RenderedNodeSpec), vmSpec)
	if err != nil {
		log.Error(err, "Failed to unmarshal node spec")
		return nil, err
	}
	networks := []kubevirtv1.Network{
		{Name: "default", NetworkSource: kubevirtv1.NetworkSource{Pod: &kubevirtv1.PodNetwork{}}},
		{Name: labInstance.Name, NetworkSource: kubevirtv1.NetworkSource{Multus: &kubevirtv1.MultusNetwork{NetworkName: labInstance.Name + "-vm"}}},
	}
	interfaces := []kubevirtv1.Interface{
		{Name: "default", InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{Bridge: &kubevirtv1.InterfaceBridge{}}},
		{Name: labInstance.Name, InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{Bridge: &kubevirtv1.InterfaceBridge{}}},
	}
	vmSpec.Template.Spec.Domain.Devices.Interfaces = interfaces
	vmSpec.Template.Spec.Networks = networks
	vmSpec.Template.ObjectMeta.Labels = map[string]string{"app": labInstance.Name + "-" + node.Name + "-remote-access"}
	log.Info("VM Spec", "Spec applied to VM", vmSpec)
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metadata,
		Spec:       *vmSpec,
	}
	return vm, nil
}

func CreateIngress(labInstance *ltbv1alpha1.LabInstance, node *ltbv1alpha1.LabInstanceNodes, kind string) (*networkingv1.Ingress, error) {
	if node == nil {
		return nil, errors.NewBadRequest("Node is nil")
	}
	if kind != "vm" && kind != "pod" {
		return nil, errors.NewBadRequest("Kind must be either vm or pod")
	}
	name := labInstance.Namespace + "-" + labInstance.Name + "-" + node.Name
	metadata := metav1.ObjectMeta{
		Name:      name,
		Namespace: labInstance.Namespace,
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/rewrite-target": "/?arg=" + kind + "&arg=" + name + "&arg=bash",
		},
	}
	className := "nginx"
	ingress := &networkingv1.Ingress{
		ObjectMeta: metadata,
		Spec: networkingv1.IngressSpec{
			IngressClassName: &className,
			Rules: []networkingv1.IngressRule{
				{Host: name + "." + labInstance.Spec.DNSAddress,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									PathType: func() *networkingv1.PathType {
										pathType := networkingv1.PathTypePrefix
										return &pathType
									}(),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: labInstance.Name + "-ttyd-service",
											Port: networkingv1.ServiceBackendPort{
												Name: "ttyd",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return ingress, nil
}

func CreatePod(labInstance *ltbv1alpha1.LabInstance, node *ltbv1alpha1.LabInstanceNodes) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	var err error
	if labInstance == nil {
		return nil, errors.NewBadRequest("LabInstance is nil")
	}

	if node == nil {
		pod.ObjectMeta = metav1.ObjectMeta{Namespace: labInstance.Namespace}
		pod.ObjectMeta.Name = labInstance.Name + "-ttyd-pod"
		pod.ObjectMeta.Labels = map[string]string{"app": labInstance.Name + "-ttyd-service"}
		pod.Spec = corev1.PodSpec{
			ServiceAccountName: labInstance.Name + "-ttyd-svcacc",
			Containers: []corev1.Container{
				{
					Name:  labInstance.Name + "-ttyd-container",
					Image: "ghcr.io/insrapperswil/kube-ttyd:latest",
					Args:  []string{"ttyd", "-a", "konnect"},
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 7681,
						},
					},
				},
			},
		}
		err = nil
	} else {
		pod, err = MapTemplateToPod(labInstance, node)
	}
	return pod, err
}

func CreateService(labInstance *ltbv1alpha1.LabInstance, node *ltbv1alpha1.LabInstanceNodes) (*corev1.Service, error) {
	var serviceName string
	ports := []corev1.ServicePort{}
	serviceType := corev1.ServiceTypeLoadBalancer
	if labInstance == nil {
		return nil, errors.NewBadRequest("LabInstance is nil")
	}

	if node == nil {
		serviceName = fmt.Sprintf("%s-%s", labInstance.Name, "ttyd-service")
		ports = append(ports, corev1.ServicePort{
			Name:       "ttyd",
			Port:       7681,
			TargetPort: intstr.FromInt(7681),
		})
		serviceType = corev1.ServiceTypeClusterIP
	} else {
		serviceName = fmt.Sprintf("%s-%s-%s", labInstance.Name, node.Name, "remote-access")
		for _, port := range node.Ports {
			ports = append(ports, corev1.ServicePort{
				Name:       port.Name,
				Port:       port.Port,
				TargetPort: intstr.IntOrString{IntVal: port.Port},
				Protocol:   port.Protocol,
			})
		}
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: labInstance.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": serviceName},
			Ports:    ports,
			Type:     serviceType,
		},
	}
	return service, nil
}

func CreateSvcAccRoleRoleBind(labInstance *ltbv1alpha1.LabInstance) (*corev1.ServiceAccount, *rbacv1.Role, *rbacv1.RoleBinding) {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labInstance.Name + "-ttyd-svcacc",
			Namespace: labInstance.Namespace,
		},
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labInstance.Name + "-ttyd-role",
			Namespace: labInstance.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "pods/log"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs:     []string{"create", "update", "delete"},
			},
			{
				APIGroups: []string{"subresources.kubevirt.io"},
				Resources: []string{"virtualmachineinstances/console"},
				Verbs:     []string{"get", "list", "create", "update", "delete"},
			},
		},
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labInstance.Name + "-ttyd-rolebind",
			Namespace: labInstance.Namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      labInstance.Name + "-ttyd-svcacc",
				Namespace: labInstance.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     labInstance.Name + "-ttyd-role",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	return serviceAccount, role, roleBinding

}

func UpdateLabInstanceStatus(pods []*corev1.Pod, vms []*kubevirtv1.VirtualMachine, labInstance *ltbv1alpha1.LabInstance) error {
	var podStatus corev1.PodPhase
	var vmStatus kubevirtv1.VirtualMachinePrintableStatus
	var numVMsRunning, numPodsRunning int
	if pods == nil && vms == nil {
		return errors.NewBadRequest("No resources found")
	}
	if labInstance == nil {
		return errors.NewBadRequest("LabInstance is nil")
	}
	for _, pod := range pods {
		podStatus = pod.Status.Phase
		if podStatus != corev1.PodRunning {
			break
		}
		numPodsRunning++
	}
	labInstance.Status.NumPodsRunning = fmt.Sprint(numPodsRunning) + "/" + fmt.Sprint(len(pods))

	for _, vm := range vms {
		vmStatus = vm.Status.PrintableStatus
		if !vm.Status.Ready {
			break
		}
		numVMsRunning++
	}
	labInstance.Status.NumVMsRunning = fmt.Sprint(numVMsRunning) + "/" + fmt.Sprint(len(vms))

	if podStatus == "Running" && vmStatus == "VM Ready" {
		labInstance.Status.Status = "Running"
	} else {
		if podStatus != "Running" {
			labInstance.Status.Status = string(podStatus)
		} else {
			labInstance.Status.Status = string(vmStatus)
		}
	}
	return nil
}

func (r *LabInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ltbv1alpha1.LabInstance{}).
		Owns(&corev1.Pod{}).
		Owns(&kubevirtv1.VirtualMachine{}).
		Complete(r)
}
