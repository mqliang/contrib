package main

import "github.com/prometheus/client_golang/prometheus"

var (
	qos = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "qos",
			Help: "qos",
		})
)

func init() {
	prometheus.MustRegister(qos)
}
