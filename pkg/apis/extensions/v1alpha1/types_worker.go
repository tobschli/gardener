// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
)

var _ Object = (*Worker)(nil)

// WorkerResource is a constant for the name of the Worker resource.
const WorkerResource = "Worker"

const (
	// ScaleDownUtilizationThresholdAnnotation is the annotation key for the value of NodeGroupAutoscalingOptions.ScaleDownUtilizationThreshold of cluster-autoscaler
	ScaleDownUtilizationThresholdAnnotation = "autoscaler.gardener.cloud/scale-down-utilization-threshold"
	// ScaleDownGpuUtilizationThresholdAnnotation is the annotation key for the value of NodeGroupAutoscalingOptions.ScaleDownGpuUtilizationThreshold of cluster-autoscaler
	ScaleDownGpuUtilizationThresholdAnnotation = "autoscaler.gardener.cloud/scale-down-gpu-utilization-threshold"
	// ScaleDownUnneededTimeAnnotation is the annotation key for the value of NodeGroupAutoscalingOptions.ScaleDownUnneededTime of cluster-autoscaler
	ScaleDownUnneededTimeAnnotation = "autoscaler.gardener.cloud/scale-down-unneeded-time"
	// ScaleDownUnreadyTimeAnnotation is the annotation key for the value of NodeGroupAutoscalingOptions.ScaleDownUnreadyTime of cluster-autoscaler
	ScaleDownUnreadyTimeAnnotation = "autoscaler.gardener.cloud/scale-down-unready-time"
	// MaxNodeProvisionTimeAnnotation is the annotation key for the value of NodeGroupAutoscalingOptions.MaxNodeProvisionTime of cluster-autoscaler
	MaxNodeProvisionTimeAnnotation = "autoscaler.gardener.cloud/max-node-provision-time"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced,path=workers,singular=worker
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name=Type,JSONPath=".spec.type",type=string,description="The type of the cloud provider for this resource."
// +kubebuilder:printcolumn:name=Region,JSONPath=".spec.region",type=string,description="The region into which the worker should be deployed."
// +kubebuilder:printcolumn:name=Status,JSONPath=".status.lastOperation.state",type=string,description="Status of the worker."
// +kubebuilder:printcolumn:name=Age,JSONPath=".metadata.creationTimestamp",type=date,description="creation timestamp"

// Worker is a specification for a Worker resource.
type Worker struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the Worker.
	// If the object's deletion timestamp is set, this field is immutable.
	Spec WorkerSpec `json:"spec"`
	// +optional
	Status WorkerStatus `json:"status"`
}

// GetExtensionSpec implements Object.
func (i *Worker) GetExtensionSpec() Spec {
	return &i.Spec
}

// GetExtensionStatus implements Object.
func (i *Worker) GetExtensionStatus() Status {
	return &i.Status
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WorkerList is a list of Worker resources.
type WorkerList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of Worker.
	Items []Worker `json:"items"`
}

// WorkerSpec is the spec for a Worker resource.
type WorkerSpec struct {
	// DefaultSpec is a structure containing common fields used by all extension resources.
	DefaultSpec `json:",inline"`

	// InfrastructureProviderStatus is a raw extension field that contains the provider status that has
	// been generated by the controller responsible for the `Infrastructure` resource.
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	InfrastructureProviderStatus *runtime.RawExtension `json:"infrastructureProviderStatus,omitempty"`
	// Region is the name of the region where the worker pool should be deployed to. This field is immutable.
	Region string `json:"region"`
	// SecretRef is a reference to a secret that contains the cloud provider specific credentials.
	SecretRef corev1.SecretReference `json:"secretRef"`
	// SSHPublicKey is the public SSH key that should be used with these workers.
	// +optional
	SSHPublicKey []byte `json:"sshPublicKey,omitempty"`
	// Pools is a list of worker pools.
	// +patchMergeKey=name
	// +patchStrategy=merge
	Pools []WorkerPool `json:"pools" patchStrategy:"merge" patchMergeKey:"name"`
}

// WorkerPool is the definition of a specific worker pool.
type WorkerPool struct {
	// MachineType contains information about the machine type that should be used for this worker pool.
	MachineType string `json:"machineType"`
	// Maximum is the maximum size of the worker pool.
	Maximum int32 `json:"maximum"`
	// MaxSurge is maximum number of VMs that are created during an update.
	MaxSurge intstr.IntOrString `json:"maxSurge"`
	// MaxUnavailable is the maximum number of VMs that can be unavailable during an update.
	MaxUnavailable intstr.IntOrString `json:"maxUnavailable"`
	// Annotations is a map of key/value pairs for annotations for all the `Node` objects in this worker pool.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// Labels is a map of key/value pairs for labels for all the `Node` objects in this worker pool.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Taints is a list of taints for all the `Node` objects in this worker pool.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`
	// MachineImage contains logical information about the name and the version of the machie image that
	// should be used. The logical information must be mapped to the provider-specific information (e.g.,
	// AMIs, ...) by the provider itself.
	MachineImage MachineImage `json:"machineImage,omitempty"`
	// Minimum is the minimum size of the worker pool.
	Minimum int32 `json:"minimum"`
	// Name is the name of this worker pool.
	Name string `json:"name"`
	// NodeAgentSecretName is uniquely identifying selected aspects of the OperatingSystemConfig. If it changes, then the
	// worker pool must be rolled.
	// +optional
	NodeAgentSecretName *string `json:"nodeAgentSecretName,omitempty"`
	// ProviderConfig is a provider specific configuration for the worker pool.
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	ProviderConfig *runtime.RawExtension `json:"providerConfig,omitempty"`
	// UserDataSecretRef references a Secret and a data key containing the data that is sent to the provider's APIs when
	// a new machine/VM that is part of this worker pool shall be spawned.
	UserDataSecretRef corev1.SecretKeySelector `json:"userDataSecretRef"`
	// Volume contains information about the root disks that should be used for this worker pool.
	// +optional
	Volume *Volume `json:"volume,omitempty"`
	// DataVolumes contains a list of additional worker volumes.
	// +optional
	DataVolumes []DataVolume `json:"dataVolumes,omitempty"`
	// KubeletDataVolumeName contains the name of a dataVolume that should be used for storing kubelet state.
	// +optional
	KubeletDataVolumeName *string `json:"kubeletDataVolumeName,omitempty"`
	// Zones contains information about availability zones for this worker pool.
	// +optional
	Zones []string `json:"zones,omitempty"`
	// MachineControllerManagerSettings contains configurations for different worker-pools. Eg. MachineDrainTimeout, MachineHealthTimeout.
	// +optional
	MachineControllerManagerSettings *gardencorev1beta1.MachineControllerManagerSettings `json:"machineControllerManager,omitempty"`
	// KubernetesVersion is the kubernetes version in this worker pool
	// +optional
	KubernetesVersion *string `json:"kubernetesVersion,omitempty"`
	// NodeTemplate contains resource information of the machine which is used by Cluster Autoscaler to generate nodeTemplate during scaling a nodeGroup from zero
	// +optional
	NodeTemplate *NodeTemplate `json:"nodeTemplate,omitempty"`
	// Architecture is the CPU architecture of the worker pool machines and machine image.
	// +optional
	Architecture *string `json:"architecture,omitempty"`
	// ClusterAutoscaler contains the cluster autoscaler configurations for the worker pool.
	// +optional
	ClusterAutoscaler *ClusterAutoscalerOptions `json:"clusterAutoscaler,omitempty"`
	// Priority (or weight) is the importance by which this worker pool will be scaled by cluster autoscaling.
	// +optional
	Priority *int32 `json:"priority"`
}

// ClusterAutoscalerOptions contains the cluster autoscaler configurations for a worker pool.
type ClusterAutoscalerOptions struct {
	// ScaleDownUtilizationThreshold defines the threshold in fraction (0.0 - 1.0) under which a node is being removed.
	// +optional
	ScaleDownUtilizationThreshold *string `json:"scaleDownUtilizationThreshold,omitempty"`
	// ScaleDownGpuUtilizationThreshold defines the threshold in fraction (0.0 - 1.0) of gpu resources under which a node is being removed.
	// +optional
	ScaleDownGpuUtilizationThreshold *string `json:"scaleDownGpuUtilizationThreshold,omitempty"`
	// ScaleDownUnneededTime defines how long a node should be unneeded before it is eligible for scale down.
	// +optional
	ScaleDownUnneededTime *metav1.Duration `json:"scaleDownUnneededTime,omitempty"`
	// ScaleDownUnreadyTime defines how long an unready node should be unneeded before it is eligible for scale down.
	// +optional
	ScaleDownUnreadyTime *metav1.Duration `json:"scaleDownUnreadyTime,omitempty"`
	// MaxNodeProvisionTime defines how long cluster autoscaler should wait for a node to be provisioned.
	// +optional
	MaxNodeProvisionTime *metav1.Duration `json:"maxNodeProvisionTime,omitempty"`
}

// NodeTemplate contains information about the expected node properties.
type NodeTemplate struct {
	// Capacity represents the expected Node capacity.
	Capacity corev1.ResourceList `json:"capacity"`
}

// MachineImage contains logical information about the name and the version of the machie image that
// should be used. The logical information must be mapped to the provider-specific information (e.g.,
// AMIs, ...) by the provider itself.
type MachineImage struct {
	// Name is the logical name of the machine image.
	Name string `json:"name"`
	// Version is the version of the machine image.
	Version string `json:"version"`
}

// Volume contains information about the root disks that should be used for worker pools.
type Volume struct {
	// Name of the volume to make it referenceable.
	// +optional
	Name *string `json:"name,omitempty"`
	// Type is the type of the volume.
	// +optional
	Type *string `json:"type,omitempty"`
	// Size is the of the root volume.
	Size string `json:"size"`
	// Encrypted determines if the volume should be encrypted.
	// +optional
	Encrypted *bool `json:"encrypted,omitempty"`
}

// DataVolume contains information about a data volume.
type DataVolume struct {
	// Name of the volume to make it referenceable.
	Name string `json:"name"`
	// Type is the type of the volume.
	// +optional
	Type *string `json:"type,omitempty"`
	// Size is the of the root volume.
	Size string `json:"size"`
	// Encrypted determines if the volume should be encrypted.
	// +optional
	Encrypted *bool `json:"encrypted,omitempty"`
}

// WorkerStatus is the status for a Worker resource.
type WorkerStatus struct {
	// DefaultStatus is a structure containing common fields used by all extension resources.
	DefaultStatus `json:",inline"`
	// MachineDeployments is a list of created machine deployments. It will be used to e.g. configure
	// the cluster-autoscaler properly.
	// +patchMergeKey=name
	// +patchStrategy=merge
	MachineDeployments []MachineDeployment `json:"machineDeployments,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	// MachineDeploymentsLastUpdateTime is the timestamp when the status.MachineDeployments slice was last updated.
	// +optional
	MachineDeploymentsLastUpdateTime *metav1.Time `json:"machineDeploymentsLastUpdateTime,omitempty"`
}

// MachineDeployment is a created machine deployment.
type MachineDeployment struct {
	// Name is the name of the `MachineDeployment` resource.
	Name string `json:"name"`
	// Minimum is the minimum number for this machine deployment.
	Minimum int32 `json:"minimum"`
	// Maximum is the maximum number for this machine deployment.
	Maximum int32 `json:"maximum"`
	// Priority (or weight) is the importance by which this machine deployment will be scaled by cluster autoscaling.
	Priority int32 `json:"priority"`
}
