// If you are AI: This file implements a Prometheus collector that reads live state
// from the bus registry on each scrape. No tight loop, no allocations between scrapes.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

// collector implements prometheus.Collector by querying the bus registry on demand.
type collector struct {
	registry        *bus.Registry
	relays          RelayManager
	streamsDesc     *prometheus.Desc
	publishersDesc  *prometheus.Desc
	subscribersDesc *prometheus.Desc
	publishedDesc   *prometheus.Desc
	droppedDesc     *prometheus.Desc
	relayTasksDesc  *prometheus.Desc
}

// newCollector builds the collector with all metric descriptors.
func newCollector(reg *bus.Registry, relays RelayManager) *collector {
	return &collector{
		registry: reg,
		relays:   relays,
		streamsDesc: prometheus.NewDesc(
			"nonchalant_streams",
			"Number of streams currently registered.",
			nil, nil,
		),
		publishersDesc: prometheus.NewDesc(
			"nonchalant_publishers",
			"Number of streams that currently have a publisher attached.",
			nil, nil,
		),
		subscribersDesc: prometheus.NewDesc(
			"nonchalant_subscribers",
			"Number of subscribers currently attached to a given stream.",
			[]string{"app", "name"}, nil,
		),
		publishedDesc: prometheus.NewDesc(
			"nonchalant_messages_published_total",
			"Cumulative count of media messages routed through a stream's bus.",
			[]string{"app", "name"}, nil,
		),
		droppedDesc: prometheus.NewDesc(
			"nonchalant_messages_dropped_total",
			"Cumulative count of media messages dropped due to subscriber backpressure.",
			[]string{"app", "name"}, nil,
		),
		relayTasksDesc: prometheus.NewDesc(
			"nonchalant_relay_tasks",
			"Number of configured relay tasks.",
			nil, nil,
		),
	}
}

// Describe sends all metric descriptors to the channel.
func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.streamsDesc
	ch <- c.publishersDesc
	ch <- c.subscribersDesc
	ch <- c.publishedDesc
	ch <- c.droppedDesc
	ch <- c.relayTasksDesc
}

// Collect snapshots the bus registry and emits one Prometheus sample per stream.
// Allocation is bounded by stream count (small) and never blocks the media path.
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	keys := c.registry.List()

	publishers := 0
	for _, key := range keys {
		stream := c.registry.Get(key)
		if stream == nil {
			continue
		}
		if stream.HasPublisher() {
			publishers++
		}
		ch <- prometheus.MustNewConstMetric(
			c.subscribersDesc, prometheus.GaugeValue,
			float64(stream.SubscriberCount()), key.App, key.Name,
		)
		ch <- prometheus.MustNewConstMetric(
			c.publishedDesc, prometheus.CounterValue,
			float64(stream.MessagesPublished()), key.App, key.Name,
		)
		ch <- prometheus.MustNewConstMetric(
			c.droppedDesc, prometheus.CounterValue,
			float64(stream.TotalDropped()), key.App, key.Name,
		)
	}

	ch <- prometheus.MustNewConstMetric(
		c.streamsDesc, prometheus.GaugeValue, float64(len(keys)),
	)
	ch <- prometheus.MustNewConstMetric(
		c.publishersDesc, prometheus.GaugeValue, float64(publishers),
	)
	if c.relays != nil {
		ch <- prometheus.MustNewConstMetric(
			c.relayTasksDesc, prometheus.GaugeValue, float64(c.relays.TaskCount()),
		)
	}
}
