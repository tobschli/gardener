// Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package care

import (
	"context"
	"fmt"
	"strings"
	"time"

	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	apiextensions "github.com/gardener/gardener/pkg/api/extensions"
	"github.com/gardener/gardener/pkg/apis/core"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	v1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/extensions"
	"github.com/gardener/gardener/pkg/features"
	gardenletconfig "github.com/gardener/gardener/pkg/gardenlet/apis/config"
	gardenlethelper "github.com/gardener/gardener/pkg/gardenlet/apis/config/helper"
	"github.com/gardener/gardener/pkg/operation/botanist"
	"github.com/gardener/gardener/pkg/operation/shoot"
	"github.com/gardener/gardener/pkg/utils/flow"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/kubernetes/health"
	healthchecker "github.com/gardener/gardener/pkg/utils/kubernetes/health/checker"
)

var (
	requiredControlPlaneEtcds = sets.New(
		v1beta1constants.ETCDMain,
		v1beta1constants.ETCDEvents,
	)

	commonControlPlaneDeployments = sets.New(
		v1beta1constants.DeploymentNameGardenerResourceManager,
		v1beta1constants.DeploymentNameKubeAPIServer,
		v1beta1constants.DeploymentNameKubeControllerManager,
	)

	commonMonitoringDeployments = sets.New(
		v1beta1constants.DeploymentNameKubeStateMetrics,
		v1beta1constants.DeploymentNamePlutono,
	)
)

// Health contains information needed to execute shoot health checks.
type Health struct {
	shoot        *shoot.Shoot
	gardenClient client.Client
	seedClient   kubernetes.Interface

	initializeShootClients ShootClientInit

	log logr.Logger

	gardenletConfiguration                    *gardenletconfig.GardenletConfiguration
	clock                                     clock.Clock
	controllerRegistrationToLastHeartbeatTime map[string]*metav1.MicroTime
	conditionThresholds                       map[gardencorev1beta1.ConditionType]time.Duration
	healthChecker                             *healthchecker.HealthChecker
}

// ShootClientInit is a function that initializes a kubernetes client for a Shoot.
type ShootClientInit func() (kubernetes.Interface, bool, error)

// NewHealth creates a new Health instance with the given parameters.
func NewHealth(
	log logr.Logger,
	shoot *shoot.Shoot,
	seedClientSet kubernetes.Interface,
	gardenClient client.Client,
	shootClientInit ShootClientInit,
	clock clock.Clock,
	gardenletConfig *gardenletconfig.GardenletConfiguration,
	conditionThresholds map[gardencorev1beta1.ConditionType]time.Duration,
) *Health {
	return &Health{
		shoot:                  shoot,
		gardenClient:           gardenClient,
		seedClient:             seedClientSet,
		initializeShootClients: shootClientInit,
		clock:                  clock,
		log:                    log,
		gardenletConfiguration: gardenletConfig,
		controllerRegistrationToLastHeartbeatTime: map[string]*metav1.MicroTime{},
		conditionThresholds:                       conditionThresholds,
		healthChecker:                             healthchecker.NewHealthChecker(seedClientSet.Client(), clock, conditionThresholds, shoot.GetInfo().Status.LastOperation),
	}
}

// Check conducts the health checks on all the given conditions.
func (h *Health) Check(
	ctx context.Context,
	healthCheckOutdatedThreshold *metav1.Duration,
	conditions ShootConditions,
) []gardencorev1beta1.Condition {
	var (
		lastOp     = h.shoot.GetInfo().Status.LastOperation
		lastErrors = h.shoot.GetInfo().Status.LastErrors
	)

	if h.shoot.HibernationEnabled || h.shoot.GetInfo().Status.IsHibernated {
		updatedConditions := shootHibernatedConditions(h.clock, conditions.ConvertToSlice())
		return PardonConditions(h.clock, updatedConditions, lastOp, lastErrors)
	}

	// Get extensions' conditions that are examined by health checks.
	extensionConditionsControlPlaneHealthy, extensionConditionsEveryNodeReady, extensionConditionsSystemComponentsHealthy, err := h.getAllExtensionConditions(ctx)
	if err != nil {
		h.log.Error(err, "Error getting extension conditions")
	}

	// Health checks that can be executed in all cases.
	taskFns := []flow.TaskFn{
		func(ctx context.Context) error {
			newControlPlane, err := h.checkControlPlane(ctx, conditions.controlPlaneHealthy, extensionConditionsControlPlaneHealthy, healthCheckOutdatedThreshold)
			conditions.controlPlaneHealthy = v1beta1helper.NewConditionOrError(h.clock, conditions.controlPlaneHealthy, newControlPlane, err)
			return nil
		}, func(ctx context.Context) error {
			newObservabilityComponents, err := h.checkObservabilityComponents(ctx, conditions.observabilityComponentsHealthy)
			conditions.observabilityComponentsHealthy = v1beta1helper.NewConditionOrError(h.clock, conditions.observabilityComponentsHealthy, newObservabilityComponents, err)
			return nil
		},
	}

	// Health checks with dependencies to the Kube-Apiserver.
	shootClient, apiServerRunning, err := h.initializeShootClients()
	if apiServerRunning && err == nil {
		taskFns = append(taskFns,
			func(ctx context.Context) error {
				conditions.apiServerAvailable = h.checkAPIServerAvailability(ctx, shootClient.RESTClient(), conditions.apiServerAvailable)
				return nil
			},
			func(ctx context.Context) error {
				newSystemComponents, err := h.checkSystemComponents(ctx, shootClient, conditions.systemComponentsHealthy, extensionConditionsSystemComponentsHealthy, healthCheckOutdatedThreshold)
				conditions.systemComponentsHealthy = v1beta1helper.NewConditionOrError(h.clock, conditions.systemComponentsHealthy, newSystemComponents, err)
				return nil
			},
		)
		if conditions.everyNodeReady != nil {
			taskFns = append(taskFns,
				func(ctx context.Context) error {
					newNodes, err := h.checkWorkers(ctx, shootClient, *conditions.everyNodeReady, extensionConditionsEveryNodeReady, healthCheckOutdatedThreshold)
					nodeCondition := v1beta1helper.NewConditionOrError(h.clock, *conditions.everyNodeReady, newNodes, err)
					conditions.everyNodeReady = &nodeCondition
					return nil
				})
		}
	} else {
		// Some health checks cannot be executed when the API server is not running.
		// Maintain the affected conditions here.
		message := shootControlPlaneNotRunningMessage(h.shoot.GetInfo().Status.LastOperation)
		if err != nil {
			h.log.Error(err, "Could not initialize Shoot client for health check")
			message = fmt.Sprintf("Could not initialize Shoot client for health check: %+v", err)
		}

		conditions.apiServerAvailable = v1beta1helper.FailedCondition(h.clock, h.shoot.GetInfo().Status.LastOperation, h.conditionThresholds, conditions.apiServerAvailable, "APIServerDown", "Could not reach API server during client initialization.")
		conditions.systemComponentsHealthy = v1beta1helper.UpdatedConditionUnknownErrorMessageWithClock(h.clock, conditions.systemComponentsHealthy, message)
		if conditions.everyNodeReady != nil {
			nodeCondition := v1beta1helper.UpdatedConditionUnknownErrorMessageWithClock(h.clock, *conditions.everyNodeReady, message)
			conditions.everyNodeReady = &nodeCondition
		}
	}

	// Execute all relevant health checks.
	_ = flow.Parallel(taskFns...)(ctx)

	return PardonConditions(h.clock, conditions.ConvertToSlice(), lastOp, lastErrors)
}

func (h *Health) getAllExtensionConditions(ctx context.Context) ([]healthchecker.ExtensionCondition, []healthchecker.ExtensionCondition, []healthchecker.ExtensionCondition, error) {
	objs, err := h.retrieveExtensions(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	controllerInstallations := &gardencorev1beta1.ControllerInstallationList{}
	if err := h.gardenClient.List(ctx, controllerInstallations, client.MatchingFields{core.SeedRefName: h.gardenletConfiguration.SeedConfig.Name}); err != nil {
		return nil, nil, nil, err
	}

	controllerRegistrations := &gardencorev1beta1.ControllerRegistrationList{}
	if err := h.gardenClient.List(ctx, controllerRegistrations); err != nil {
		return nil, nil, nil, err
	}

	var (
		conditionsControlPlaneHealthy     []healthchecker.ExtensionCondition
		conditionsEveryNodeReady          []healthchecker.ExtensionCondition
		conditionsSystemComponentsHealthy []healthchecker.ExtensionCondition
	)

	for _, obj := range objs {
		acc, err := apiextensions.Accessor(obj)
		if err != nil {
			return nil, nil, nil, err
		}

		gvk, err := apiutil.GVKForObject(obj, kubernetes.SeedScheme)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to identify GVK for object: %w", err)
		}

		kind := gvk.Kind
		name := acc.GetName()
		extensionType := acc.GetExtensionSpec().GetExtensionType()
		namespace := acc.GetNamespace()

		lastHeartbeatTime, err := h.getLastHeartbeatTimeForExtension(ctx, controllerInstallations, controllerRegistrations, kind, extensionType)
		if err != nil {
			return nil, nil, nil, err
		}

		for _, condition := range acc.GetExtensionStatus().GetConditions() {
			switch condition.Type {
			case gardencorev1beta1.ShootControlPlaneHealthy:
				conditionsControlPlaneHealthy = append(conditionsControlPlaneHealthy, healthchecker.ExtensionCondition{
					Condition:          condition,
					ExtensionType:      kind,
					ExtensionName:      name,
					ExtensionNamespace: namespace,
					LastHeartbeatTime:  lastHeartbeatTime,
				})
			case gardencorev1beta1.ShootEveryNodeReady:
				conditionsEveryNodeReady = append(conditionsEveryNodeReady, healthchecker.ExtensionCondition{
					Condition:          condition,
					ExtensionType:      kind,
					ExtensionName:      name,
					ExtensionNamespace: namespace,
					LastHeartbeatTime:  lastHeartbeatTime,
				})
			case gardencorev1beta1.ShootSystemComponentsHealthy:
				conditionsSystemComponentsHealthy = append(conditionsSystemComponentsHealthy, healthchecker.ExtensionCondition{
					Condition:          condition,
					ExtensionType:      kind,
					ExtensionName:      name,
					ExtensionNamespace: namespace,
					LastHeartbeatTime:  lastHeartbeatTime,
				})
			}
		}
	}

	return conditionsControlPlaneHealthy, conditionsEveryNodeReady, conditionsSystemComponentsHealthy, nil
}

func (h *Health) retrieveExtensions(ctx context.Context) ([]runtime.Object, error) {
	var (
		allExtensions       []runtime.Object
		extensionObjectList = []client.ObjectList{
			&extensionsv1alpha1.ExtensionList{},
		}
	)

	if !h.shoot.IsWorkerless {
		extensionObjectList = append(extensionObjectList,
			&extensionsv1alpha1.ContainerRuntimeList{},
			&extensionsv1alpha1.ControlPlaneList{},
			&extensionsv1alpha1.InfrastructureList{},
			&extensionsv1alpha1.NetworkList{},
			&extensionsv1alpha1.OperatingSystemConfigList{},
			&extensionsv1alpha1.WorkerList{},
		)
	}

	for _, listObj := range extensionObjectList {
		if err := h.seedClient.Client().List(ctx, listObj, client.InNamespace(h.shoot.SeedNamespace)); err != nil {
			return nil, err
		}

		if err := meta.EachListItem(listObj, func(obj runtime.Object) error {
			allExtensions = append(allExtensions, obj)
			return nil
		}); err != nil {
			return nil, fmt.Errorf("error during evaluation of kind %T for extensions health check: %w", listObj, err)
		}
	}

	// Get BackupEntries separately as they are not namespaced i.e., they cannot be narrowed down
	// to a shoot namespace like other extension resources above.
	be := &extensionsv1alpha1.BackupEntry{}
	if err := h.seedClient.Client().Get(ctx, kubernetesutils.Key(h.shoot.BackupEntryName), be); err == nil {
		allExtensions = append(allExtensions, be)
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}

	return allExtensions, nil
}

func (h *Health) getLastHeartbeatTimeForExtension(ctx context.Context, controllerInstallations *gardencorev1beta1.ControllerInstallationList, controllerRegistrations *gardencorev1beta1.ControllerRegistrationList, extensionKind, extensionType string) (*metav1.MicroTime, error) {
	controllerRegistration, err := getControllerRegistrationForExtensionKindAndType(controllerRegistrations, extensionKind, extensionType)
	if err != nil {
		return nil, err
	}

	if lastHeartbeatTime, exists := h.controllerRegistrationToLastHeartbeatTime[controllerRegistration.Name]; exists {
		return lastHeartbeatTime, nil
	}

	controllerInstallation, err := getControllerInstallationForControllerRegistration(controllerInstallations, controllerRegistration)
	if err != nil {
		return nil, err
	}

	heartBeatLease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      extensions.HeartBeatResourceName,
			Namespace: gardenerutils.NamespaceNameForControllerInstallation(controllerInstallation),
		},
	}

	if err := h.seedClient.Client().Get(ctx, client.ObjectKeyFromObject(heartBeatLease), heartBeatLease); err != nil {
		if apierrors.IsNotFound(err) {
			h.controllerRegistrationToLastHeartbeatTime[controllerRegistration.Name] = nil
			return nil, nil
		}
		return nil, err
	}

	h.controllerRegistrationToLastHeartbeatTime[controllerRegistration.Name] = heartBeatLease.Spec.RenewTime
	return heartBeatLease.Spec.RenewTime, nil
}

func getControllerRegistrationForExtensionKindAndType(controllerRegistrations *gardencorev1beta1.ControllerRegistrationList, extensionKind, extensionType string) (*gardencorev1beta1.ControllerRegistration, error) {
	for _, controllerRegistration := range controllerRegistrations.Items {
		for _, resource := range controllerRegistration.Spec.Resources {
			if resource.Kind == extensionKind && resource.Type == extensionType {
				return &controllerRegistration, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find ControllerRegistration for extension kind %s and type %s", extensionKind, extensionType)
}

func getControllerInstallationForControllerRegistration(controllerInstallations *gardencorev1beta1.ControllerInstallationList, controllerRegistration *gardencorev1beta1.ControllerRegistration) (*gardencorev1beta1.ControllerInstallation, error) {
	for _, controllerInstallation := range controllerInstallations.Items {
		if controllerInstallation.Spec.RegistrationRef.Name == controllerRegistration.Name {
			return &controllerInstallation, nil
		}
	}
	return nil, fmt.Errorf("could not find ControllerInstallation for ControllerRegistration %s", client.ObjectKeyFromObject(controllerRegistration))
}

// checkAPIServerAvailability checks if the API server of a Shoot cluster is reachable and measure the response time.
func (h *Health) checkAPIServerAvailability(ctx context.Context, shootRestClient rest.Interface, condition gardencorev1beta1.Condition) gardencorev1beta1.Condition {
	return health.CheckAPIServerAvailability(ctx, h.clock, h.log, condition, shootRestClient, func(conditionType, message string) gardencorev1beta1.Condition {
		return v1beta1helper.FailedCondition(h.clock, h.shoot.GetInfo().Status.LastOperation, h.conditionThresholds, condition, conditionType, message)
	})
}

// checkControlPlane checks whether the core components of the Shoot controlplane (ETCD, KAPI, KCM..) are healthy.
func (h *Health) checkControlPlane(
	ctx context.Context,
	condition gardencorev1beta1.Condition,
	extensionConditions []healthchecker.ExtensionCondition,
	healthCheckOutdatedThreshold *metav1.Duration,
) (*gardencorev1beta1.Condition, error) {
	requiredControlPlaneDeployments, err := ComputeRequiredControlPlaneDeployments(h.shoot.GetInfo())
	if err != nil {
		return nil, err
	}

	if exitCondition, err := h.healthChecker.CheckControlPlane(ctx, h.shoot.SeedNamespace, requiredControlPlaneDeployments, requiredControlPlaneEtcds, condition); err != nil || exitCondition != nil {
		return exitCondition, err
	}

	if exitCondition := h.healthChecker.CheckExtensionCondition(condition, extensionConditions, healthCheckOutdatedThreshold); exitCondition != nil {
		return exitCondition, nil
	}

	c := v1beta1helper.UpdatedConditionWithClock(h.clock, condition, gardencorev1beta1.ConditionTrue, "ControlPlaneRunning", "All control plane components are healthy.")
	return &c, nil
}

var monitoringSelector = labels.SelectorFromSet(map[string]string{v1beta1constants.GardenRole: v1beta1constants.GardenRoleMonitoring})

// checkObservabilityComponents checks whether the  observability components of the Shoot control plane (Prometheus, Vali, Plutono..) are healthy.
func (h *Health) checkObservabilityComponents(
	ctx context.Context,
	condition gardencorev1beta1.Condition,
) (*gardencorev1beta1.Condition, error) {
	if h.shoot.Purpose != gardencorev1beta1.ShootPurposeTesting && gardenlethelper.IsMonitoringEnabled(h.gardenletConfiguration) {
		if exitCondition, err := h.healthChecker.CheckMonitoringControlPlane(
			ctx,
			h.shoot.SeedNamespace,
			ComputeRequiredMonitoringSeedDeployments(h.shoot.GetInfo()),
			ComputeRequiredMonitoringStatefulSets(h.shoot.WantsAlertmanager),
			monitoringSelector,
			condition,
		); err != nil || exitCondition != nil {
			return exitCondition, err
		}
	}

	if h.shoot.Purpose != gardencorev1beta1.ShootPurposeTesting && gardenlethelper.IsLoggingEnabled(h.gardenletConfiguration) {
		if exitCondition, err := h.healthChecker.CheckLoggingControlPlane(
			ctx,
			h.shoot.SeedNamespace,
			gardenlethelper.IsLoggingEnabled(h.gardenletConfiguration),
			gardenlethelper.IsValiEnabled(h.gardenletConfiguration),
			condition,
		); err != nil || exitCondition != nil {
			return exitCondition, err
		}
	}

	c := v1beta1helper.UpdatedConditionWithClock(h.clock, condition, gardencorev1beta1.ConditionTrue, "ObservabilityComponentsRunning", "All observability components are healthy.")
	return &c, nil
}

// checkSystemComponents checks whether the system components of a Shoot are running.
func (h *Health) checkSystemComponents(
	ctx context.Context,
	shootClient kubernetes.Interface,
	condition gardencorev1beta1.Condition,
	extensionConditions []healthchecker.ExtensionCondition,
	healthCheckOutdatedThreshold *metav1.Duration,
) (
	*gardencorev1beta1.Condition,
	error,
) {
	mrList := &resourcesv1alpha1.ManagedResourceList{}
	if err := h.seedClient.Client().List(ctx, mrList, client.InNamespace(h.shoot.SeedNamespace)); err != nil {
		return nil, err
	}

	for _, mr := range mrList.Items {
		if mr.Spec.Class != nil {
			continue
		}

		if exitCondition := h.healthChecker.CheckManagedResource(condition, &mr, gardenlethelper.GetManagedResourceProgressingThreshold(h.gardenletConfiguration)); exitCondition != nil {
			return exitCondition, nil
		}
	}

	if exitCondition := h.healthChecker.CheckExtensionCondition(condition, extensionConditions, healthCheckOutdatedThreshold); exitCondition != nil {
		return exitCondition, nil
	}

	if !h.shoot.IsWorkerless {
		podsList := &corev1.PodList{}
		if err := shootClient.Client().List(ctx, podsList, client.InNamespace(metav1.NamespaceSystem), client.MatchingLabels{"type": "tunnel"}); err != nil {
			return nil, err
		}

		if len(podsList.Items) == 0 {
			c := v1beta1helper.FailedCondition(h.clock, h.shoot.GetInfo().Status.LastOperation, h.conditionThresholds, condition, "NoTunnelDeployed", "no tunnels are currently deployed to perform health-check on")
			return &c, nil
		}

		if established, err := botanist.CheckTunnelConnection(ctx, logr.Discard(), shootClient, v1beta1constants.VPNTunnel); err != nil || !established {
			msg := "Tunnel connection has not been established"
			if err != nil {
				msg += fmt.Sprintf(" (%+v)", err)
			}
			c := v1beta1helper.FailedCondition(h.clock, h.shoot.GetInfo().Status.LastOperation, h.conditionThresholds, condition, "TunnelConnectionBroken", msg)
			return &c, nil
		}
	}

	c := v1beta1helper.UpdatedConditionWithClock(h.clock, condition, gardencorev1beta1.ConditionTrue, "SystemComponentsRunning", "All system components are healthy.")
	return &c, nil
}

// checkWorkers checks whether every node registered at the Shoot cluster is in "Ready" state, that
// as many nodes are registered as desired, and that every machine is running.
func (h *Health) checkWorkers(
	ctx context.Context,
	shootClient kubernetes.Interface,
	condition gardencorev1beta1.Condition,
	extensionConditions []healthchecker.ExtensionCondition,
	healthCheckOutdatedThreshold *metav1.Duration,
) (*gardencorev1beta1.Condition, error) {
	if exitCondition := h.healthChecker.CheckExtensionCondition(condition, extensionConditions, healthCheckOutdatedThreshold); exitCondition != nil {
		return exitCondition, nil
	}
	if exitCondition, err := h.CheckClusterNodes(ctx, shootClient, condition); err != nil || exitCondition != nil {
		return exitCondition, err
	}

	c := v1beta1helper.UpdatedConditionWithClock(h.clock, condition, gardencorev1beta1.ConditionTrue, "EveryNodeReady", "All nodes are ready.")
	return &c, nil
}

// ComputeRequiredMonitoringStatefulSets returns names of monitoring statefulsets based on the given shoot.
func ComputeRequiredMonitoringStatefulSets(wantsAlertmanager bool) sets.Set[string] {
	var requiredMonitoringStatefulSets = sets.New(v1beta1constants.StatefulSetNamePrometheus)
	if wantsAlertmanager {
		requiredMonitoringStatefulSets.Insert(v1beta1constants.StatefulSetNameAlertManager)
	}
	return requiredMonitoringStatefulSets
}

// ComputeRequiredMonitoringSeedDeployments returns names of monitoring deployments based on the given shoot.
func ComputeRequiredMonitoringSeedDeployments(shoot *gardencorev1beta1.Shoot) sets.Set[string] {
	requiredDeployments := commonMonitoringDeployments.Clone()
	if v1beta1helper.IsWorkerless(shoot) {
		requiredDeployments.Delete(v1beta1constants.DeploymentNameKubeStateMetrics)
	}

	return requiredDeployments
}

// annotationKeyNotManagedByMCM is a constant for an annotation on the node resource that indicates that the node is not
// handled by machine-controller-manager.
const annotationKeyNotManagedByMCM = "node.machine.sapcloud.io/not-managed-by-mcm"

// CheckClusterNodes checks whether cluster nodes are healthy and within the desired range.
// Additional checks are executed in the provider extension.
func (h *Health) CheckClusterNodes(
	ctx context.Context,
	shootClient kubernetes.Interface,
	condition gardencorev1beta1.Condition,
) (
	*gardencorev1beta1.Condition,
	error,
) {
	workerPoolToNodes, err := botanist.WorkerPoolToNodesMap(ctx, shootClient.Client())
	if err != nil {
		return nil, err
	}

	roleValue, oscOutdatedReason := v1beta1constants.GardenRoleCloudConfig, "CloudConfigOutdated"
	if features.DefaultFeatureGate.Enabled(features.UseGardenerNodeAgent) {
		oscOutdatedReason = "OperatingSystemConfigOutdated"

		oscSecretsExist, err := kubernetesutils.ResourcesExist(ctx, shootClient.Client(), &corev1.SecretList{}, shootClient.Client().Scheme(), client.InNamespace(metav1.NamespaceSystem), client.MatchingLabels{v1beta1constants.GardenRole: v1beta1constants.GardenRoleOperatingSystemConfig})
		if err != nil {
			return nil, err
		}
		if oscSecretsExist {
			roleValue = v1beta1constants.GardenRoleOperatingSystemConfig
		}
	}

	workerPoolToCloudConfigSecretMeta, err := botanist.WorkerPoolToOperatingSystemConfigSecretMetaMap(ctx, shootClient.Client(), roleValue)
	if err != nil {
		return nil, err
	}

	for _, pool := range h.shoot.GetInfo().Spec.Provider.Workers {
		nodes := workerPoolToNodes[pool.Name]

		kubernetesVersion, err := v1beta1helper.CalculateEffectiveKubernetesVersion(h.shoot.KubernetesVersion, pool.Kubernetes)
		if err != nil {
			return nil, err
		}

		if exitCondition := h.healthChecker.CheckNodes(condition, nodes, pool.Name, kubernetesVersion); exitCondition != nil {
			return exitCondition, nil
		}

		if len(nodes) < int(pool.Minimum) {
			c := v1beta1helper.FailedCondition(h.clock, h.shoot.GetInfo().Status.LastOperation, h.conditionThresholds, condition, "MissingNodes", fmt.Sprintf("Not enough worker nodes registered in worker pool %q to meet minimum desired machine count. (%d/%d).", pool.Name, len(nodes), pool.Minimum))
			return &c, nil
		}
	}

	if err := botanist.OperatingSystemConfigUpdatedForAllWorkerPools(h.shoot.GetInfo().Spec.Provider.Workers, workerPoolToNodes, workerPoolToCloudConfigSecretMeta); err != nil {
		c := v1beta1helper.FailedCondition(h.clock, h.shoot.GetInfo().Status.LastOperation, h.conditionThresholds, condition, oscOutdatedReason, err.Error())
		return &c, nil
	}

	machineDeploymentList := &machinev1alpha1.MachineDeploymentList{}
	if err := h.seedClient.Client().List(ctx, machineDeploymentList, client.InNamespace(h.shoot.SeedNamespace)); err != nil {
		return nil, err
	}

	var (
		nodeList            = convertWorkerPoolToNodesMappingToNodeList(workerPoolToNodes)
		readyNodes          int
		registeredNodes     = len(nodeList.Items)
		desiredMachines     = getDesiredMachineCount(machineDeploymentList.Items)
		nodeNotManagedByMCM int
	)

	for _, node := range nodeList.Items {
		if metav1.HasAnnotation(node.ObjectMeta, annotationKeyNotManagedByMCM) && node.Annotations[annotationKeyNotManagedByMCM] == "1" {
			nodeNotManagedByMCM++
			continue
		}
		if node.Spec.Unschedulable {
			continue
		}
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				readyNodes++
			}
		}
	}

	// only nodes that are managed by MCM is considered
	registeredNodes = registeredNodes - nodeNotManagedByMCM

	machineList := &machinev1alpha1.MachineList{}
	if registeredNodes != desiredMachines || readyNodes != desiredMachines {
		if err := h.seedClient.Client().List(ctx, machineList, client.InNamespace(h.shoot.SeedNamespace)); err != nil {
			return nil, err
		}
	}

	if features.DefaultFeatureGate.Enabled(features.UseGardenerNodeAgent) {
		leaseList := &coordinationv1.LeaseList{}
		if err := shootClient.Client().List(ctx, leaseList, client.InNamespace(metav1.NamespaceSystem)); err != nil {
			return nil, err
		}

		if err := CheckNodeAgentLeases(nodeList, leaseList, h.clock); err != nil {
			c := v1beta1helper.FailedCondition(h.clock, h.shoot.GetInfo().Status.LastOperation, h.conditionThresholds, condition, "NodeAgentUnhealthy", err.Error())
			return &c, nil
		}
	}

	// First check if the MachineDeployments report failed machines. If false then check if the MachineDeployments are
	// "available". If false then check if there is a regular scale-up happening or if there are machines with an erroneous
	// phase. Only then check the other MachineDeployment conditions. As last check, check if there is a scale-down happening
	// (e.g., in case of a rolling-update).

	checkScaleUp := false
	for _, deployment := range machineDeploymentList.Items {
		if len(deployment.Status.FailedMachines) > 0 {
			break
		}

		for _, condition := range deployment.Status.Conditions {
			if condition.Type == machinev1alpha1.MachineDeploymentAvailable && condition.Status != machinev1alpha1.ConditionTrue {
				checkScaleUp = true
				break
			}
		}
	}

	if checkScaleUp {
		if err := CheckNodesScalingUp(machineList, readyNodes, desiredMachines); err != nil {
			c := v1beta1helper.FailedCondition(h.clock, h.shoot.GetInfo().Status.LastOperation, h.conditionThresholds, condition, "NodesScalingUp", err.Error())
			return &c, nil
		}
	}

	if err := CheckNodesScalingDown(machineList, nodeList, registeredNodes, desiredMachines); err != nil {
		c := v1beta1helper.FailedCondition(h.clock, h.shoot.GetInfo().Status.LastOperation, h.conditionThresholds, condition, "NodesScalingDown", err.Error())
		return &c, nil
	}

	return nil, nil
}

// CheckNodeAgentLeases checks if all given nodes have a corresponding lease
func CheckNodeAgentLeases(nodeList *corev1.NodeList, leaseList *coordinationv1.LeaseList, clock clock.Clock) error {
	if len(leaseList.Items) == 0 {
		return fmt.Errorf("no leases")
	}

	leases := map[string]coordinationv1.Lease{}
	for _, lease := range leaseList.Items {
		nodeName := strings.ReplaceAll(lease.Name, gardenerutils.NodeLeasePrefix, "")
		leases[nodeName] = lease
	}

	for _, node := range nodeList.Items {
		nodeLease, ok := leases[node.Name]
		if !ok {
			return fmt.Errorf("gardener-node-agent is not running on node %q", node.Name)
		}

		if nodeLease.Spec.RenewTime.Add(time.Second * time.Duration(*nodeLease.Spec.LeaseDurationSeconds)).Before(clock.Now()) {
			return fmt.Errorf("gardener-node-agent stopped running on node %q", node.Name)
		}
	}

	return nil
}

// CheckNodesScalingUp returns an error if nodes are being scaled up.
func CheckNodesScalingUp(machineList *machinev1alpha1.MachineList, readyNodes, desiredMachines int) error {
	if readyNodes == desiredMachines {
		return nil
	}

	if machineObjects := len(machineList.Items); machineObjects < desiredMachines {
		return fmt.Errorf("not enough machine objects created yet (%d/%d)", machineObjects, desiredMachines)
	}

	var pendingMachines, erroneousMachines int
	for _, machine := range machineList.Items {
		switch machine.Status.CurrentStatus.Phase {
		case machinev1alpha1.MachineRunning, machinev1alpha1.MachineAvailable:
			// machine is already running fine
			continue
		case machinev1alpha1.MachinePending, "": // https://github.com/gardener/machine-controller-manager/issues/466
			// machine is in the process of being created
			pendingMachines++
		default:
			// undesired machine phase
			erroneousMachines++
		}
	}

	if erroneousMachines > 0 {
		return fmt.Errorf("%s erroneous", cosmeticMachineMessage(erroneousMachines))
	}
	if pendingMachines == 0 {
		return fmt.Errorf("not enough ready worker nodes registered in the cluster (%d/%d)", readyNodes, desiredMachines)
	}

	return fmt.Errorf("%s provisioning and should join the cluster soon", cosmeticMachineMessage(pendingMachines))
}

// CheckNodesScalingDown returns an error if nodes are being scaled down.
func CheckNodesScalingDown(machineList *machinev1alpha1.MachineList, nodeList *corev1.NodeList, registeredNodes, desiredMachines int) error {
	if registeredNodes == desiredMachines {
		return nil
	}

	// Check if all nodes that are cordoned map to machines with a deletion timestamp. This might be the case during
	// a rolling update.
	nodeNameToMachine := map[string]machinev1alpha1.Machine{}
	for _, machine := range machineList.Items {
		if machine.Labels != nil && machine.Labels["node"] != "" {
			nodeNameToMachine[machine.Labels["node"]] = machine
		}
	}

	var cordonedNodes int
	for _, node := range nodeList.Items {
		if metav1.HasAnnotation(node.ObjectMeta, annotationKeyNotManagedByMCM) && node.Annotations[annotationKeyNotManagedByMCM] == "1" {
			continue
		}
		if node.Spec.Unschedulable {
			machine, ok := nodeNameToMachine[node.Name]
			if !ok {
				return fmt.Errorf("machine object for cordoned node %q not found", node.Name)
			}
			if machine.DeletionTimestamp == nil {
				return fmt.Errorf("cordoned node %q found but corresponding machine object does not have a deletion timestamp", node.Name)
			}
			cordonedNodes++
		}
	}

	// If there are still more nodes than desired then report an error.
	if registeredNodes-cordonedNodes != desiredMachines {
		return fmt.Errorf("too many worker nodes are registered. Exceeding maximum desired machine count (%d/%d)", registeredNodes, desiredMachines)
	}

	return fmt.Errorf("%s waiting to be completely drained from pods. If this persists, check your pod disruption budgets and pending finalizers. Please note, that nodes that fail to be drained will be deleted automatically", cosmeticMachineMessage(cordonedNodes))
}

func convertWorkerPoolToNodesMappingToNodeList(workerPoolToNodes map[string][]corev1.Node) *corev1.NodeList {
	nodeList := &corev1.NodeList{}

	for _, nodes := range workerPoolToNodes {
		nodeList.Items = append(nodeList.Items, nodes...)
	}

	return nodeList
}

func getDesiredMachineCount(machineDeployments []machinev1alpha1.MachineDeployment) int {
	desiredMachines := 0
	for _, deployment := range machineDeployments {
		if deployment.DeletionTimestamp == nil {
			desiredMachines += int(deployment.Spec.Replicas)
		}
	}
	return desiredMachines
}

func cosmeticMachineMessage(numberOfMachines int) string {
	if numberOfMachines == 1 {
		return fmt.Sprintf("%d machine is", numberOfMachines)
	}
	return fmt.Sprintf("%d machines are", numberOfMachines)
}

// ComputeRequiredControlPlaneDeployments returns names of required deployments based on the given shoot.
func ComputeRequiredControlPlaneDeployments(shoot *gardencorev1beta1.Shoot) (sets.Set[string], error) {
	requiredControlPlaneDeployments := commonControlPlaneDeployments.Clone()

	if !v1beta1helper.IsWorkerless(shoot) {
		requiredControlPlaneDeployments.Insert(v1beta1constants.DeploymentNameKubeScheduler)
		requiredControlPlaneDeployments.Insert(v1beta1constants.DeploymentNameMachineControllerManager)

		shootWantsClusterAutoscaler, err := v1beta1helper.ShootWantsClusterAutoscaler(shoot)
		if err != nil {
			return nil, err
		}

		if shootWantsClusterAutoscaler {
			requiredControlPlaneDeployments.Insert(v1beta1constants.DeploymentNameClusterAutoscaler)
		}

		if v1beta1helper.ShootWantsVerticalPodAutoscaler(shoot) {
			for _, vpaDeployment := range v1beta1constants.GetShootVPADeploymentNames() {
				requiredControlPlaneDeployments.Insert(vpaDeployment)
			}
		}
	}

	return requiredControlPlaneDeployments, nil
}

func shootHibernatedConditions(clock clock.Clock, conditions []gardencorev1beta1.Condition) []gardencorev1beta1.Condition {
	hibernationConditions := make([]gardencorev1beta1.Condition, 0, len(conditions))
	for _, cond := range conditions {
		hibernationConditions = append(hibernationConditions, v1beta1helper.UpdatedConditionWithClock(clock, cond, gardencorev1beta1.ConditionTrue, "ConditionNotChecked", "Shoot cluster has been hibernated."))
	}
	return hibernationConditions
}

func shootControlPlaneNotRunningMessage(lastOperation *gardencorev1beta1.LastOperation) string {
	switch {
	case lastOperation == nil || lastOperation.Type == gardencorev1beta1.LastOperationTypeCreate:
		return "Shoot control plane has not been fully created yet."
	case lastOperation.Type == gardencorev1beta1.LastOperationTypeDelete:
		return "Shoot control plane has already been or is about to be deleted."
	}
	return "Shoot control plane is not running at the moment."
}

// PardonConditions pardons the given condition if the Shoot is either in create (except successful create) or delete state.
func PardonConditions(clock clock.Clock, conditions []gardencorev1beta1.Condition, lastOp *gardencorev1beta1.LastOperation, lastErrors []gardencorev1beta1.LastError) []gardencorev1beta1.Condition {
	pardoningConditions := make([]gardencorev1beta1.Condition, 0, len(conditions))
	for _, cond := range conditions {
		if (lastOp == nil || isUnstableLastOperation(lastOp, lastErrors)) && cond.Status == gardencorev1beta1.ConditionFalse {
			pardoningConditions = append(pardoningConditions, v1beta1helper.UpdatedConditionWithClock(clock, cond, gardencorev1beta1.ConditionProgressing, cond.Reason, cond.Message, cond.Codes...))
			continue
		}
		pardoningConditions = append(pardoningConditions, cond)
	}
	return pardoningConditions
}

func isUnstableLastOperation(lastOperation *gardencorev1beta1.LastOperation, lastErrors []gardencorev1beta1.LastError) bool {
	return (isUnstableOperationType(lastOperation.Type) && lastOperation.State != gardencorev1beta1.LastOperationStateSucceeded) ||
		(lastOperation.State == gardencorev1beta1.LastOperationStateProcessing && lastErrors == nil)
}

var unstableOperationTypes = map[gardencorev1beta1.LastOperationType]struct{}{
	gardencorev1beta1.LastOperationTypeCreate: {},
	gardencorev1beta1.LastOperationTypeDelete: {},
}

func isUnstableOperationType(lastOperationType gardencorev1beta1.LastOperationType) bool {
	_, ok := unstableOperationTypes[lastOperationType]
	return ok
}

// ShootConditions contains all shoot related conditions of the shoot status subresource.
type ShootConditions struct {
	apiServerAvailable             gardencorev1beta1.Condition
	controlPlaneHealthy            gardencorev1beta1.Condition
	observabilityComponentsHealthy gardencorev1beta1.Condition
	systemComponentsHealthy        gardencorev1beta1.Condition
	everyNodeReady                 *gardencorev1beta1.Condition
}

// ConvertToSlice returns the shoot conditions as a slice.
func (s ShootConditions) ConvertToSlice() []gardencorev1beta1.Condition {
	conditions := []gardencorev1beta1.Condition{
		s.apiServerAvailable,
		s.controlPlaneHealthy,
		s.observabilityComponentsHealthy,
	}

	if s.everyNodeReady != nil {
		conditions = append(conditions, *s.everyNodeReady)
	}

	return append(conditions, s.systemComponentsHealthy)
}

// ConditionTypes returns all shoot condition types.
func (s ShootConditions) ConditionTypes() []gardencorev1beta1.ConditionType {
	types := []gardencorev1beta1.ConditionType{
		s.apiServerAvailable.Type,
		s.controlPlaneHealthy.Type,
		s.observabilityComponentsHealthy.Type,
	}

	if s.everyNodeReady != nil {
		types = append(types, gardencorev1beta1.ShootEveryNodeReady)
	}

	return append(types, s.systemComponentsHealthy.Type)
}

// NewShootConditions returns a new instance of ShootConditions.
// All conditions are retrieved from the given 'shoot' or newly initialized.
func NewShootConditions(clock clock.Clock, shoot *gardencorev1beta1.Shoot) ShootConditions {
	shootConditions := ShootConditions{
		apiServerAvailable:             v1beta1helper.GetOrInitConditionWithClock(clock, shoot.Status.Conditions, gardencorev1beta1.ShootAPIServerAvailable),
		controlPlaneHealthy:            v1beta1helper.GetOrInitConditionWithClock(clock, shoot.Status.Conditions, gardencorev1beta1.ShootControlPlaneHealthy),
		observabilityComponentsHealthy: v1beta1helper.GetOrInitConditionWithClock(clock, shoot.Status.Conditions, gardencorev1beta1.ShootObservabilityComponentsHealthy),
		systemComponentsHealthy:        v1beta1helper.GetOrInitConditionWithClock(clock, shoot.Status.Conditions, gardencorev1beta1.ShootSystemComponentsHealthy),
	}

	if !v1beta1helper.IsWorkerless(shoot) {
		nodeCondition := v1beta1helper.GetOrInitConditionWithClock(clock, shoot.Status.Conditions, gardencorev1beta1.ShootEveryNodeReady)
		shootConditions.everyNodeReady = &nodeCondition
	}

	return shootConditions
}
