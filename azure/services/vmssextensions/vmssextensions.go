/*
Copyright 2021 The Kubernetes Authors.

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

package vmssextensions

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2020-06-30/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1alpha4"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"

	"github.com/go-logr/logr"
	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

// VMSSExtensionScope defines the scope interface for a vmss extension service.
type VMSSExtensionScope interface {
	logr.Logger
	azure.ClusterDescriber
	VMSSExtensionSpecs() []azure.VMSSExtensionSpec
	SetCondition(clusterv1.ConditionType, string, clusterv1.ConditionSeverity, bool)
}

// Service provides operations on azure resources
type Service struct {
	Scope VMSSExtensionScope
	client
}

// New creates a new vm extension service.
func New(scope VMSSExtensionScope) *Service {
	return &Service{
		Scope:  scope,
		client: newClient(scope),
	}
}

// Reconcile creates or updates the VMSS extension.
func (s *Service) Reconcile(ctx context.Context) error {
	_, span := tele.Tracer().Start(ctx, "vmssextensions.Service.Reconcile")
	defer span.End()

	for _, extensionSpec := range s.Scope.VMSSExtensionSpecs() {
		if existing, err := s.client.Get(ctx, s.Scope.ResourceGroup(), extensionSpec.ScaleSetName, extensionSpec.Name); err == nil {
			// check the extension status and set the associated conditions.
			switch compute.ProvisioningState(to.String(existing.ProvisioningState)) {
			case compute.ProvisioningStateSucceeded:
				s.Scope.V(4).Info("extension provisioning state is succeeded", "vm extension", extensionSpec.Name, "scale set", extensionSpec.ScaleSetName)
				s.Scope.SetCondition(infrav1.BootstrapSucceededCondition, "", "", true)
			case compute.ProvisioningStateCreating:
				s.Scope.V(4).Info("extension provisioning state is creating", "vm extension", extensionSpec.Name, "scale set", extensionSpec.ScaleSetName)
				s.Scope.SetCondition(infrav1.BootstrapSucceededCondition, infrav1.BootstrapInProgressReason, clusterv1.ConditionSeverityInfo, false)
				return errors.New("extension still provisioning")
			case compute.ProvisioningStateFailed:
				s.Scope.V(4).Info("extension provisioning state is failed", "vm extension", extensionSpec.Name, "scale set", extensionSpec.ScaleSetName)
				s.Scope.SetCondition(infrav1.BootstrapSucceededCondition, infrav1.BootstrapFailedReason, clusterv1.ConditionSeverityError, false)
				return errors.New("extension state failed")
			}
		} else if !azure.ResourceNotFound(err) {
			return errors.Wrapf(err, "failed to get vm extension %s on scale set %s", extensionSpec.Name, extensionSpec.ScaleSetName)
		}

		//  Nothing else to do here, the extensions are applied to the model as part of the scale set Reconcile.
	}
	return nil
}

// Delete is a no-op. Extensions will be deleted as part of VMSS deletion.
func (s *Service) Delete(_ context.Context) error {
	return nil
}
