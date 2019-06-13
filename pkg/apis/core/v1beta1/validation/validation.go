/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validation

import (
	"fmt"
	"strings"
	"time"

	apiextv1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apimachineryval "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	valutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/leaderelection"

	"sigs.k8s.io/kubefed/pkg/apis/core/typeconfig"
	"sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
	"sigs.k8s.io/kubefed/pkg/features"
)

func ValidateFederatedTypeConfig(obj *v1beta1.FederatedTypeConfig, statusSubResource bool) field.ErrorList {
	var allErrs field.ErrorList
	if !statusSubResource {
		allErrs = ValidateFederatedTypeConfigName(obj)
		allErrs = append(allErrs, ValidateFederatedTypeConfigSpec(&obj.Spec, field.NewPath("spec"))...)
	} else {
		allErrs = ValidateFederatedTypeConfigStatus(&obj.Status, field.NewPath("status"))
	}
	return allErrs
}

const federatedTypeConfigNameErrorMsg string = "name must be 'TARGET_PLURAL_NAME(.TARGET_GROUP_NAME)'"

func ValidateFederatedTypeConfigName(obj *v1beta1.FederatedTypeConfig) field.ErrorList {
	expectedName := typeconfig.GroupQualifiedName(obj.GetTargetType())
	if expectedName != obj.Name {
		return field.ErrorList{field.Invalid(field.NewPath("name"), obj.Name, federatedTypeConfigNameErrorMsg)}
	}
	return field.ErrorList{}
}

func ValidateFederatedTypeConfigSpec(spec *v1beta1.FederatedTypeConfigSpec, fldPath *field.Path) field.ErrorList {
	allErrs := ValidateAPIResource(&spec.TargetType, fldPath.Child("targetType"))
	allErrs = append(allErrs, validateEnumStrings(fldPath.Child("propagation"), string(spec.Propagation), []string{string(v1beta1.PropagationEnabled), string(v1beta1.PropagationDisabled)})...)
	allErrs = append(allErrs, ValidateFederatedAPIResource(&spec.FederatedType, fldPath.Child("federatedType"))...)
	if spec.StatusType != nil {
		allErrs = append(allErrs, ValidateStatusAPIResource(spec.StatusType, fldPath.Child("statusType"))...)
	}

	if spec.StatusCollection != nil {
		allErrs = append(allErrs, validateEnumStrings(fldPath.Child("statusCollection"), string(*spec.StatusCollection), []string{string(v1beta1.StatusCollectionEnabled), string(v1beta1.StatusCollectionDisabled)})...)
	}

	return allErrs
}

const domainWithAtLeastOneDot string = "should be a domain with at least one dot"

func ValidateFederatedAPIResource(fedType *v1beta1.APIResource, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(fedType.Group) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("group"), ""))
	} else if len(strings.Split(fedType.Group, ".")) < 2 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("group"), fedType.Group, domainWithAtLeastOneDot))
	}

	allErrs = append(allErrs, ValidateAPIResource(fedType, fldPath)...)
	return allErrs
}

func ValidateStatusAPIResource(statusType *v1beta1.APIResource, fldPath *field.Path) field.ErrorList {
	return ValidateFederatedAPIResource(statusType, fldPath)
}

func ValidateAPIResource(obj *v1beta1.APIResource, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(obj.Group) != 0 {
		if errs := valutil.IsDNS1123Subdomain(obj.Group); len(errs) > 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("group"), obj.Group, strings.Join(errs, ",")))
		}
	}

	if len(obj.Version) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("version"), ""))
	} else if errs := valutil.IsDNS1035Label(obj.Version); len(errs) > 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("version"), obj.Version, strings.Join(errs, ",")))
	}

	if len(obj.Kind) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("kind"), ""))
	} else if errs := valutil.IsDNS1035Label(strings.ToLower(obj.Kind)); len(errs) > 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), obj.Kind, strings.Join(errs, ",")))
	}

	if len(obj.PluralName) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("pluralName"), ""))
	} else if errs := valutil.IsDNS1035Label(obj.PluralName); len(errs) > 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("pluralName"), obj.PluralName, strings.Join(errs, ",")))
	}

	allErrs = append(allErrs, validateEnumStrings(fldPath.Child("scope"), string(obj.Scope), []string{string(apiextv1b1.ClusterScoped), string(apiextv1b1.NamespaceScoped)})...)

	return allErrs
}

func validateEnumStrings(fldPath *field.Path, value string, accepted []string) field.ErrorList {
	if value == "" {
		return field.ErrorList{field.Required(fldPath, "")}
	}
	for _, a := range accepted {
		if a == value {
			return field.ErrorList{}
		}
	}
	return field.ErrorList{field.NotSupported(fldPath, value, accepted)}
}

func ValidateFederatedTypeConfigStatus(status *v1beta1.FederatedTypeConfigStatus, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, apimachineryval.ValidateNonnegativeField(status.ObservedGeneration, fldPath.Child("observedGeneration"))...)
	allErrs = append(allErrs, validateEnumStrings(fldPath.Child("propagationController"), string(status.PropagationController), []string{string(v1beta1.ControllerStatusRunning), string(v1beta1.ControllerStatusNotRunning)})...)

	if status.StatusController != nil {
		allErrs = append(allErrs, validateEnumStrings(fldPath.Child("statusController"), string(*status.StatusController), []string{string(v1beta1.ControllerStatusRunning), string(v1beta1.ControllerStatusNotRunning)})...)
	}
	return allErrs
}

func ValidateKubeFedConfig(kubeFedConfig *v1beta1.KubeFedConfig) field.ErrorList {
	allErrs := field.ErrorList{}

	spec := kubeFedConfig.Spec
	specPath := field.NewPath("spec")
	allErrs = append(allErrs, validateEnumStrings(specPath.Child("scope"), string(spec.Scope),
		[]string{string(apiextv1b1.ClusterScoped), string(apiextv1b1.NamespaceScoped)})...)

	duration := spec.ControllerDuration
	durationPath := specPath.Child("controllerDuration")
	if duration == nil {
		allErrs = append(allErrs, field.Required(durationPath, ""))
	} else {
		allErrs = append(allErrs, validateDurationGreaterThan0(durationPath.Child("availableDelay"), duration.AvailableDelay)...)
		allErrs = append(allErrs, validateDurationGreaterThan0(durationPath.Child("unavailableDelay"), duration.UnavailableDelay)...)
	}

	elect := spec.LeaderElect
	electPath := specPath.Child("leaderElect")
	if elect == nil {
		allErrs = append(allErrs, field.Required(electPath, ""))
	} else {
		allErrs = append(allErrs, validateDurationGreaterThan0(electPath.Child("leaseDuration"), elect.LeaseDuration)...)
		allErrs = append(allErrs, validateDurationGreaterThan0(electPath.Child("renewDeadline"), elect.RenewDeadline)...)
		allErrs = append(allErrs, validateDurationGreaterThan0(electPath.Child("retryPeriod"), elect.RetryPeriod)...)
		if elect.LeaseDuration != nil && elect.RenewDeadline != nil &&
			elect.LeaseDuration.Duration <= elect.RenewDeadline.Duration {

			allErrs = append(allErrs, field.Invalid(electPath.Child("leaseDuration"), elect.LeaseDuration,
				"leaseDuration must be greater than renewDeadline"))
		}
		if elect.RenewDeadline != nil && elect.RetryPeriod != nil &&
			elect.RenewDeadline.Duration <= time.Duration(float64(elect.RetryPeriod.Duration)*leaderelection.JitterFactor) {

			allErrs = append(allErrs, field.Invalid(electPath.Child("renewDeadline"), elect.RenewDeadline,
				fmt.Sprintf("renewDeadline must be greater than retryPeriod*JitterFactor(%.1f)", leaderelection.JitterFactor)))
		}
		resourceLockPath := electPath.Child("resourceLock")
		if elect.ResourceLock == nil {
			allErrs = append(allErrs, field.Required(resourceLockPath, ""))
		} else {
			allErrs = append(allErrs, validateEnumStrings(resourceLockPath, string(*elect.ResourceLock),
				[]string{string(v1beta1.ConfigMapsResourceLock), string(v1beta1.EndpointsResourceLock)})...)
		}
	}

	gates := spec.FeatureGates
	gatesPath := specPath.Child("featureGates")
	if gates == nil {
		allErrs = append(allErrs, field.Required(gatesPath, ""))
	} else {
		existingNames := make(map[string]bool)
		for _, gate := range gates {
			_, ok := existingNames[gate.Name]
			if ok {
				allErrs = append(allErrs, field.Duplicate(gatesPath.Child("name"), gate.Name))
				continue
			}
			existingNames[gate.Name] = true

			allErrs = append(allErrs, validateEnumStrings(gatesPath.Child("name"), string(gate.Name),
				[]string{string(features.PushReconciler), string(features.SchedulerPreferences),
					string(features.CrossClusterServiceDiscovery), string(features.FederatedIngress)})...)

			allErrs = append(allErrs, validateEnumStrings(gatesPath.Child("configuration"), string(gate.Configuration),
				[]string{string(v1beta1.ConfigurationEnabled), string(v1beta1.ConfigurationDisabled)})...)
		}
	}

	health := spec.ClusterHealthCheck
	healthPath := specPath.Child("clusterHealthCheck")
	if health == nil {
		allErrs = append(allErrs, field.Required(healthPath, ""))
	} else {
		allErrs = append(allErrs, validateDurationGreaterThan0(healthPath.Child("period"), health.Period)...)
		allErrs = append(allErrs, validateIntPtrGreaterThan0(healthPath.Child("failureThreshold"), health.FailureThreshold)...)
		allErrs = append(allErrs, validateIntPtrGreaterThan0(healthPath.Child("successThreshold"), health.SuccessThreshold)...)
		allErrs = append(allErrs, validateDurationGreaterThan0(healthPath.Child("timeout"), health.Timeout)...)
	}

	sync := spec.SyncController
	syncPath := specPath.Child("syncController")
	adoptPath := syncPath.Child("adoptResources")
	if sync == nil {
		allErrs = append(allErrs, field.Required(syncPath, ""))
	} else if sync.AdoptResources == nil {
		allErrs = append(allErrs, field.Required(adoptPath, ""))
	} else {
		allErrs = append(allErrs, validateEnumStrings(adoptPath, string(*sync.AdoptResources),
			[]string{string(v1beta1.AdoptResourcesEnabled), string(v1beta1.AdoptResourcesDisabled)})...)
	}

	return allErrs
}

func validateDurationGreaterThan0(path *field.Path, duration *metav1.Duration) field.ErrorList {
	errs := field.ErrorList{}
	if duration == nil {
		errs = append(errs, field.Required(path, ""))
	} else {
		errs = validateGreaterThan0(path, int64(duration.Duration))
	}
	return errs
}

func validateIntPtrGreaterThan0(path *field.Path, value *int64) field.ErrorList {
	errs := field.ErrorList{}
	if value == nil {
		errs = append(errs, field.Required(path, ""))
	} else {
		errs = validateGreaterThan0(path, *value)
	}
	return errs
}

func validateGreaterThan0(path *field.Path, value int64) field.ErrorList {
	errs := field.ErrorList{}
	if value <= 0 {
		errs = append(errs, field.Invalid(path, value, "should be greater than 0"))
	}
	return errs
}

func ValidateKubeFedCluster(object *v1beta1.KubeFedCluster) field.ErrorList {
	allErrs := field.ErrorList{}
	return allErrs
}
