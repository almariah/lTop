package metrics

import (
	//"fmt"
)

// Point represents a single data point for a given timestamp.
type Sample struct {
	T int64
	V float64
}

// Series is a stream of data points belonging to a metric.
type PointSeries struct {
	Metric Labels
	Points []float64
	evalInterval int64
}

func (ps *PointSeries) EvalInterval() int64 {
	return ps.evalInterval
}

func NewPointSeries(labels Labels, evalInterval int64) *PointSeries {
	return  &PointSeries{
		Metric: labels,
		evalInterval: evalInterval,
	}
}

type Matrix []PointSeries

func Rate(ps PointSeries) PointSeries {
	result := NewPointSeries(ps.Metric, ps.evalInterval)
	result.Metric = ps.Metric
	result.Points = append(result.Points, 0.2)
	if len(ps.Points) == 0 {
		result.Points = append(result.Points, 0)
	}
	for i, _ := range ps.Points {
		if i == 0 {
			continue
		}
		r := (ps.Points[i] - ps.Points[i-1]) / float64(ps.evalInterval)
		result.Points = append(result.Points, r)
	}
	return *result
}

func Avg(ps PointSeries) float64 {
	var sumRate float64
	for _, p := range ps.Points {
		sumRate += p
	}
	samples := len(ps.Points)
	return sumRate / float64(samples)
}

func dim(m Matrix) int {
	var max int
	for _, s := range m {
		l := len(s.Points)
		if l > max {
			max = l
		}
	}
	return max
}

func extrapolate(m Matrix) Matrix {

	d := dim(m)

	for i, _ := range m {
		l := len(m[i].Points)
		if l < d {
			for j := 1; j <= d-l; j++ {
				m[i].Points = append([]float64{0.0}, m[i].Points...)
			}
		}
	}

	return m
}

func Sum(m Matrix) PointSeries {

	d := dim(m)
	m = extrapolate(m)

	evalInterval := m[0].EvalInterval()

	result := NewPointSeries(Labels{}, evalInterval)

	for i := 0; i < d; i++ {
		sum := 0.0
		for _, s := range m {
			sum += s.Points[i]
		}
		result.Points = append(result.Points, sum)
	}

	return *result
}

func existInList(l []string, val string) bool {

    for _, item := range l {
        if item == val {
            return true
        }
    }
    return false
}

func SumBy(m Matrix, by []string) Matrix {

	var result Matrix

	matrixMap := make(map[uint64]Matrix)

	for _, s := range m {
		
		var mergeLables Labels
		
		for _, l := range s.Metric {
			if  existInList(by, l.Name) {
				mergeLables = append(mergeLables, Label{Name: l.Name, Value: l.Value})
			}
		}
		
		hash := mergeLables.Hash()
		s.Metric = mergeLables
		
		if _, ok := matrixMap[hash]; ok {
			matrixMap[hash] = append(matrixMap[hash], s)
		} else {
			matrixMap[hash] = []PointSeries{s}
		}
	}

	for _, m := range matrixMap {
		sum := Sum(m)
		sum.Metric = m[0].Metric
		result = append(result, sum)
	}

	return result
}