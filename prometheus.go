package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	globalLogger "github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
)

var oomMetric *prometheus.CounterVec
var clusterName string

func initPrometheus() {
	if isTruthy(os.Getenv("SENTRY_K8S_INTEGRATION_GKE_ENABLED")) {
		clusterName = instanceIntegrationGKE.clusterName
		globalLogger.Info().Msgf("Using cluster name: %s", clusterName)
	}

	r := prometheus.NewRegistry()

	upMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "kube_events_tracker_up",
		Help:        "Indicates if the Sentry Kubernetes integration is running (1 for running, 0 for not running)",
		ConstLabels: prometheus.Labels{"cluster": clusterName},
	})
	upMetric.Set(1)

	oomMetric = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_events_tracker_oomkilled_total",
			Help: "Total number of OOMKilled events detected",
		},
		[]string{"cluster", "namespace", "component", "area", "deployment"},
	)

	r.MustRegister(upMetric, oomMetric)

	handler := promhttp.HandlerFor(r, promhttp.HandlerOpts{})
	http.Handle("/metrics", handler)
	port := 9000
	globalLogger.Info().Msgf("Starting Prometheus server on port %d", port)

	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		globalLogger.Fatal().Msgf("Prometheus init error: %s", err)
	}
}

func updateOOMMetric(podObj *v1.Pod) {
	area, ok := podObj.Labels["app.kubernetes.io/area"]
	if !ok {
		area = "unknown"
	}
	component, ok := podObj.Labels["app.kubernetes.io/component"]
	if !ok {
		component = "unknown"
	}
	oomMetric.With(prometheus.Labels{
		"cluster":    clusterName,
		"namespace":  podObj.Namespace,
		"component":  component,
		"area":       area,
		"deployment": getOwnerName(podObj),
	}).Inc()
}

func getOwnerName(podObj *v1.Pod) string {
	if len(podObj.OwnerReferences) == 0 {
		return podObj.Name
	}
	name := "unknown"
	switch podObj.OwnerReferences[0].Kind {
	case KindReplicaset, KindJob:
		name = trimPossibleHash(podObj.OwnerReferences[0].Name)
	default:
		name = podObj.OwnerReferences[0].Name
	}
	return name
}

func trimPossibleHash(name string) string {
	parts := strings.Split(name, "-")
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return name
}
