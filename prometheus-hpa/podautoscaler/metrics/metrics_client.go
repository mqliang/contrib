/*
Copyright 2015 The Kubernetes Authors.

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

package metrics

import (
	"context"
	"fmt"
	"time"
	"strings"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/api/prometheus"
	"github.com/prometheus/common/model"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/labels"
)

const (
	DefaultPrometheusAddress    = "http://prometheus-0.kube-system.svc:9090"
	DefaultCPUUtilizationMetric = "container_cpu_usage_seconds_total"
)

// MetricsClient is an interface for getting metrics for pods.
type MetricsClient interface {
	// GetCPUUtilization returns the average utilization over all pods represented as a percent of requested CPU
	// (e.g. 70 means that an average pod uses 70% of the requested CPU)
	// and the time of generation of the oldest of utilization reports for pods.
	GetCPUUtilization(namespace string, selector labels.Selector) (*int, time.Time, error)

	// GetCustomMetric returns the average value of the given custom metrics from the
	// pods picked using the namespace and selector passed as arguments.
	GetCustomMetric(customMetricName string, namespace string, selector labels.Selector) (*float64, time.Time, error)
}

// PrometheusMetricsClient is Prometheus-based implementation of MetricsClient
type PrometheusMetricsClient struct {
	client   kubernetes.Interface
	queryAPI prometheus.QueryAPI
}

// NewPrometheusMetricsClient returns a new instance of Prometheus-based implementation of MetricsClient interface.
func NewPrometheusMetricsClient(client kubernetes.Interface, queryAPI prometheus.QueryAPI) *PrometheusMetricsClient {
	return &PrometheusMetricsClient{
		client:   client,
		queryAPI: queryAPI,
	}
}

func (h *PrometheusMetricsClient) GetCPUUtilization(namespace string, selector labels.Selector) (*int, time.Time, error) {
	avgConsumption, avgRequest, timestamp, err := h.GetCpuConsumptionAndRequestInMillis(namespace, selector)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to get CPU consumption and request: %v", err)
	}
	utilization := int((avgConsumption * 100) / avgRequest)
	glog.V(4).Infof("avg-consumption: %v", avgConsumption)
	glog.V(4).Infof("avg-request: %v", avgRequest)
	glog.V(4).Infof("utilization: %v", utilization)
	return &utilization, timestamp, nil
}

func (h *PrometheusMetricsClient) GetCpuConsumptionAndRequestInMillis(namespace string, selector labels.Selector) (avgConsumption int64,
	avgRequest int64, timestamp time.Time, err error) {

	podList, err := h.client.Core().Pods(namespace).
		List(api.ListOptions{LabelSelector: selector})

	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("failed to get pod list: %v", err)
	}
	podNames := map[string]struct{}{}
	requestSum := int64(0)
	missing := false
	for _, pod := range podList.Items {
		if pod.Status.Phase != v1.PodRunning {
			// Count only running pods.
			continue
		}

		podNames[pod.Name] = struct{}{}
		for _, container := range pod.Spec.Containers {
			if containerRequest, ok := container.Resources.Requests[v1.ResourceCPU]; ok {
				requestSum += containerRequest.MilliValue()
			} else {
				missing = true
			}
		}
	}
	if len(podNames) == 0 && len(podList.Items) > 0 {
		return 0, 0, time.Time{}, fmt.Errorf("no running pods")
	}
	if missing || requestSum == 0 {
		return 0, 0, time.Time{}, fmt.Errorf("some pods do not have request for cpu")
	}
	glog.V(4).Infof("%s %s - sum of CPU requested: %d", namespace, selector, requestSum)
	requestAvg := requestSum / int64(len(podNames))
	// Consumption is already averaged and in millis.
	consumption, timestamp, err := h.getCpuUtilizationForPods(namespace, selector, podNames)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	return consumption, requestAvg, timestamp, nil
}

func (h *PrometheusMetricsClient) getCpuUtilizationForPods(namespace string, selector labels.Selector, podNames map[string]struct{}) (consumption int64, timestamp time.Time, err error) {
	podListReg := "("
	for podName := range podNames {
		podListReg += podName + "|"
	}
	// trim the last "|"
	strings.TrimSuffix(podListReg, "|")
	podListReg += ").*"

	query := fmt.Sprintf("sum(rate({__name__='%s',namespace='%s',pod_name=~'%s'}[30m]))", DefaultCPUUtilizationMetric, namespace, podListReg)
	glog.V(4).Infof("Prometheus query: %v", query)

	ctx := context.TODO()
	result, err := h.queryAPI.Query(ctx, query, time.Now())
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to get metrics %v for pods: %v due to: %v", DefaultCPUUtilizationMetric, podListReg, err)
	}

	metrics := result.(model.Vector)
	if len(metrics) == 0 {
		return 0, time.Time{}, fmt.Errorf("cpu metrics missing for pods %s", podListReg)
	}
	glog.V(4).Infof("Prometheus metrics result: %#v", metrics)

	// we already sum in the query statement, prometheus will sum for us
	// *1000 to get the k8s MilliValue: in k8s, 1000m = 1 core
	sum := float64(metrics[0].Value) *1000

	return int64(sum / float64(len(podNames))), metrics[0].Timestamp.Time(), nil
}

// GetCustomMetric returns the average value of the given custom metric from the
// pods picked using the namespace and selector passed as arguments.
func (h *PrometheusMetricsClient) GetCustomMetric(customMetricName string, namespace string, selector labels.Selector) (*float64, time.Time, error) {
	podList, err := h.client.Core().Pods(namespace).List(api.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to get pod list: %v", err)
	}

	podNames := []string{}
	for _, pod := range podList.Items {
		if pod.Status.Phase == v1.PodPending {
			// Skip pending pods.
			continue
		}
		podNames = append(podNames, pod.Name)
	}
	if len(podNames) == 0 && len(podList.Items) > 0 {
		return nil, time.Time{}, fmt.Errorf("no running pods")
	}

	value, timestamp, err := h.getCustomMetricForPods(customMetricName, namespace, podNames)
	if err != nil {
		return nil, time.Time{}, err
	}
	return &value, timestamp, nil
}

func (h *PrometheusMetricsClient) getCustomMetricForPods(customMetricName string, namespace string, podNames []string) (float64, time.Time, error) {
	podListReg := "("
	for _, podName := range podNames {
		podListReg += podName + "|"
	}
	// trim the last "|"
	strings.TrimSuffix(podListReg, "|")
	podListReg += ").*"

	query := fmt.Sprintf("sum({__name__='%s',kubernetes_namespace='%s',kubernetes_pod_name=~'%s'})", customMetricName, namespace, podListReg)
	glog.V(4).Infof("Prometheus query: %v", query)

	ctx := context.TODO()
	result, err := h.queryAPI.Query(ctx, query, time.Now())
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to get metrics %v for pods: %v due to: %v", customMetricName, podListReg, err)
	}

	metrics := result.(model.Vector)
	if len(metrics) == 0 {
		return 0, time.Time{}, fmt.Errorf("metric %v missing for pods: %s", customMetricName, podListReg)
	}
	glog.V(4).Infof("Prometheus metrics result: %#v", metrics)

	return float64(metrics[0].Value) / float64(len(podNames)), metrics[0].Timestamp.Time(), nil
}
