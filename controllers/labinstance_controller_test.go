package controllers

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	ltbv1alpha1 "github.com/Lab-Topology-Builder/LTB-K8s-Backend/api/v1alpha1"
	network "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// var _ = Describe("LabInstance Controller", func() {
type fields struct {
	Client client.Client
	Scheme *runtime.Scheme
}

var (
	ctx                               context.Context
	r                                 *LabInstanceReconciler
	testLabInstance                   *ltbv1alpha1.LabInstance
	testLabTemplate                   *ltbv1alpha1.LabTemplate
	testNodeTypeVM, testNodeTypePod   *ltbv1alpha1.NodeType
	err                               error
	podNode, vmNode, testNode         *ltbv1alpha1.LabInstanceNodes
	fakeClient                        client.Client
	testPod, testNodePod, testTtydPod *corev1.Pod
	field                             fields
	testVM, testNodeVM                *kubevirtv1.VirtualMachine
	testPodIngress, testVMIngress     *networkingv1.Ingress
	testService, testTtydService      *corev1.Service
	testRole                          *rbacv1.Role
	testRoleBinding                   *rbacv1.RoleBinding
	testServiceAccount                *corev1.ServiceAccount
)

const namespace = "test-namespace"

func TestMain(m *testing.M) {
	initialize()
	code := m.Run()
	os.Exit(code)
}

func initialize() {
	ctx = context.Background()
	testLabInstance = &ltbv1alpha1.LabInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-labinstance",
			Namespace: namespace,
		},
		Spec: ltbv1alpha1.LabInstanceSpec{
			LabTemplateReference: "test-labtemplate",
		},
		Status: ltbv1alpha1.LabInstanceStatus{
			Status: "Running",
		},
	}

	nodeSpecYAMLVM := `
	running: true
	template:
	  spec:
	    domain:
	      resources:
	        requests:
	          memory: 4096M
	      cpu:
	        cores: 2
	      devices:
	        disks:
	          - name: containerdisk
	            disk:
	              bus: virtio
	    terminationGracePeriodSeconds: 0
	    volumes:
	      - name: containerdisk
	        containerDisk:
	          image: quay.io/containerdisks/ubuntu:22.04
	`

	// vmConfig := `
	//   #cloud-config
	//   password: ubuntu
	//   chpasswd: { expire: False }
	//   ssh_pwauth: True
	//   packages:
	//     - qemu-guest-agent
	//   runcmd:
	//     - [ systemctl, start, qemu-guest-agent ]
	// `

	nodeSpecYAMLPod := `
	containers:
	  - name: {{ .Name }}
	    image: {{ .NodeTypeRef.Image}}:{{ .NodeTypeRef.Version }}
	    command: ["/bin/bash", "-c", "apt update && apt install -y openssh-server && service ssh start && sleep 365d"]
	    ports:
	      {{- range $index, $port := .Ports }}
	      - name: {{ $port.Name }}
	        containerPort: {{ $port.Port }}
	        protocol: {{ $port.Protocol }}
	      {{- end }}
	`

	testNodeTypeVM = &ltbv1alpha1.NodeType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testNodeVM",
			Namespace: namespace,
		},
		Spec: ltbv1alpha1.NodeTypeSpec{
			Kind:     "vm",
			NodeSpec: nodeSpecYAMLVM,
		},
	}

	testNodeTypePod = &ltbv1alpha1.NodeType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testNodePod",
			Namespace: namespace,
		},
		Spec: ltbv1alpha1.NodeTypeSpec{
			Kind: "pod",
			//			NodeSpec: nodeSpecYAMLPod,
		},
	}

	testLabTemplate = &ltbv1alpha1.LabTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-labtemplate",
			Namespace: namespace,
		},
		Spec: ltbv1alpha1.LabTemplateSpec{
			Nodes: []ltbv1alpha1.LabInstanceNodes{
				{
					Name: "test-node-0",
					NodeTypeRef: ltbv1alpha1.NodeTypeRef{
						Type:    testNodeTypeVM.Name,
						Image:   "ubuntu",
						Version: "22.04",
					},
					Ports: []ltbv1alpha1.Port{
						{
							Name:     "test-ssh-port",
							Protocol: "TCP",
							Port:     22,
						},
					},
					RenderedNodeSpec: nodeSpecYAMLVM,
				},
				{
					Name: "test-node-1",
					NodeTypeRef: ltbv1alpha1.NodeTypeRef{
						Type:    testNodeTypePod.Name,
						Image:   "ubuntu",
						Version: "20.04",
					},
					RenderedNodeSpec: nodeSpecYAMLPod,
				},
				{
					Name: "test-node-2",
					NodeTypeRef: ltbv1alpha1.NodeTypeRef{
						Type:    "test",
						Image:   "ubuntu",
						Version: "20.04",
					},
					Ports: []ltbv1alpha1.Port{
						{
							Name:     "test-ssh-port",
							Protocol: "TCP",
							Port:     22,
						},
					},
				},
			},
		},
	}
	vmNode = &testLabTemplate.Spec.Nodes[0]
	podNode = &testLabTemplate.Spec.Nodes[1]
	testNode = &testLabTemplate.Spec.Nodes[2]

	testPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + podNode.Name,
			Namespace: namespace,
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": testLabInstance.Name + "-pod",
			},
			Labels: map[string]string{
				"app": testLabInstance.Name + "-" + podNode.Name + "-remote-access",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	testVM = &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + vmNode.Name,
			Namespace: testLabInstance.Namespace,
		},
		// Spec: kubevirtv1.VirtualMachineSpec{
		// 	Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Labels: map[string]string{
		// 				"app": testLabInstance.Name + "-" + vmNode.Name + "-remote-access",
		// 			},
		// 		},
		// 	},
		// },
		Status: kubevirtv1.VirtualMachineStatus{
			Ready:           true,
			PrintableStatus: "VM Ready",
		},
	}

	testNodeVM = &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + vmNode.Name + "-2",
			Namespace: testLabInstance.Namespace,
		},
		Status: kubevirtv1.VirtualMachineStatus{
			Ready: false,
		},
	}

	testNodePod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + testNode.Name,
			Namespace: namespace,
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": testLabInstance.Name + "-pod",
			},
			Labels: map[string]string{
				"app": testLabInstance.Name + "-" + testNode.Name + "-remote-access",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	testTtydPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-ttyd-pod",
			Namespace: namespace,
			Labels: map[string]string{
				"app": testLabInstance.Name + "-ttyd-service",
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: testLabInstance.Name + "-ttyd-svcacc",
			Containers: []corev1.Container{
				{
					Name:  testLabInstance.Name + "-ttyd-container",
					Image: "ghcr.io/insrapperswil/kube-ttyd:latest",
					Args:  []string{"ttyd", "-a", "konnect"},
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 7681,
						},
					},
				},
			},
		},
	}

	testTtydService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-ttyd-service",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": testLabInstance.Name + "-ttyd-service",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "ttyd",
					Port:       7681,
					TargetPort: intstr.FromInt(7681),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	testService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + testNode.Name + "-remote-access",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": testLabInstance.Name + "-" + testNode.Name + "-remote-access",
			},
			Ports: []corev1.ServicePort{
				{
					Name:     "test-ssh-port",
					Port:     22,
					Protocol: "TCP",
				},
			},
			Type: corev1.ServiceTypeLoadBalancer,
		},
	}

	testPodIngress = &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + testNode.Name + "-ingress",
			Namespace: namespace,
		},
	}

	testVMIngress = &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + vmNode.Name + "-ingress",
			Namespace: namespace,
		},
	}

	testServiceAccount = &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-ttyd-svcacc",
			Namespace: namespace,
		},
	}

	testRole = &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-ttyd-role",
			Namespace: namespace,
		},
	}

	testRoleBinding = &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-ttyd-rolebind",
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      testLabInstance.Name + "-ttyd-svcacc",
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     testLabInstance.Name + "-ttyd-role",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	// TODO: Need to check if this
	err = ltbv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}
	err = kubevirtv1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}

	err = network.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}
	//expectedReturnValue = ReturnToReconciler{ShouldReturn: false, Result: ctrl.Result{}, Err: nil}

	fakeClient = fake.NewClientBuilder().WithObjects(testLabInstance, testLabTemplate, testNodeTypePod, testNodeTypeVM, testPod).Build()
	r = &LabInstanceReconciler{Client: fakeClient, Scheme: scheme.Scheme}
	field = fields{fakeClient, scheme.Scheme}
}

// func TestGetTemplate(t *testing.T) {
// 	returnValue = r.GetLabTemplate(ctx, testLabInstance, testLabTemplate)
// 	assert.Equal(t, expectedReturnValue, returnValue)
// }

// func TestGetNodeType(t *testing.T) {
// 	returnValue = r.GetNodeType(ctx, &podNode.NodeTypeRef, testNodeTypePod)
// 	assert.Equal(t, expectedReturnValue, returnValue)
// }

// func TestCreatePod(t *testing.T) {
// 	createdPod := CreatePod(testLabInstance, podNode)
// 	assert.Equal(t, testLabInstance.Name+"-"+podNode.Name, createdPod.Name)
// 	assert.Equal(t, testLabInstance.Namespace, createdPod.Namespace)
// 	createdTtydPod := CreatePod(testLabInstance, nil)
// 	assert.Equal(t, testLabInstance.Name+"-ttyd-pod", createdTtydPod.Name)
// }

// func TestMapTemplateToVM(t *testing.T) {
// 	t.Log(vmNode)
// 	mappedVM := MapTemplateToVM(testLabInstance, vmNode)
// 	t.Log(mappedVM)
// 	t.Log(vmNode)
// 	assert.Equal(t, testLabInstance.Name+"-"+vmNode.Name, mappedVM.Name)
// }

// func TestCreateIngress(t *testing.T) {
// Pod ingress
// 	createdPodIngress := CreateIngress(testLabInstance, podNode)
// 	assert.Equal(t, testLabInstance.Name+"-"+podNode.Name+"-ingress", createdPodIngress.Name)
// 	assert.Equal(t, testLabInstance.Namespace, createdPodIngress.Namespace)

// VM ingress
// 	createdVMIngress := CreateIngress(testLabInstance, vmNode)
// 	assert.Equal(t, testLabInstance.Name+"-"+vmNode.Name+"-ingress", createdVMIngress.Name)
// 	assert.Equal(t, testLabInstance.Namespace, createdVMIngress.Namespace)
// }

// func TestCreateService(t *testing.T) {
// Pod service
// 	createdPodService := CreateService(testLabInstance, podNode)
// 	assert.Equal(t, testLabInstance.Name+"-"+podNode.Name+"-remote-access", createdPodService.Name)
// 	assert.Equal(t, testLabInstance.Namespace, createdPodService.Namespace)
// 	assert.Equal(t, "LoadBalancer", string(createdPodService.Spec.Type))
// 	assert.Equal(t, 0, len(createdPodService.Spec.Ports))

// VM service
// 	createdVMService := CreateService(testLabInstance, vmNode)
// 	assert.Equal(t, testLabInstance.Name+"-"+vmNode.Name+"-remote-access", createdVMService.Name)
// 	assert.Equal(t, testLabInstance.Namespace, createdVMService.Namespace)
// 	assert.Equal(t, "LoadBalancer", string(createdVMService.Spec.Type))
// 	assert.Equal(t, 1, len(createdVMService.Spec.Ports))

// TTYD service
// 	createdTTYDService := CreateService(testLabInstance, nil)
// 	assert.Equal(t, testLabInstance.Name+"-ttyd-service", createdTTYDService.Name)
// 	assert.Equal(t, testLabInstance.Namespace, createdTTYDService.Namespace)
// 	assert.Equal(t, "ClusterIP", string(createdTTYDService.Spec.Type))
// 	assert.Equal(t, 1, len(createdTTYDService.Spec.Ports))
// 	assert.Equal(t, 7681, int(createdTTYDService.Spec.Ports[0].Port))
// }

// func TestCreateSvcAccRoleRoleBind(t *testing.T) {
// 	svcAcc, role, roleBind := CreateSvcAccRoleRoleBind(testLabInstance)

// Service Account
// 	assert.Equal(t, testLabInstance.Name+"-ttyd-svcacc", svcAcc.Name)
// 	assert.Equal(t, testLabInstance.Namespace, svcAcc.Namespace)

// Role
// 	assert.Equal(t, testLabInstance.Name+"-ttyd-role", role.Name)
// 	assert.Equal(t, testLabInstance.Namespace, role.Namespace)
// 	assert.NotEqual(t, 0, len(role.Rules))

// Role Binding
// 	assert.Equal(t, testLabInstance.Name+"-ttyd-rolebind", roleBind.Name)
// 	assert.Equal(t, testLabInstance.Namespace, roleBind.Namespace)
// 	assert.Equal(t, testLabInstance.Name+"-ttyd-svcacc", roleBind.Subjects[0].Name)
// 	assert.Equal(t, testLabInstance.Namespace, roleBind.Subjects[0].Namespace)
// 	assert.Equal(t, testLabInstance.Name+"-ttyd-role", roleBind.RoleRef.Name)
// }

// func TestErrorMsg(t *testing.T) {
// 	err := errors.New("Resource not found")
// 	returnValue = ErrorMsg(ctx, err, testLabInstance.Name+"-test-node-3")
// 	assert.Equal(t, true, returnValue.ShouldReturn)
// 	assert.Equal(t, ctrl.Result{}, returnValue.Result)
// 	assert.Equal(t, err, returnValue.Err)
// }

// func TestResourceExists(t *testing.T) {
// 	shouldReturn, err := ResourceExists(r, &corev1.Pod{}, testLabInstance.Name+"-test-node-3", testLabInstance.Namespace)
// 	assert.Equal(t, false, shouldReturn)
// 	assert.NotEqual(t, nil, err)
// }

// func TestCreateResource(t *testing.T) {
// 	createdPod := CreateResource(testLabInstance, podNode, &corev1.Pod{})
// 	assert.Equal(t, testLabInstance.Name+"-"+podNode.Name, createdPod.GetName())
// 	assert.Equal(t, testLabInstance.Namespace, createdPod.GetNamespace())
// }

// func TestReconcileResource(t *testing.T) {
// 	expectedReturnValue = ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{Requeue: true}}
// Non-Existing resource
// 	createdPod, returnValue := ReconcileResource(r, testLabInstance, &corev1.Pod{}, podNode, testLabInstance.Name+"-"+podNode.Name)
// 	assert.Equal(t, expectedReturnValue, returnValue)
// 	assert.Equal(t, testLabInstance.Name+"-"+podNode.Name, createdPod.GetName())
// 	assert.Equal(t, testLabInstance.Namespace, createdPod.GetNamespace())

// Existing resource
// 	createdPod, returnValue = ReconcileResource(r, testLabInstance, &corev1.Pod{}, podNode, testLabInstance.Name+"-"+podNode.Name)
// 	assert.Equal(t, ReturnToReconciler{ShouldReturn: false, Result: ctrl.Result{}, Err: nil}, returnValue)

// Error

// }

// func TestReconcileNetwork(t *testing.T) {
// 	returnValue = r.ReconcileNetwork(ctx, testLabInstance)
// 	t.Log(returnValue)
// 	assert.Equal(t, ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{Requeue: true}}, returnValue)
// }

func TestLabInstanceReconciler_Reconcile(t *testing.T) {
	type args struct {
		ctx context.Context
		req ctrl.Request
	}
	tests := []struct {
		name    string
		args    args
		want    ctrl.Result
		wantErr bool
	}{

		{
			name: "Happy Case with empty request",
			args: args{
				ctx: context.Background(),
				req: ctrl.Request{},
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
		{
			name: "Happy case with namespaced request",
			args: args{
				ctx: context.Background(),
				req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "test", Name: "test"}},
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Reconcile(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("LabInstanceReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLabInstanceReconciler_ReconcileNetwork(t *testing.T) {
	type args struct {
		ctx         context.Context
		labInstance *ltbv1alpha1.LabInstance
	}
	tests := []struct {
		name string
		args args
		want ReturnToReconciler
	}{
		{
			name: "Network will be created",
			args: args{
				ctx:         context.Background(),
				labInstance: testLabInstance,
			},
			want: ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{Requeue: true}, Err: nil},
		},
		// TODO: See how the other test cases can be implemented, but we might have the issue to test the call to the client functions (eg. Get, Create, etc.)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.ReconcileNetwork(tt.args.ctx, tt.args.labInstance)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLabInstanceReconciler_GetLabTemplate(t *testing.T) {
	type args struct {
		ctx         context.Context
		labInstance *ltbv1alpha1.LabInstance
		labTemplate *ltbv1alpha1.LabTemplate
	}
	tests := []struct {
		name string
		args args
		want ReturnToReconciler
	}{
		{
			name: "LabTemplate exists",
			args: args{
				ctx:         context.Background(),
				labInstance: testLabInstance,
				labTemplate: testLabTemplate,
			},
			want: ReturnToReconciler{ShouldReturn: false, Result: ctrl.Result{}, Err: nil},
		},
		{
			name: "LabTemplate can't be nil",
			args: args{
				ctx:         context.Background(),
				labInstance: testLabInstance,
				labTemplate: nil,
			},
			want: ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{}, Err: errors.New("expected pointer, but got nil")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.GetLabTemplate(tt.args.ctx, tt.args.labInstance, tt.args.labTemplate)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLabInstanceReconciler_GetNodeType(t *testing.T) {
	type args struct {
		ctx         context.Context
		nodeTypeRef *ltbv1alpha1.NodeTypeRef
		nodeType    *ltbv1alpha1.NodeType
	}
	tests := []struct {
		name    string
		args    args
		want    ReturnToReconciler
		wantErr bool
	}{
		{
			name: "Node type exists",
			args: args{
				ctx:         context.Background(),
				nodeTypeRef: &podNode.NodeTypeRef,
				nodeType:    testNodeTypePod,
			},
			want:    ReturnToReconciler{ShouldReturn: false, Result: ctrl.Result{}, Err: nil},
			wantErr: false,
		},
		{
			name: "Node type couldn't be retrieved",
			args: args{
				ctx:         context.Background(),
				nodeTypeRef: &testNode.NodeTypeRef,
				nodeType:    testNodeTypePod,
			},
			want:    ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{}, Err: errors.New("Node type doesn't exist")},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.GetNodeType(tt.args.ctx, tt.args.nodeTypeRef, tt.args.nodeType)
			if tt.wantErr {
				assert.Error(t, got.Err)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMapTemplateToPod(t *testing.T) {
	type args struct {
		labInstance *ltbv1alpha1.LabInstance
		node        *ltbv1alpha1.LabInstanceNodes
	}
	tests := []struct {
		name string
		args args
		want *corev1.Pod
	}{
		{
			name: "Pod will be created",
			args: args{
				labInstance: testLabInstance,
				node:        podNode,
			},
			want: testPod,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapTemplateToPod(tt.args.labInstance, tt.args.node)
			assert.Equal(t, tt.want.GetName(), got.GetName())
			assert.Equal(t, tt.want.GetNamespace(), got.GetNamespace())
			assert.Equal(t, tt.want.GetLabels(), got.GetLabels())
			assert.Equal(t, tt.want.GetAnnotations(), got.GetAnnotations())
		})
	}
}

// TODO: this functions needs to be checked
func TestMapTemplateToVM(t *testing.T) {
	type args struct {
		labInstance *ltbv1alpha1.LabInstance
		node        *ltbv1alpha1.LabInstanceNodes
	}
	tests := []struct {
		name string
		args args
		want *kubevirtv1.VirtualMachine
	}{
		{
			name: "VM will be created",
			args: args{
				labInstance: testLabInstance,
				node:        vmNode,
			},
			want: testVM,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapTemplateToVM(tt.args.labInstance, tt.args.node)
			assert.Equal(t, tt.want.GetName(), got.GetName())
		})
	}
}

func TestUpdateLabInstanceStatus(t *testing.T) {
	type args struct {
		ctx         context.Context
		pods        []*corev1.Pod
		vms         []*kubevirtv1.VirtualMachine
		labInstance *ltbv1alpha1.LabInstance
	}
	tests := []struct {
		name          string
		args          args
		runningStatus bool
		isPodRunning  bool
		isVMReady     bool
	}{
		{
			name: "Running status",
			args: args{
				ctx:         context.Background(),
				pods:        []*corev1.Pod{testPod},
				vms:         []*kubevirtv1.VirtualMachine{testVM},
				labInstance: testLabInstance,
			},
			runningStatus: true,
			isPodRunning:  true,
			isVMReady:     true,
		},
		{
			name: "Pending status",
			args: args{
				ctx:         context.Background(),
				pods:        []*corev1.Pod{testPod, testNodePod},
				vms:         []*kubevirtv1.VirtualMachine{testVM},
				labInstance: testLabInstance,
			},
			runningStatus: false,
			isPodRunning:  false,
			isVMReady:     true,
		},
		{
			name: "Not Ready status",
			args: args{
				ctx:         context.Background(),
				pods:        []*corev1.Pod{testPod},
				vms:         []*kubevirtv1.VirtualMachine{testVM, testNodeVM},
				labInstance: testLabInstance,
			},
			runningStatus: false,
			isPodRunning:  true,
			isVMReady:     false,
		},
		{
			name: "Not Running status",
			args: args{
				ctx:         context.Background(),
				pods:        []*corev1.Pod{testPod, testNodePod},
				vms:         []*kubevirtv1.VirtualMachine{testVM, testNodeVM},
				labInstance: testLabInstance,
			},
			runningStatus: false,
			isPodRunning:  false,
			isVMReady:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			UpdateLabInstanceStatus(tt.args.ctx, tt.args.pods, tt.args.vms, tt.args.labInstance)
			t.Log(tt.args.labInstance.Status)
			if tt.runningStatus && tt.isPodRunning && tt.isVMReady {
				assert.Equal(t, "Running", tt.args.labInstance.Status.Status)
				assert.Equal(t, "1/1", tt.args.labInstance.Status.NumPodsRunning)
				assert.Equal(t, "1/1", tt.args.labInstance.Status.NumVMsRunning)
			} else if !tt.runningStatus && !tt.isPodRunning && tt.isVMReady {
				assert.NotEqual(t, "Running", tt.args.labInstance.Status.Status)
				assert.Equal(t, "1/2", tt.args.labInstance.Status.NumPodsRunning)
				assert.Equal(t, "1/1", tt.args.labInstance.Status.NumVMsRunning)
			} else if !tt.runningStatus && tt.isPodRunning && !tt.isVMReady {
				assert.NotEqual(t, "Running", tt.args.labInstance.Status.Status)
				assert.Equal(t, "1/1", tt.args.labInstance.Status.NumPodsRunning)
				assert.Equal(t, "1/2", tt.args.labInstance.Status.NumVMsRunning)
			} else {
				assert.NotEqual(t, "Running", tt.args.labInstance.Status.Status)
				assert.Equal(t, "1/2", tt.args.labInstance.Status.NumPodsRunning)
				assert.Equal(t, "1/2", tt.args.labInstance.Status.NumVMsRunning)
			}
		})
	}
}

func TestLabInstanceReconciler_SetupWithManager(t *testing.T) {
	type args struct {
		mgr ctrl.Manager
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "Error case",
			args:    args{mgr: nil},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := r.SetupWithManager(tt.args.mgr); (err != nil) != tt.wantErr {
				t.Errorf("LabInstanceReconciler.SetupWithManager() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReconcileResource(t *testing.T) {
	type args struct {
		r            *LabInstanceReconciler
		labInstance  *ltbv1alpha1.LabInstance
		resource     client.Object
		node         *ltbv1alpha1.LabInstanceNodes
		resourceName string
	}
	tests := []struct {
		name  string
		args  args
		want  client.Object
		want1 ReturnToReconciler
	}{
		{
			name: "Pod already exists",
			args: args{
				r:            r,
				labInstance:  testLabInstance,
				resource:     &corev1.Pod{},
				node:         podNode,
				resourceName: testLabInstance.Name + "-" + podNode.Name,
			},
			want:  testPod,
			want1: ReturnToReconciler{ShouldReturn: false, Result: ctrl.Result{}, Err: nil},
		},
		{
			name: "Pod will be created",
			args: args{
				r:            r,
				labInstance:  testLabInstance,
				resource:     &corev1.Pod{},
				node:         testNode,
				resourceName: testLabInstance.Name + "-" + testNode.Name,
			},
			want:  testNodePod,
			want1: ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{Requeue: true}, Err: nil},
		},

		{
			name: "Ttyd Service will be created",
			args: args{
				r:            r,
				labInstance:  testLabInstance,
				resource:     &corev1.Service{},
				node:         nil,
				resourceName: testLabInstance.Name + "-ttyd-service",
			},
			want:  testTtydService,
			want1: ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{Requeue: true}, Err: nil},
		},
		{
			name: "Ingress will be created for a vm",
			args: args{
				r:            r,
				labInstance:  testLabInstance,
				resource:     &networkingv1.Ingress{},
				node:         vmNode,
				resourceName: testLabInstance.Name + "-" + vmNode.Name + "-ingress",
			},
			want:  testVMIngress,
			want1: ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{Requeue: true}, Err: nil},
		},
		{
			name: "Service Account will be created",
			args: args{
				r:            r,
				labInstance:  testLabInstance,
				resource:     &corev1.ServiceAccount{},
				node:         nil,
				resourceName: testLabInstance.Name + "-ttyd-svcacc",
			},
			want:  testServiceAccount,
			want1: ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{Requeue: true}, Err: nil},
		},
		{
			name: "Role will be created",
			args: args{
				r:            r,
				labInstance:  testLabInstance,
				resource:     &rbacv1.Role{},
				node:         nil,
				resourceName: testLabInstance.Name + "-role",
			},
			want:  testRole,
			want1: ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{Requeue: true}, Err: nil},
		},
		{
			name: "Rolebinding will be created",
			args: args{
				r:            r,
				labInstance:  testLabInstance,
				resource:     &rbacv1.RoleBinding{},
				node:         nil,
				resourceName: testLabInstance.Name + "-rolebind",
			},
			want:  testRoleBinding,
			want1: ReturnToReconciler{ShouldReturn: true, Result: ctrl.Result{Requeue: true}, Err: nil},
		},

		// TODO: There are other two cases to test:
		// 1.resource already exists, but could not be retrieved
		// 2.resource does not exist, but could not be created
		// Those are the calls to the reconciler functions that return an error (e.g. r.Get(), r.Create())
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := ReconcileResource(tt.args.r, tt.args.labInstance, tt.args.resource, tt.args.node, tt.args.resourceName)
			assert.Equal(t, tt.want.GetName(), got.GetName())
			assert.Equal(t, tt.want.GetNamespace(), got.GetNamespace())
			assert.Equal(t, tt.want.GetLabels(), got.GetLabels())
			assert.Equal(t, tt.want1, got1)
		})
	}
}

func TestCreateResource(t *testing.T) {
	type args struct {
		labInstance *ltbv1alpha1.LabInstance
		node        *ltbv1alpha1.LabInstanceNodes
		resource    client.Object
	}
	tests := []struct {
		name string
		args args
		want client.Object
	}{
		{
			name: "Resource not supported",
			args: args{
				labInstance: testLabInstance,
				node:        nil,
				resource:    &corev1.Secret{},
			},
			want: nil,
		},

		// The other tests are covered by the ReconcileResource tests
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CreateResource(tt.args.labInstance, tt.args.node, tt.args.resource); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateResource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResourceExists(t *testing.T) {
	type args struct {
		r            *LabInstanceReconciler
		resource     client.Object
		resourceName string
		nameSpace    string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "Pod exists",
			args: args{
				r:            r,
				resource:     &corev1.Pod{},
				resourceName: testLabInstance.Name + "-" + testNode.Name,
				nameSpace:    namespace,
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Pod does not exist",
			args: args{
				r:            r,
				resource:     &corev1.Pod{},
				resourceName: testLabInstance.Name + "-" + testNode.Name + "-1",
				nameSpace:    namespace,
			},
			want:    false,
			wantErr: true,
		},
		// TODO: There is another case to test:
		// resource already exists, but could not be retrieved
		// test the call to r.Get()
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResourceExists(tt.args.r, tt.args.resource, tt.args.resourceName, tt.args.nameSpace)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResourceExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ResourceExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateIngress(t *testing.T) {
	type args struct {
		labInstance *ltbv1alpha1.LabInstance
		node        *ltbv1alpha1.LabInstanceNodes
	}
	tests := []struct {
		name string
		args args
		want *networkingv1.Ingress
	}{
		{
			name: "Ingress will be created",
			args: args{
				labInstance: testLabInstance,
				node:        testNode,
			},
			want: testPodIngress,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CreateIngress(tt.args.labInstance, tt.args.node)
			assert.Equal(t, tt.want.GetName(), got.GetName())
			assert.Equal(t, tt.want.GetNamespace(), got.GetNamespace())
			assert.Equal(t, tt.want.GetLabels(), got.GetLabels())
		})
	}
}

func TestCreatePod(t *testing.T) {
	type args struct {
		labInstance *ltbv1alpha1.LabInstance
		node        *ltbv1alpha1.LabInstanceNodes
	}
	tests := []struct {
		name string
		args args
		want *corev1.Pod
	}{
		{
			name: "Ttyd Pod will be created",
			args: args{
				labInstance: testLabInstance,
				node:        nil,
			},
			want: testTtydPod,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CreatePod(tt.args.labInstance, tt.args.node)
			assert.Equal(t, tt.want.GetName(), got.GetName())
			assert.Equal(t, tt.want.GetNamespace(), got.GetNamespace())
			assert.Equal(t, tt.want.GetLabels(), got.GetLabels())
		})
	}
}

func TestCreateService(t *testing.T) {
	type args struct {
		labInstance *ltbv1alpha1.LabInstance
		node        *ltbv1alpha1.LabInstanceNodes
	}
	tests := []struct {
		name string
		args args
		want *corev1.Service
	}{
		{
			name: "Service will be created",
			args: args{

				labInstance: testLabInstance,
				node:        testNode,
			},
			want: testService,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CreateService(tt.args.labInstance, tt.args.node)
			assert.Equal(t, tt.want.GetName(), got.GetName())
			assert.Equal(t, tt.want.GetNamespace(), got.GetNamespace())
			assert.Equal(t, tt.want.GetLabels(), got.GetLabels())
		})
	}
}

func TestErrorMsg(t *testing.T) {
	type args struct {
		ctx      context.Context
		err      error
		resource string
	}
	tests := []struct {
		name string
		args args
		want ReturnToReconciler
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorMsg(tt.args.ctx, tt.args.err, tt.args.resource); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ErrorMsg() = %v, want %v", got, tt.want)
			}
		})
	}
}
