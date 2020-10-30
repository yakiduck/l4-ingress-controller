/*


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

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/l4-ingress-controller/util"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	poweriov1 "github.com/l4-ingress-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	IngressNamespace      = "ingress-nginx"
	IngressTcpConfigmap   = "tcp-services"
	IngressBackendService = "default-http-backend"

	IngressTcpConfigmapCleanupFinalizer   = "power.io/ingress-tcp-configmap-cleanup"
	IngressBackendServiceCleanupFinalizer = "power.io/ingress-backend-service-cleanup"
)

// TcpIngressReconciler reconciles a TcpIngress object
type TcpIngressReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=power.io,resources=tcpingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=power.io,resources=tcpingresses/status,verbs=get;update;patch

func (r *TcpIngressReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("tcpingress", req.NamespacedName)

	var tcpIngress poweriov1.TcpIngress
	if err := r.Get(ctx, req.NamespacedName, &tcpIngress); err != nil {
		if errors.IsNotFound(err) {
			log.Info("tcpingress is not found")
		} else {
			log.Error(err, "unable to fetch tcpingress")
		}
		//忽略掉 not-found 错误，它们不能通过重新排队修复（要等待新的通知）
		//在删除一个不存在的对象时，可能会报这个错误。
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: IngressBackendService, Namespace: IngressNamespace}, service); err != nil {
		log.Error(err, "unable to get ingress controller backend service")
		return ctrl.Result{}, err
	}
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: IngressTcpConfigmap, Namespace: IngressNamespace}, configMap); err != nil {
		log.Error(err, "unable to get ingress controller tcp configmap")
		return ctrl.Result{}, err
	}

	if tcpIngress.DeletionTimestamp.IsZero() {
		if !util.ContainString(tcpIngress.Finalizers, IngressTcpConfigmapCleanupFinalizer) {
			tcpIngress.Finalizers = append(tcpIngress.Finalizers, IngressTcpConfigmapCleanupFinalizer)
		}

		if !util.ContainString(tcpIngress.Finalizers, IngressBackendServiceCleanupFinalizer) {
			tcpIngress.Finalizers = append(tcpIngress.Finalizers, IngressBackendServiceCleanupFinalizer)
		}

		if err := r.Update(ctx, &tcpIngress); err != nil {
			log.Error(err, "failed to init tcpingress finalizers")
			return ctrl.Result{}, err
		}
	} else {
		// TcpIngress is being deleted
		if util.ContainString(tcpIngress.Finalizers, IngressTcpConfigmapCleanupFinalizer) {
			if err := r.deleteConfigMapData(ctx, configMap, tcpIngress); err != nil {
				log.Error(err, "failed to delete data in ingress controller tcp configmap")
				return ctrl.Result{}, err
			}
			tcpIngress.Finalizers = util.RemoveString(tcpIngress.Finalizers, IngressTcpConfigmapCleanupFinalizer)
			if err := r.Update(ctx, &tcpIngress); err != nil {
				log.Error(err, "failed to execute tcpingress finalizers")
				return ctrl.Result{}, err
			}
		}

		if util.ContainString(tcpIngress.Finalizers, IngressBackendServiceCleanupFinalizer) {
			if err := r.deleteServicePort(ctx, service, tcpIngress); err != nil {
				log.Error(err, "failed to delete port in ingress controller backend service")
				return ctrl.Result{}, err
			}
			tcpIngress.Finalizers = util.RemoveString(tcpIngress.Finalizers, IngressBackendServiceCleanupFinalizer)
			if err := r.Update(ctx, &tcpIngress); err != nil {
				log.Error(err, "failed to execute tcpingress finalizers")
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the finalizer item is being deleted
		return ctrl.Result{}, nil
	}

	if err := r.updateService(ctx, service, tcpIngress); err != nil {
		log.Error(err, "failed to update ingress controller backend service")
		return ctrl.Result{}, err
	}

	if err := r.updateConfigMap(ctx, configMap, tcpIngress); err != nil {
		log.Error(err, "failed to update ingress controller tcp configmap")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *TcpIngressReconciler) updateConfigMap(ctx context.Context, configMap *corev1.ConfigMap, tcpIngress poweriov1.TcpIngress) error {
	if configMap.Data == nil {
		configMap.Data = map[string]string{}
	}

	removeConfigMapDataInTcpIngress(configMap, tcpIngress)

	for _, port := range tcpIngress.Spec.Ports {
		externalPort := strconv.FormatInt(int64(port.ExternalPort), 10)
		if val, ok := configMap.Data[externalPort]; ok && getConfigMapDataValuePrefix(val) != getServiceFullName(tcpIngress) {
			return fmt.Errorf("port %s is in use", externalPort)
		}
		configMap.Data[externalPort] = fmt.Sprintf("%s:%d", getServiceFullName(tcpIngress), port.ServicePort)

	}

	if err := r.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update configmap: %v", err)
	}

	return nil
}

func (r *TcpIngressReconciler) updateService(ctx context.Context, service *corev1.Service, tcpIngress poweriov1.TcpIngress) error {
	portNamePrefix := getPortNamePrefix(tcpIngress)

	for index, port := range service.Spec.Ports {
		if port.Port == 80 && service.Spec.Ports[index].Name == "" {
			service.Spec.Ports[index].Name = "http"
		}
		if port.Port == 443 && service.Spec.Ports[index].Name == "" {
			service.Spec.Ports[index].Name = "https"
		}
	}

	removeServicePortInTcpIngress(service, tcpIngress)

	for index, port := range tcpIngress.Spec.Ports {
		if isPortInUse(service.Spec.Ports, port.ExternalPort) {
			return fmt.Errorf("port [%d] is in use by %s", port.ExternalPort, portNamePrefix)
		}
		servicePorts := corev1.ServicePort{
			Name:       fmt.Sprintf("%s%d", portNamePrefix, index+1),
			Port:       port.ExternalPort,
			TargetPort: intstr.FromInt(int(port.ExternalPort)),
		}
		service.Spec.Ports = append(service.Spec.Ports, servicePorts)
	}

	if err := r.Update(ctx, service); err != nil {
		return fmt.Errorf("failed to update service: %v", err)
	}

	return nil
}

func (r *TcpIngressReconciler) deleteConfigMapData(ctx context.Context, configMap *corev1.ConfigMap, tcpIngress poweriov1.TcpIngress) error {
	if configMap.Data == nil {
		return nil
	}

	removeConfigMapDataInTcpIngress(configMap, tcpIngress)

	if err := r.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update ingress controller tcp configmap")
	}

	return nil
}

func removeConfigMapDataInTcpIngress(configMap *corev1.ConfigMap, tcpIngress poweriov1.TcpIngress) {
	for key, val := range configMap.Data {
		if getConfigMapDataValuePrefix(val) == getServiceFullName(tcpIngress) {
			delete(configMap.Data, key)
		}
	}
}

func (r *TcpIngressReconciler) deleteServicePort(ctx context.Context, service *corev1.Service, tcpIngress poweriov1.TcpIngress) error {
	removeServicePortInTcpIngress(service, tcpIngress)

	if err := r.Update(ctx, service); err != nil {
		return fmt.Errorf("failed to update ingress controller backend service")
	}

	return nil
}

func removeServicePortInTcpIngress(service *corev1.Service, tcpIngress poweriov1.TcpIngress) {
	i := 0
	for _, port := range service.Spec.Ports {
		if !strings.HasPrefix(port.Name, getPortNamePrefix(tcpIngress)) {
			service.Spec.Ports[i] = port
			i++
		}
	}
	service.Spec.Ports = service.Spec.Ports[:i]
}

func getConfigMapDataValuePrefix(val string) string {
	return strings.Split(val, ":")[0]
}

func getServiceFullName(tcpIngress poweriov1.TcpIngress) string {
	return fmt.Sprintf("%s/%s", tcpIngress.Namespace, tcpIngress.Spec.ServiceName)
}

func getPortNamePrefix(tcpIngress poweriov1.TcpIngress) string {
	return fmt.Sprintf("%s-%s-tcp", tcpIngress.Namespace, tcpIngress.Name)
}

func isPortInUse(src []corev1.ServicePort, port int32) bool {
	for _, servicePort := range src {
		if servicePort.Port == port {
			return true
		}
	}
	return false
}

func (r *TcpIngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&poweriov1.TcpIngress{}).
		Complete(r)
}
