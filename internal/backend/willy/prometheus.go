package willy

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ad = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "backend_willy_api_duration_seconds",
		Help: "The duration of Willy API calls (per endpoint).",
	}, []string{"endpoint"})
)

func willyAPIDuration(e string) prometheus.Observer {
	return ad.With(prometheus.Labels{"endpoint": e})
}
