/*
Copyright 2023.

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

package controller

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"

	apiv1 "github.com/williamnoble/coop/api/v1"
)

// CoopReconciler reconciles a Coop object
type CoopReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	coopFinalizer     = "util.williamnoble.developer.com/finalizer"
	typeCoopAvailable = "Available"
	typeCoopDegraded  = "Degraded"
)

//+kubebuilder:rbac:groups=util.williamnoble.developer.com,resources=coops,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=util.williamnoble.developer.com,resources=coops/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=util.williamnoble.developer.com,resources=coops/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *CoopReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	coop := &apiv1.Coop{}
	err := r.Get(ctx, req.NamespacedName, coop)
	if err != nil {
		log.Info("Failed to fetch Coop object. Ignoring, probably deleted")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.V(1).Info("Updating Status Condition")
	if len(coop.Status.Conditions) == 0 {
		meta.SetStatusCondition(&coop.Status.Conditions,
			metav1.Condition{
				Type:    typeCoopAvailable,
				Status:  metav1.ConditionUnknown,
				Reason:  "Reconciling",
				Message: "Starting Reconciliation",
			})

		if err := r.Status().Update(ctx, coop); err != nil {
			if errors.IsConflict(err) {
				log.Info("Resource conflict detected, re-queuing")
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "Failed to update coop status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, coop); err != nil {
			log.Error(err, "Failed to fetch Coop after updating conditions")
			return ctrl.Result{}, err
		}
	}

	log.V(1).Info("Updating Finalizer")
	if !controllerutil.ContainsFinalizer(coop, coopFinalizer) {
		log.Info("Adding finalizer")
		if ok := controllerutil.AddFinalizer(coop, coopFinalizer); !ok {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{Requeue: true}, nil
		}

		if err := r.Update(ctx, coop); err != nil {
			log.Error(err, "Failed to update Coop and add finalizer")
			return ctrl.Result{}, err
		}
	}

	coopIsMarkedForDeletion := coop.GetDeletionTimestamp() != nil
	if coopIsMarkedForDeletion {
		log.V(1).Info("Marked for deletion")
		if controllerutil.ContainsFinalizer(coop, coopFinalizer) {
			err := r.cleanupResources(ctx, coop)
			if err != nil {
				log.Error(err, "Failed to clean up resources")
				return ctrl.Result{}, err
			}
		}

		meta.SetStatusCondition(&coop.Status.Conditions, metav1.Condition{
			Type:    typeCoopDegraded,
			Status:  metav1.ConditionUnknown,
			Reason:  "Finalizing",
			Message: "Performing finalizer for Coop",
		})

		if err := r.Status().Update(ctx, coop); err != nil {
			if errors.IsConflict(err) {
				log.Info("Resource conflict detected, re-queuing")
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "Failed to update coop status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, coop); err != nil {
			log.Error(err, "Failed to re-fetch Coop")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&coop.Status.Conditions, metav1.Condition{
			Type:    typeCoopDegraded,
			Status:  metav1.ConditionTrue,
			Reason:  "Finalizing",
			Message: "Finalizer operations for Coop were successful",
		})

		if err := r.Status().Update(ctx, coop); err != nil {
			if errors.IsConflict(err) {
				log.Info("Resource conflict detected, requeuing")
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "Failed to update coop status")
			return ctrl.Result{}, err
		}

		log.Info("Removing finalizer for Coop")
		if ok := controllerutil.RemoveFinalizer(coop, coopFinalizer); !ok {
			log.Error(err, "Failed to remove finalizer.")
			return ctrl.Result{Requeue: true}, nil
		}

		if err := r.Update(ctx, coop); err != nil {
			log.Error(err, "Failed to remove finalizer for memcached")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	if err := validateSpec(coop); err != nil {
		log.Error(err, "Encountered an error reading the coop spec")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Performing main reconciliation logic")
	// check if the source config map already exists
	sourceConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: coop.Spec.Source.Name, Namespace: coop.Spec.Source.Namespace}, sourceConfigMap)
	if err == nil {
		// if the source config map exists
		log.Info("Found ConfigMap in source Namespace")

		configMap := &corev1.ConfigMap{}
		// we need to check if the configmap already exists in the destination
		log.Info("Checking the destination namespace")
		err = r.Get(ctx, types.NamespacedName{Name: coop.Spec.Source.Name, Namespace: coop.Spec.Destination.Namespace}, configMap)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Destination namespace was empty so creating configmap shortly")
			newConfigMap := sourceConfigMap.DeepCopy()
			newConfigMap.Namespace = coop.Spec.Destination.Namespace
			newConfigMap.ResourceVersion = ""

			log.Info(fmt.Sprintf("Copying %q ConfigMap from %q namespace to %q namespace", coop.Spec.Source.Name, coop.Spec.Source.Namespace, coop.Spec.Destination.Namespace))

			// attempt to create a new config map in the destination namespace
			if err = r.Create(ctx, newConfigMap); err != nil {
				log.Error(err, "Failed to create a new config map at destination")
				return ctrl.Result{}, err
			}

			log.Info("Found Coop: reconciling [EndOfConfigMapCheck]")
			// requeue after 5 seconds to ensure the configmap created
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
	}
	log.V(1).Info("Completed Reconcilliaton Loop")
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CoopReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1.Coop{}).
		Complete(r)
}

func validateSpec(c *apiv1.Coop) error {
	if c.Spec.Source.Namespace == "" || c.Spec.Destination.Namespace == "" || c.Spec.Source.Name == "" {
		return fmt.Errorf("invalid Spec\n")
	}

	return nil
}

func (r *CoopReconciler) cleanupResources(ctx context.Context, coop *apiv1.Coop) error {
	log := log.FromContext(ctx)

	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      coop.Spec.Source.Name,
		Namespace: coop.Spec.Destination.Namespace,
	}, configMap)

	if err == nil {
		log.Info("Deleting ConfigMap from destination namespace as part of cleanup",
			"namespace", coop.Spec.Destination.Namespace,
			"name", coop.Spec.Source.Name)

		if err = r.Delete(ctx, configMap); err != nil {
			log.Error(err, "Failed to delete ConfigMap during cleanup")
			return err
		}
	} else if !errors.IsNotFound(err) {
		return err
	}

	return nil
}
