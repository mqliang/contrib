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

package main

import (
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/api/prometheus"
	"github.com/spf13/pflag"
	"k8s.io/contrib/prometheus-hpa/podautoscaler"
	"k8s.io/contrib/prometheus-hpa/podautoscaler/metrics"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/rest"
)

const defaultHorizontalPodAutoscalerSyncPeriod = 30 * time.Second

var (
	flags        = pflag.NewFlagSet("", pflag.ContinueOnError)
	resyncPeriod = flags.Duration("horizontal-pod-autoscaler-sync-period", defaultHorizontalPodAutoscalerSyncPeriod,
		`The period for syncing the number of pods in horizontal pod autoscaler.`)
)

func init() {
	flag.Set("logtostderr", "true")
	flag.Parse()
	go wait.Until(glog.Flush, 10*time.Second, wait.NeverStop)
}

func main() {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(rest.AddUserAgent(config, "caicloud-horizontal-pod-autoscaler"))
	if err != nil {
		panic(err.Error())
	}

	// create prometheus metrics client
	prometheusClient, err := prometheus.New(prometheus.Config{Address: metrics.DefaultPrometheusAddress})
	if err != nil {
		panic(err.Error())
	}
	metricsClient := metrics.NewPrometheusMetricsClient(clientset, prometheus.NewQueryAPI(prometheusClient))

	hpac := podautoscaler.NewHorizontalController(clientset.Core(), clientset.Extensions(), clientset, metricsClient, *resyncPeriod)

	hpac.Run(wait.NeverStop)

	return
}
