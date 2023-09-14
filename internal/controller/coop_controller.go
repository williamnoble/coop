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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	utilv1 "github.com/williamnoble/coop/api/v1"
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
	coop := &utilv1.Coop{}
	err := r.Get(ctx, req.NamespacedName, coop)
	if err != nil {
		log.Error(err, "Failed to fetch Coop")
		// this is more succint than checking if err != IsNotFound(err) and returning nil
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if len(coop.Status.Conditions) == 0 {
		meta.SetStatusCondition(&coop.Status.Conditions,
			metav1.Condition{
				Type:    typeCoopAvailable,
				Status:  metav1.ConditionUnknown,
				Reason:  "Reconciling",
				Message: "Starting Reconciliation",
			})

		if err := r.Status().Update(ctx, coop); err != nil {
			log.Error(err, "Failed to update coop status")
			return ctrl.Result{}, err // requeue
		}

		if err := r.Get(ctx, req.NamespacedName, coop); err != nil {
			log.Error(err, "Failed to re-fetch Coop")
			return ctrl.Result{}, err
		}
	}

	if !controllerutil.ContainsFinalizer(coop, coopFinalizer) {

		log.Info("Adding finalizer for Coop")
		if ok := controllerutil.AddFinalizer(coop, coopFinalizer); !ok {
			log.Info("ONE")
			log.Error(err, "Failed to add finalizer to coop")
			log.Info("TWO")
			return ctrl.Result{Requeue: true}, nil // bool so requeue as no err
		}

		if err := r.Update(ctx, coop); err != nil {
			log.Info("THREE")
			log.Error(err, "Failed to update Coop and add finalizer")
			return ctrl.Result{}, err
		}
	}

	coopIsMarkedForDeletion := coop.GetDeletionTimestamp() != nil
	if coopIsMarkedForDeletion {
		if controllerutil.ContainsFinalizer(coop, coopFinalizer) {
			log.Info("Found Coop finalizer, performing cleanup of resources")
			// perform some cleanup, delete configmaps
		}

		meta.SetStatusCondition(&coop.Status.Conditions, metav1.Condition{
			Type:    typeCoopDegraded,
			Status:  metav1.ConditionUnknown,
			Reason:  "Finalizing",
			Message: "Performing finalizer for Coop",
		})

		if err := r.Status().Update(ctx, coop); err != nil {
			log.Error(err, "Failed to update Coop status")
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
			log.Error(err, "Failed to update Coop status")
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

	// validate Spec

	// copyConfigMap i.e. update targets

	// 1. Check if the config map already exists
	v1ConfigMap := &v1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: coop.Spec.Source.Name, Namespace: coop.Spec.Destination.Namespace}, v1ConfigMap)
	if err != nil && errors.IsNotFound(err) {

		// 2. Assuming the config map doesn't exist copy it
		newConfigMap := &v1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      coop.Spec.Source.Name,
				Namespace: coop.Spec.Destination.Namespace,
			},
			Data: v1ConfigMap.Data,
		}

		log.Info(fmt.Sprintf("Copying %q ConfigMap from %q namespace to %q namespace", coop.Spec.Source.Name, coop.Spec.Source.Namespace, coop.Spec.Destination.Namespace))
		// 3. Attempt to create a new config map in the destination namespace
		if err = r.Create(ctx, newConfigMap); err != nil {
			log.Error(err, "Failed to create a new config map at destination")
			return ctrl.Result{}, err
		}

		// requeue after 10 seconds to ensure configmap created
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil

	} else if err != nil {
		log.Error(err, "Failed to get Coop Source configMap")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CoopReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&utilv1.Coop{}).
		Complete(r)
}
