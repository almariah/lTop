package filter

import (
	"time"
	"github.com/almariah/ltop/pkg/printer"
)

type Filter interface {
	HandleEntry(time time.Time, entry string) error
	Summary(int64) printer.Summary
	RegisterMetrics()
	RegisterMonitors()
}