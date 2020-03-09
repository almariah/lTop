package metrics

import (
	"sync"
	"time"
	"fmt"
	"github.com/golang/glog"
)

const (
	// Capacity for the channel to collect metrics and descriptors.
	capMetricChan = 1000
	capDescChan   = 10
)

var defaultRegistry = NewRegistry()

type Registry struct {
	mtx                   sync.RWMutex
	collectorsByID        map[uint64]Collector // ID is a hash of the descIDs.
	seriesSet             map[uint64][]Series
	collectInterval		  int
}

func NewRegistry() *Registry {
	return &Registry{
		collectorsByID:  map[uint64]Collector{},
		seriesSet:  map[uint64][]Series{},
	}
}

func SetCollectInterval(i int) {
	r := defaultRegistry
	r.collectInterval = i
}

func Register(cs ...Collector) {
	r := defaultRegistry
	for _, c := range cs {
		if err := r.Register(c); err != nil {
			panic(err)
		}
	}
}

func (r *Registry) Register(c Collector) error {

	var (
		descChan = make(chan *Desc, capDescChan)
	)

	go func() {
		c.Describe(descChan)
		close(descChan)
	}()
	r.mtx.Lock()

	defer func() {
		r.mtx.Unlock()
	}()

	desc := <- descChan

	if _, exists := r.collectorsByID[desc.id]; exists {
		return fmt.Errorf("descriptor %s already exists with the same name", desc)
	}		

	r.collectorsByID[desc.id] = c
	
	return nil

}

func labelSetEqual(a, b Labels) bool {
	if len(a) != len(b) {
			return false
	}
	for i, v := range a {
			if v.Name != b[i].Name {
					return false
			}

			if v.Name == b[i].Name {
				if v.Value != b[i].Value {
					return false
				}
		}
	}
	return true
}


func (r *Registry) getOrCreateMemSeries(id uint64, lset Labels) *memSeries {
	if ss, ok := r.seriesSet[id]; ok {
		for _, s := range ss {
			memS := s.(*memSeries)
			if labelSetEqual(memS.lset, lset) {
				return memS
			}
		}
	}

	memS := NewMemSeries(id, lset, 10000)

	r.seriesSet[id] = append(r.seriesSet[id], memS)

	return memS
	
}

func Gather() {

	r := defaultRegistry

	if r.collectInterval == 0 {
		glog.Fatal("invalid collect interval")
	}

	for _, c := range r.collectorsByID {

		metricCh := make(chan Metric, capMetricChan)

		go func() {

			c.Collect(metricCh)

			for {
				select {
		
				case <-time.After(time.Duration(r.collectInterval) * time.Second):
					c.Collect(metricCh)
				}

			}
		}()
		
		go func() {
			for {
				select {
		
				case x := <-metricCh:
					m := r.getOrCreateMemSeries(x.Desc().id, x.Labels())
					m.Append(time.Now().Unix(), x.Value())
				}
			}
		}()

	}
}


func matchLables(a, selector Labels) bool {
	return true
}

func (r *Registry) Select(name string, selector Labels) []*memSeries {

	var result []*memSeries

	id := newHash(name)

	if ss, ok := r.seriesSet[id]; ok {

		for _, s := range ss {
			
			memS := s.(*memSeries)
			
			if matchLables(memS.lset, selector) {
				result = append(result, memS)
			}
		}
	}

	return result
}

func QueryLast(name string, selector Labels, last int64, evalInterval int64) Matrix {
	r := defaultRegistry
	return r.QueryLast(name, selector, last, evalInterval)
}

func interpolateSample(t int64, p1, p2 Sample) float64 {
	return p1.V + (float64(t) * ( (p2.V - p1.V) / float64((p2.T - p1.T)) ) )
}

func (r *Registry) QueryLast(name string, selector Labels, last int64, evalInterval int64) Matrix {

	var result Matrix

	t := time.Now().Add(-1 * time.Duration(last) * time.Second)

	mss := r.Select(name, selector)

	for _, ms := range mss {
		
		it := ms.Iterator()
		it.Seek(t.Unix())

		ps := NewPointSeries(ms.Labels(), evalInterval)

		startT, _ := it.At()

		var sB Sample
		var sA Sample
		var i int64

		for it.Next() {

			iTs := startT + (evalInterval * i)

			t, v := it.At()

			if t == iTs {
				ps.Points = append(ps.Points, v)
				i++
			} else if t < iTs {
				sB = Sample{T: t, V: v}
				continue
			} else if t > iTs {
				sA = Sample{T: t, V: v}
				vv := interpolateSample(iTs, sB, sA)
				ps.Points = append(ps.Points, vv)
				i++
			}
			
		}

		result = append(result, *ps)

	}

	return result
}