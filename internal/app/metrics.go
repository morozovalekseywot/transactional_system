package app

import (
	"bwg_transactional_system/internal/broker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"strconv"
)

var respStatus = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "app",
		Subsystem: "queries",
		Name:      "status_counter",
	}, []string{"operation", "status"})

func accountMetrics(operation broker.Operation, status int) {
	respStatus.WithLabelValues(string(operation), strconv.Itoa(status)).Inc()
}
