package metrics

import (
	"sync"
	"github.com/almariah/ltop/chunkenc"
	"math"
	"time"
	//"fmt"
) 

/*
type Appender interface {
	Append(int64, float64)
}
*/	

type Series interface {
	// Labels returns the complete set of labels identifying the series.
	Labels() Labels
	// Iterator returns a new iterator of the data of the series.
	Iterator() Iterator
	// Appender returns an appender to append samples to the series.
	//Appender() (Appender, error)
	// NumSamples returns the number of samples in the series.
	NumSamples() int
}

type Iterator interface {
	// Next advances the iterator by one.
	Next() bool
	// Seek advances the iterator forward to the first sample with the timestamp equal or greater than t.
	// If current sample found by previous `Next` or `Seek` operation already has this property, Seek has no effect.
	// Seek returns true, if such sample exists, false otherwise.
	// Iterator is exhausted when the Seek returns false.
	Seek(t int64) bool
	// At returns the current timestamp/value pair.
	// Before the iterator has advanced At behaviour is unspecified.
	At() (int64, float64)
	// Err returns the current error. It should be used only after iterator is
	// exhausted, that is `Next` or `Seek` returns false.
	Err() error
}

type sample struct {
	t int64
	v float64
}

type memChunk struct {
	chunk            chunkenc.XORChunk
	minTime, maxTime int64
}

type memSeries struct {
	sync.Mutex

	ref          uint64
	lset         Labels
	chunks       []*memChunk
	headChunk    *memChunk
	chunkRange   int64
	firstChunkID int

	nextAt        int64 // Timestamp at which to cut the next chunk.
	sampleBuf     [4]sample

	app chunkenc.Appender // Current appender for the chunk.

	mint, maxt int64
}

func NewMemSeries(id uint64, lset Labels, chunkRange int64) *memSeries {
	return &memSeries{
		ref: id,
		lset: lset,
		chunkRange: chunkRange,
		mint: time.Now().Unix(),
	}
}

func rangeForTimestamp(t int64, width int64) (maxt int64) {
	return (t/width)*width + width
}

func computeChunkEndTime(start, cur, max int64) int64 {
	a := (max - start) / ((cur - start + 1) * 4)
	if a == 0 {
		return max
	}
	return start + (max-start)/a
}

// The memSeries has a compactable range when the head time range is 1.5 times the chunk range.
// The 0.5 acts as a buffer of the appendable window.
func (s *memSeries) truncatable() int64 {
	var before int64
	if s.maxt-s.mint > s.chunkRange/2*3 {
		before = (s.maxt-s.mint)/3*2
	}
	return before
}

// truncateChunksBefore removes all chunks from the series that have not timestamp
// at or after mint. Chunk IDs remain unchanged.
func (s *memSeries) truncateChunksBefore(mint int64) (removed int) {
	var k int
	for i, c := range s.chunks {
		if c.maxTime >= mint {
			break
		}
		k = i + 1
	}
	s.chunks = append(s.chunks[:0], s.chunks[k:]...)
	s.firstChunkID += k
	if len(s.chunks) == 0 {
		s.headChunk = nil
	} else {
		s.headChunk = s.chunks[len(s.chunks)-1]
	}

	return k
}

func (s *memSeries) cut(mint int64) *memChunk {
	c := &memChunk{
		chunk:   *chunkenc.NewXORChunk(),
		minTime: mint,
		maxTime: math.MinInt64,
	}
	s.chunks = append(s.chunks, c)
	s.headChunk = c

	// Remove exceeding capacity from the previous chunk byte slice to save memory.
	if l := len(s.chunks); l > 1 {
		s.chunks[l-2].chunk.Compact()
	}

	// Set upper bound on when the next chunk must be started. An earlier timestamp
	// may be chosen dynamically at a later point.
	s.nextAt = rangeForTimestamp(mint, s.chunkRange)

	app, err := c.chunk.Appender()
	if err != nil {
		panic(err)
	}
	s.app = app
	return c
}

func (s *memSeries) Append(t int64, v float64) (success, chunkCreated bool) {
	// Based on Gorilla white papers this offers near-optimal compression ratio
	// so anything bigger that this has diminishing returns and increases
	// the time range within which we have to decompress all samples.
	const samplesPerChunk = 120

	c := s.headChunk

	if c == nil {
		c = s.cut(t)
		chunkCreated = true
	}
	numSamples := c.chunk.NumSamples()

	// Out of order sample.
	if c.maxTime >= t {
		return false, chunkCreated
	}
	// If we reach 25% of a chunk's desired sample count, set a definitive time
	// at which to start the next chunk.
	// At latest it must happen at the timestamp set when the chunk was cut.
	if numSamples == samplesPerChunk/4 {
		s.nextAt = computeChunkEndTime(c.minTime, c.maxTime, s.nextAt)
	}
	if t >= s.nextAt {
		c = s.cut(t)
		chunkCreated = true
	}
	s.app.Append(t, v)

	c.maxTime = t

	s.maxt = t

	s.sampleBuf[0] = s.sampleBuf[1]
	s.sampleBuf[1] = s.sampleBuf[2]
	s.sampleBuf[2] = s.sampleBuf[3]
	s.sampleBuf[3] = sample{t: t, v: v}

	if before := s.truncatable(); before > 0 {
		s.truncateChunksBefore(before)
	}

	return true, chunkCreated
}

func (s memSeries) Labels() Labels {
	return s.lset
}

func (s memSeries) NumSamples() int {

	var n int

	for _, chunk := range s.chunks {
		n += chunk.chunk.NumSamples()  
	}

	return n + s.headChunk.chunk.NumSamples()  
}

func (s *memSeries) Iterator() Iterator {
	return newMemSeriesIterator(s.chunks, s.mint, s.maxt)
}

func newMemSeriesIterator(cs []*memChunk, mint, maxt int64) *memSeriesIterator {
	it := &memSeriesIterator{
		chunks: cs,
		i:      0,

		mint: mint,
		maxt: maxt,
	}
	it.cur = it.chunks[it.i].chunk.Iterator(it.cur)

	return it
}

// chunkSeriesIterator implements a series iterator on top
// of a list of time-sorted, non-overlapping chunks.
type memSeriesIterator struct {
	chunks []*memChunk

	i          int
	cur        chunkenc.Iterator

	maxt, mint int64 // max and min of all chunks (series)

	total int
	buf   [4]sample

}

func (it *memSeriesIterator) Seek(t int64) (ok bool) {
	if t > it.maxt {
		return false
	}

	// Seek to the first valid value after t.
	if t < it.mint {
		t = it.mint
	}

	for ; it.chunks[it.i].maxTime < t; it.i++ {
		if it.i == len(it.chunks)-1 {
			return false
		}
	}

	it.cur = it.chunks[it.i].chunk.Iterator(it.cur)

	for it.cur.Next() {
		t0, _ := it.cur.At()
		if t0 >= t {
			return true
		}
	}
	return false
}

func (it *memSeriesIterator) At() (t int64, v float64) {
	return it.cur.At()
}

func (it *memSeriesIterator) Err() error {
	return it.cur.Err()
}

func (it *memSeriesIterator) Next() bool {
	if it.cur.Next() {
		t, _ := it.cur.At()

		if t < it.mint {
			if !it.Seek(it.mint) {
				return false
			}
			t, _ = it.At()

			return t <= it.maxt
		}
		if t > it.maxt {
			return false
		}
		return true
	}
	if err := it.cur.Err(); err != nil {
		return false
	}
	if it.i == len(it.chunks)-1 {
		return false
	}

	it.i++
	it.cur = it.chunks[it.i].chunk.Iterator(it.cur)

	return it.Next()
}