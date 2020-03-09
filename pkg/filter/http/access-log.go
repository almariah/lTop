package http

import (
	"time"
	"github.com/almariah/ltop/pkg/metrics"
	"github.com/almariah/ltop/pkg/printer"
	"fmt"
	"regexp"
	"bytes"
	"strconv"
	"strings"
	"github.com/golang/glog"
)

const (
	evalIntervalNumber = 60
)

var (
	alertThreshold float64
	alertEvaluateInterval int64
)

// metrics
var (

	requestCounter = metrics.NewCounterVec(
		"request_total",
		"Counter of requests broken out for each verb, section, and HTTP response code.",
		[]string{"method", "section", "status"},
	)
	
)

type HTTPAccessLogFilter struct {
	re *regexp.Regexp
	quit chan struct{}
	done chan struct{}
}

type HTTPAccessLogEntry struct {
	RemoteHost    string
	RemoteLogname string
	User          string
	Time          time.Time
	Method        string
	URI           string
	// uri before the second '/'
	Section       string
	Protocol      string
	Status        int
	BytesSent     int
	Referer       string
	UserAgent     string
	URL           string
}

func (e *HTTPAccessLogEntry) parse(entry string, re *regexp.Regexp) error {
	
	matches := re.FindStringSubmatch(entry)

	if len(matches) != 12 {
		return fmt.Errorf("could not parse line: '%s'", matches)
	}


	time, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[4])
	if err != nil {
		return fmt.Errorf("could not parse time of line: '%s'", matches)
	}

	status, err := strconv.Atoi(matches[8])
	if err != nil {
		return fmt.Errorf("could not parse status code of line: '%s'", matches)
	}

	bytesSent, err := strconv.Atoi(matches[9])
	if err != nil {
		return fmt.Errorf("could not parse bytes sent of line: '%s'", matches)
	}	

	e.RemoteHost = matches[1]
	e.RemoteLogname = matches[2]
	e.User = matches[3]
	e.Time = time
	e.Method = matches[5]
	e.URI = matches[6]
	e.Protocol = matches[7]
	e.Status = status
	e.BytesSent = bytesSent
	e.Referer = matches[10]
	e.UserAgent = matches[11]

	uriSections := strings.Split(e.URI, "/")
	parms := strings.Split(uriSections[1], "?")
	e.Section = "/" + parms[0]

	return nil
}

func NewHTTPAccessLogFilter() *HTTPAccessLogFilter {

	var buffer bytes.Buffer
	
	buffer.WriteString(`^(\S+)\s`)                    // 1) RemoteHost
 	buffer.WriteString(`(\S+)\s`)                     // Remote Logname
	buffer.WriteString(`(\S+)\s`)                     // user
	buffer.WriteString(`\[([\w:/]+\s[+\-]\d{4})\]\s`) // date
	buffer.WriteString(`"(\S+)\s?`)                   // method
	buffer.WriteString(`(\S+)?\s?`)                   // uri
	buffer.WriteString(`(\S+)?"\s`)                   // protocol
	buffer.WriteString(`(\d{3}|-)\s`)                 // status
	buffer.WriteString(`(\d+|-)\s?`)                  // bytes sent
	buffer.WriteString(`"?([^"]*)"?\s?`)              // referrer
	buffer.WriteString(`"?([^"]*)?"?$`)               // user agent
	
	re, err := regexp.Compile(buffer.String())
	if err != nil {
		glog.Fatalf("regexp: %s", err)
	}

	return &HTTPAccessLogFilter{
		re: re,
	}
}

func (f HTTPAccessLogFilter) RegisterMetrics() {
	metrics.Register(requestCounter)
}

func (f HTTPAccessLogFilter) RegisterMonitors() {

	// monitors
	var (

		highTrafficMonitor = metrics.NewMonitor(
			"High Traffic",
			alertEvaluateInterval, // deafult over two minutes
			alertThreshold,  // threshold
			func() float64 {
				l1 := []metrics.Label{}
				mt2 := metrics.QueryLast("request_total", l1, 2, 10)
				for _, s := range mt2 {
					return metrics.Avg(metrics.Rate(s))
				} 
				return 0
			},
		)

	)

	metrics.RegisterMonitor(*highTrafficMonitor)
}

func (f HTTPAccessLogFilter) HandleEntry(time time.Time, entry string) error {

	e := HTTPAccessLogEntry{}
	err := e.parse(entry, f.re)
	if err != nil {
		return err
	}

	requestCounter.WithLabelValues(e.Method, e.Section, string(e.Status)).Inc()

	return nil
}

func (f *HTTPAccessLogFilter) Summary(evalInterval int64) printer.Summary {

	var summary printer.Summary

	l1 := []metrics.Label{}

	last := evalIntervalNumber * evalInterval
	
	mt5 := metrics.QueryLast("request_total", l1, last, evalInterval)

	if len(mt5) == 0 {
		return summary
	}

	tb := printer.Table{
		Title: "request rate per second grouped by section",
		Header: []string{"section", "requests (rps)"},
	}

	sum := metrics.Sum(mt5)
	
	totalRate := metrics.Rate(sum)
	
	graph := printer.Graph{
		Title: fmt.Sprintf("%s: request_total%s for last %d seconds over %d seconds interval", time.Now(), totalRate.Metric, last, evalInterval),
		Data: totalRate.Points,
	}
	summary.Graphs = append(summary.Graphs, graph)

	currentValue := totalRate.Points[len(totalRate.Points)-1]
	tb.Data = append(tb.Data, []string{"*", fmt.Sprintf("%f", currentValue)})

	sumSections := metrics.SumBy(mt5, []string{"section"})
	for _, s := range sumSections {
		r := metrics.Rate(s)
		currentValue = r.Points[len(r.Points)-1]
		tb.Data = append(tb.Data, []string{r.Metric[0].Value, fmt.Sprintf("%f", currentValue)})
	}

	summary.Tables = append(summary.Tables, tb)
	return summary
}

func SetAlertThreshold(threshold float64) {
	alertThreshold = threshold
}

func SetAlertEvaluateInterval(interval int64) {
	alertEvaluateInterval = interval
}