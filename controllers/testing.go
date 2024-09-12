package controllers

import (
	ltbv1alpha1 "github.com/Lab-Topology-Builder/LTB-K8s-Backend/api/v1alpha1"
	network "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const namespace = "test-namespace"

var (
	testLabInstance                                                                                                                                                           *ltbv1alpha1.LabInstance
	testLabTemplateWithoutRenderedNodeSpec, testLabTemplateWithRenderedNodeSpec, testLabTemplateWithoutRenderedNodeSpec2                                                      *ltbv1alpha1.LabTemplate
	testNodeVMType, testPodNodeType, failingVMNodeType, failingPodNodeType, invalidKindNodeType, invalidNodeSpecVMNodeType, invalidNodeSpecPodNodeType, renderInvalidNodeType *ltbv1alpha1.NodeType
	testPodNode, testVMNode, nodeWithUndefinedNodeType, vmNodeYAMLProblem, podNodeYAMLProblem, podRenderSpecProblem                                                           *ltbv1alpha1.LabInstanceNodes
	fakeClient                                                                                                                                                                client.Client
	testPod, testPodUndefinedNode, testTtydPod, testPodRenderSpecProblem                                                                                                      *corev1.Pod
	testVM, testVM2                                                                                                                                                           *kubevirtv1.VirtualMachine
	testPodIngress, testVMIngress                                                                                                                                             *networkingv1.Ingress
	testService, testTtydService                                                                                                                                              *corev1.Service
	testRole                                                                                                                                                                  *rbacv1.Role
	testRoleBinding                                                                                                                                                           *rbacv1.RoleBinding
	testServiceAccount                                                                                                                                                        *corev1.ServiceAccount
	testPodNetworkAttachmentDefinition, testVMNetworkAttachmentDefinition                                                                                                     *network.NetworkAttachmentDefinition
)

func initialize() {

	// _________________________ 1. Test CR NodeTypes __________________________
	// ------------------------- 1.1 Valid NodeTypes ---------------------------
	// ======================= 1.1.1 Valid VM NodeType =========================

	testNodeVMType = &ltbv1alpha1.NodeType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testNodeVM",
			Namespace: "",
		},
		Spec: ltbv1alpha1.NodeTypeSpec{
			Kind: "vm",
			NodeSpec: `
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
`,
		},
	}

	// ======================= 1.1.2 Valid Pod NodeType ========================

	testPodNodeType = &ltbv1alpha1.NodeType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "podNodeType",
			Namespace: "",
		},
		Spec: ltbv1alpha1.NodeTypeSpec{
			Kind: "pod",
			NodeSpec: `
containers:
    - name: {{ .Name }}
      image: {{ .NodeTypeRef.Image}}:{{ .NodeTypeRef.Version }}
      command: ["/bin/bash", "-c", "apt update && apt install -y openssh-server && service ssh start && sleep 365d"]
      ports:
        {{- range $index, $port := .Ports }}
        - name: {{ $port.Name }}
          containerPort: {{ $port.Port }}
          protocol: {{ $port.Protocol }}
        {{- end }}`,
		},
	}

	// ------------------------- 1.2 Invalid NodeTypes -------------------------
	// ======================== 1.2.1 Invalid VM NodeTypes =====================

	failingVMNodeType = &ltbv1alpha1.NodeType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failingVMNodeType",
			Namespace: "",
		},
		Spec: ltbv1alpha1.NodeTypeSpec{
			Kind: "vm",
			NodeSpec: `
containers:
  - name: {{ .Name }}
    image: {{ .NodeTypeRef.Image}}:{{ .NodeTypeRef.Version }}
    command: ["/bin/bash", "-c", "apt update && apt install -y openssh-server && service ssh start && sleep 365d"]
    ports:
        {{- range $index, $port := .Ports }}
      - name: {{ $port.Name }}
        containerPort: {{ $port.Port }}
        protocol: {{ $port.Protocol }}
        {{- end }}`,
		},
	}

	invalidNodeSpecVMNodeType = &ltbv1alpha1.NodeType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidNodeType",
			Namespace: "",
		},
		Spec: ltbv1alpha1.NodeTypeSpec{
			Kind: "vm",
			NodeSpec: `
	containers:
    - name: {{ .Name }}
    image: {{ .NodeTypeRef.Image}}:{{ .NodeTypeRef.Version }}
    command: ["/bin/bash", "-c", "apt update && apt install -y openssh-server && service ssh start && sleep 365d"]
    ports:
    {{- range $index, $port := .Ports }}
    - name: {{ $port.Name }}
    containerPort: {{ $port.Port }}
    protocol: {{ $port.Protocol }}
    {{- end }}`,
		},
	}

	renderInvalidNodeType = &ltbv1alpha1.NodeType{
		ObjectMeta: metav1.ObjectMeta{
			Name: "GenericPodType",
		},
		Spec: ltbv1alpha1.NodeTypeSpec{
			Kind: "pod",
			NodeSpec: `
containers:
- name: {{}{{}} .Name }}
`,
		},
	}

	// ======================== 1.2.2 Invalid Pod NodeTypes ====================

	failingPodNodeType = &ltbv1alpha1.NodeType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failingPodNodeType",
			Namespace: "",
		},
		Spec: ltbv1alpha1.NodeTypeSpec{
			Kind: "pod",
			NodeSpec: `
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
`,
		},
	}

	invalidNodeSpecPodNodeType = &ltbv1alpha1.NodeType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidNodeType",
			Namespace: "",
		},
		Spec: ltbv1alpha1.NodeTypeSpec{
			Kind: "pod",
			NodeSpec: `
	containers:
    - name: {{ .Name }}
    image: {{ .NodeTypeRef.Image}}:{{ .NodeTypeRef.Version }}
    command: ["/bin/bash", "-c", "apt update && apt install -y openssh-server && service ssh start && sleep 365d"]
    ports:
    {{- range $index, $port := .Ports }}
    - name: {{ $port.Name }}
    containerPort: {{ $port.Port }}
    protocol: {{ $port.Protocol }}
    {{- end }}`,
		},
	}

	// ======================== 1.2.3 Undefined NodeTypes ======================

	invalidKindNodeType = &ltbv1alpha1.NodeType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalidNodeType",
			Namespace: "",
		},
		Spec: ltbv1alpha1.NodeTypeSpec{
			Kind:     "test",
			NodeSpec: ``,
		},
	}

	// _______________________________ 2. Test Nodes ___________________________
	// ---------------------------- 2.1 Valid Nodes ----------------------------
	// ========================== 2.1.1 Valid VM Nodes =========================

	testVMNode = &ltbv1alpha1.LabInstanceNodes{
		Name: "test-node-0",
		NodeTypeRef: ltbv1alpha1.NodeTypeRef{
			Type:    testNodeVMType.Name,
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
		RenderedNodeSpec: `
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
`,
	}

	// ========================== 2.1.2 Valid Pod Nodes ========================

	testPodNode = &ltbv1alpha1.LabInstanceNodes{
		Name: "test-node-1",
		NodeTypeRef: ltbv1alpha1.NodeTypeRef{
			Type:    testPodNodeType.Name,
			Image:   "ubuntu",
			Version: "20.04",
		},
		RenderedNodeSpec: `
containers:
  - name: testnode
    image: ubuntu:22.04
    command: ["/bin/bash", "-c", "apt update && apt install -y openssh-server && service ssh start && sleep 365d"]
    ports:
      - name: testsshport
        containerPort: 22
        protocol: tcp
`,
	}

	// ---------------------------- 2.2 Invalid Nodes --------------------------
	// ======================== 2.2.1 Invalid VM Nodes =========================

	vmNodeYAMLProblem = &ltbv1alpha1.LabInstanceNodes{
		Name: "test-node-3",
		NodeTypeRef: ltbv1alpha1.NodeTypeRef{
			Type:    testNodeVMType.Name,
			Image:   "ubuntu",
			Version: "22.04",
		},
		RenderedNodeSpec: `
running true // yaml syntax error
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
          image: quay.io/containerdisks/ubuntu:22.04`,
	}

	// ======================== 2.2.2 Invalid Pod Nodes ========================

	podNodeYAMLProblem = &ltbv1alpha1.LabInstanceNodes{
		Name: "test-node-4",
		NodeTypeRef: ltbv1alpha1.NodeTypeRef{
			Type:    "podNodeType",
			Image:   "ubuntu",
			Version: "20.04",
		},
		RenderedNodeSpec: `
					containers:
					- name: testnode
					  image: ubuntu:22.04
					  command: ["/bin/bash", "-c", "apt update && apt install -y openssh-server && service ssh start && sleep 365d"]
					  ports:
						- name: testsshport
						  containerPort: 22
						  protocol: tcp`,
	}

	podRenderSpecProblem = &ltbv1alpha1.LabInstanceNodes{
		Name: "test-node-5",
		NodeTypeRef: ltbv1alpha1.NodeTypeRef{
			Type:    renderInvalidNodeType.Name,
			Image:   "ubuntu",
			Version: "20.04",
		},
	}

	// ========================= 2.2.3 Undefined Node  =========================

	nodeWithUndefinedNodeType = &ltbv1alpha1.LabInstanceNodes{
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
	}

	// ______________________________ 3. Test CRs ______________________________
	testLabTemplateWithoutRenderedNodeSpec = &ltbv1alpha1.LabTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-labtemplate",
			Namespace: "",
		},
		Spec: ltbv1alpha1.LabTemplateSpec{
			Nodes: []ltbv1alpha1.LabInstanceNodes{
				{
					Name:             testVMNode.Name,
					NodeTypeRef:      testVMNode.NodeTypeRef,
					Ports:            testPodNode.Ports,
					RenderedNodeSpec: "",
				},
				{
					Name:             testPodNode.Name,
					NodeTypeRef:      testPodNode.NodeTypeRef,
					Ports:            testPodNode.Ports,
					RenderedNodeSpec: "",
				},
			},
		},
	}
	testLabTemplateWithoutRenderedNodeSpec2 = &ltbv1alpha1.LabTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-labtemplate",
			Namespace: "",
		},
		Spec: ltbv1alpha1.LabTemplateSpec{
			Nodes: []ltbv1alpha1.LabInstanceNodes{
				{
					Name:             podRenderSpecProblem.Name,
					NodeTypeRef:      podRenderSpecProblem.NodeTypeRef,
					RenderedNodeSpec: "",
				},
			},
		},
	}
	testLabTemplateWithRenderedNodeSpec = &ltbv1alpha1.LabTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-labtemplate",
			Namespace: namespace,
		},
		Spec: ltbv1alpha1.LabTemplateSpec{
			Nodes: []ltbv1alpha1.LabInstanceNodes{
				*testVMNode,
				*testPodNode,
			},
		},
	}

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

	// __________________________ 4. Test K8s Resources ________________________
	// ---------------------------- 4.1 Test Pods ------------------------------

	testPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + testPodNode.Name,
			Namespace: namespace,
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": testLabInstance.Name + "-pod",
			},
			Labels: map[string]string{
				"app": testLabInstance.Name + "-" + testPodNode.Name + "-remote-access",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	testPodUndefinedNode = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + nodeWithUndefinedNodeType.Name,
			Namespace: namespace,
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": testLabInstance.Name + "-pod",
			},
			Labels: map[string]string{
				"app": testLabInstance.Name + "-" + nodeWithUndefinedNodeType.Name + "-remote-access",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	testPodRenderSpecProblem = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + testPodNode.Name,
			Namespace: namespace,
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": testLabInstance.Name + "-pod",
			},
			Labels: map[string]string{
				"app": testLabInstance.Name + "-" + testPodNode.Name + "-remote-access",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	// ---------------------------- 4.2 Test VMs -------------------------------

	testVM = &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + testVMNode.Name,
			Namespace: testLabInstance.Namespace,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": testLabInstance.Name + "-" + testVMNode.Name + "-remote-access",
					},
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Volumes: []kubevirtv1.Volume{
						{
							Name: "cloudinitdisk",
							VolumeSource: kubevirtv1.VolumeSource{
								CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
									UserData: testVMNode.Config,
								},
							},
						},
					},
				},
			},
		},
		Status: kubevirtv1.VirtualMachineStatus{
			Ready:           true,
			PrintableStatus: "VM Ready",
		},
	}

	testVM2 = &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + testVMNode.Name + "-2",
			Namespace: testLabInstance.Namespace,
		},
		Status: kubevirtv1.VirtualMachineStatus{
			Ready:           false,
			PrintableStatus: "Not Ready",
		},
	}

	// ---------------------------- 4.3 Test Service ---------------------------

	testService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-" + testVMNode.Name + "-remote-access",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": testLabInstance.Name + "-" + testVMNode.Name + "-remote-access",
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

	// ------------------ 4.4 Test NetworkAttachmentDefinition -----------------

	testPodNetworkAttachmentDefinition = &network.NetworkAttachmentDefinition{}
	testPodNetworkAttachmentDefinition.Name = testLabInstance.Name + "-pod"
	testPodNetworkAttachmentDefinition.Namespace = testLabInstance.Namespace
	testPodNetworkAttachmentDefinition.Spec.Config = `{
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
	testVMNetworkAttachmentDefinition = &network.NetworkAttachmentDefinition{}
	testVMNetworkAttachmentDefinition.Name = testLabInstance.Name + "-vm"
	testVMNetworkAttachmentDefinition.Namespace = testLabInstance.Namespace
	testVMNetworkAttachmentDefinition.Spec.Config = `{
					"cniVersion": "0.3.1",
					"name": "mynet",
					"type": "bridge",
					"bridge": "mynet0",
					"ipam": {}
				}`

	// ---------------------------- 4.3 Test Ttyd ------------------------------
	// =========================== 4.3.1 Test Ttyd Pod =========================

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

	// ======================== 4.3.1 Test Ttyd Service ========================

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

	// ======================== 4.3.1 Test Ttyd Ingress ========================

	testPodIngress = &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Namespace + "-" + testLabInstance.Name + "-" + testPodNode.Name,
			Namespace: namespace,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/?arg=pod&arg=" + testLabInstance.Namespace + "-" + testLabInstance.Name + "-" + testPodNode.Name + "&arg=bash",
			},
		},
	}

	testVMIngress = &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Namespace + "-" + testLabInstance.Name + "-" + testVMNode.Name,
			Namespace: namespace,
		},
	}

	// ==================== 4.3.1 Test Ttyd Service Account ====================

	testServiceAccount = &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-ttyd-svcacc",
			Namespace: namespace,
		},
	}

	// ======================== 4.3.1 Test Ttyd Role ===========================

	testRole = &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLabInstance.Name + "-ttyd-role",
			Namespace: namespace,
		},
	}

	// ==================== 4.3.1 Test Ttyd Role Binding =======================

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
}
