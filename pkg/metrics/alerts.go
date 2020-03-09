package metrics

import (
	"time"
	"fmt"
	"github.com/golang/glog"
)

var defaultAlertManager = NewAlertManager()

type alertManager struct {
	alerts   *chan Monitor
	monitors []Monitor

	quit chan struct{}
	done chan struct{}
}

func NewAlertManager() *alertManager {
	alerts := make(chan Monitor)
	return &alertManager{
		alerts: &alerts,
	}
}

func Alerts() *chan Monitor {
	a := defaultAlertManager
	return a.alerts
}

type Monitor struct {
	name       string
	
	eval       func() float64

	duration   int64
	
	current    float64
	threshold  float64
	

	status     string // TRIGGERED or RECOVERED
	statusTime time.Time
}

func NewMonitor(name string, duration int64, threshold float64, eval func() float64) *Monitor {
	return &Monitor{
		name: name,
		eval: eval,
		duration: duration,
		threshold: threshold,
	}
}

func (m *Monitor) String() string {
	var msg string

	if m.status == "TRIGGERED" {
		msg = fmt.Sprintf("%s generated an alert - hits = %f, triggered at time %s", m.name, m.current, m.statusTime)
	} else if m.status == "RECOVERED" {
		msg = fmt.Sprintf("%s recovered alert - hits = %f, recovered at time %s", m.name, m.current, m.statusTime)
	}

	return msg
}

func (m *Monitor) Status() string {
	return m.status
}

func RegisterMonitor(ms ...Monitor) {
	a := defaultAlertManager
	for _, m := range ms {
		if err := a.RegisterMonitor(m); err != nil {
			glog.Fatal(err)
		}
	}
}

func (a *alertManager) RegisterMonitor(m Monitor) error {
	a.monitors = append(a.monitors, m)
	return nil
}

func (a *alertManager) StartAlertManager() {

	for _, m := range a.monitors {
		go m.start(a.alerts)
	}	
}

func StartAlertManager() {
	a := defaultAlertManager
	a.StartAlertManager()
}

func (m *Monitor) start(ch *chan Monitor) {
	for {
		select {

		case <-time.After(time.Duration(m.duration) * time.Second):
			
			m.statusTime = time.Now()
			m.current = m.eval()
			
			if m.current >= m.threshold {
				m.status = "TRIGGERED"
				*ch <- *m
			} else {
				if m.status == "TRIGGERED" {
					m.status = "RECOVERED"
					*ch <- *m
				}
			}
		}
	}
}