package metrics

import (
	"time"
	"sync"
	"github.com/cespare/xxhash/v2"
	//"fmt"
)

// fo handling collsion like {"foo", "bar"} and {"foob", "ar"}
const SeparatorByte byte = 255

type LabelPair struct {
	Name  *string
	Value *string
}

// details of the metric 
type Desc struct {
	name   string
	id     uint64 // hash the metric name using xxh
	help   string
	labels []string
}

func (d *Desc) String() string {
	return d.name
}

func newHash(name string) uint64 {
	xxh := xxhash.New()
	xxh.WriteString(name)
	return xxh.Sum64()
}

func NewDesc(name string, help string, labelNames []string) *Desc {
	d := &Desc{
		name:   name,
		help:   help,
		labels: labelNames,
	}
	
	d.id = newHash(name)

	return d
}

type Metric interface {
	Desc()  *Desc
	Value() float64
	Labels() Labels
}

type Collector interface {
	Describe(chan<- *Desc)
	Collect(chan<- Metric)
}

type metricWithLabelValues struct {
	values []string
	metric Metric
}

type metricMap struct {
	mtx       sync.RWMutex // Protects metrics.
	metrics   map[uint64][]metricWithLabelValues // using slice for handling of hash collision
	desc      *Desc
	newMetric func(labelValues ...string) Metric
}

func (m *metricMap) Collect(ch chan<- Metric) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	for _, metrics := range m.metrics {
		for _, metric := range metrics {
			ch <- metric.metric
		}
	}
}

func (m *metricMap) Describe(ch chan<- *Desc) {
	ch <- m.desc
}

type metricVec struct {
	*metricMap
	hashAdd     func(h uint64, s string) uint64
	hashAddByte func(h uint64, b byte) uint64
}

type Counter interface {
	Collector
	Inc()
	Add(float64)
}

type counter struct {
	val  uint64
	time time.Time
	lset Labels
	now func() time.Time
	desc *Desc
}

func (c *counter) Inc() {
	c.val += 1
}

func (c *counter) Add(v float64) {

}

func (c *counter) Desc() *Desc {
	return c.desc
}

func (c *counter) Value() float64 {
	return float64(c.val)
}

func (c *counter) Labels() Labels {
	return c.lset
}

func (c *counter) Collect(ch chan<- Metric) {
	ch <- c
}

func (c *counter) Describe(ch chan<- *Desc) {
	ch <- c.desc
}

type CounterVec struct {
	*metricVec
}

func newMetricVec(desc *Desc, newMetric func(lvs ...string) Metric) *metricVec {
	return &metricVec{
		metricMap: &metricMap{
			metrics:   map[uint64][]metricWithLabelValues{},
			desc:      desc,
			newMetric: newMetric,
		},
		hashAdd:     hashAdd,
		hashAddByte: hashAddByte,
	}
}


func NewCounterVec(name string, help string, labelNames []string) *CounterVec {
	
	desc := NewDesc(name, help, labelNames)

	return &CounterVec{
		metricVec: newMetricVec(desc, func(lvs ...string) Metric {
			var lset Labels
			for i, lv := range lvs {
				lset = append(lset, Label{
					Name: labelNames[i],
					Value: lv,
				})
			}
			result := &counter{desc: desc, lset: lset, now: time.Now}
			return result
		}),
	}
}

func LabelsEqual(a, b []string) bool {
	if len(a) != len(b) {
			return false
	}
	for i, v := range a {
			if v != b[i] {
					return false
			}
	}
	return true
}

func (v *CounterVec) WithLabelValues(lvs ...string) Counter {

	h, err := v.hashLabelValues(lvs)
	if err != nil {
		panic(err)
	}

	if metrics, ok := v.metrics[h]; ok {
		for _, metric := range metrics {
			if LabelsEqual(metric.values, lvs) {
				return metric.metric.(Counter)
			}
		}
	}

	inlinedLVs := make([]string, len(lvs))
	for i := range lvs {
		inlinedLVs[i] = lvs[i]
	}

	metric := v.newMetric(inlinedLVs...)
	v.metrics[h] = append( v.metrics[h], metricWithLabelValues{values: inlinedLVs, metric: metric})
	
	return metric.(Counter)
}

func (m *metricVec) hashLabelValues(vals []string) (uint64, error) {
	var iVals int
	var h = hashNew()
	for i := 0; i < len(m.desc.labels); i++ {
		h = m.hashAdd(h, vals[iVals])
		iVals++
		h = m.hashAddByte(h, SeparatorByte)
	}
	return h, nil
}