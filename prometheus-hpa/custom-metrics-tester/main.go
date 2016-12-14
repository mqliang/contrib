package main

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const interval = 10 * time.Second

func main() {

	go func() {
		for {
			qos.Add(10)
			time.Sleep(interval)
		}
	}()

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", prometheus.Handler())
		http.ListenAndServe(":9102", mux)
	}()

	select {}
}
