package main

import (
	"os"
	"os/signal"
	"syscall"
	"io"
	"github.com/spf13/cobra"
	"github.com/almariah/ltop/pkg/printer"
	"github.com/almariah/ltop/pkg/metrics"
	"github.com/almariah/ltop/pkg/log"
	httpfilter "github.com/almariah/ltop/pkg/filter/http"
	"github.com/almariah/ltop/pkg/filter"
	"time"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"flag"
	"fmt"
)

func main() {
	command := NewlTopCommand(os.Stdin, os.Stdout, os.Stderr)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}


func NewlTopCommand(in io.Reader, out, err io.Writer) *cobra.Command {

	cmds := &cobra.Command{
		Use:   "ltop",
		Short: "ltop: CLI tool for log files Monitoring",
		Long:  "ltop: CLI tool for log files Monitoring",
		Run: func(cmd *cobra.Command, args []string) {
			runApp(cmd, out)
		},
	}

	cmds.Flags().StringP("log-file", "l", "/tmp/access.log", "The path to the log file")

	cmds.Flags().StringP("filter", "f", "", "The filter name to parse the log file")
	cmds.MarkFlagRequired("filter")

	cmds.Flags().IntP("collect-interval", "c", 5, "The interval for metrics collection in seconds")

	cmds.Flags().Int64P("evaluate-interval", "e", 10, "The interval which metrics evaluated (or interpolated if needed) in seconds")

	cmds.Flags().Float64P("alert-threshold", "", 10, "The alert threshold for total number of request per second")
	cmds.Flags().Int64P("alert-evaluate-interval", "", 120, "The alert evaluation interval in second")
	
	return cmds
}

func runApp(cmd *cobra.Command, out io.Writer) {

	logFile, err := cmd.Flags().GetString("log-file")
	if err != nil {
		glog.Fatal(err)
	}

	filterName, err := cmd.Flags().GetString("filter")
	if err != nil {
		glog.Fatal(err)
	}

	evalInterval, err := cmd.Flags().GetInt64("evaluate-interval")
	if err != nil {
		glog.Fatal(err)
	}

	f, err := selectFilter(filterName)
	if err != nil {
		glog.Warning(err)
		return
	}
			
	tailer, err := log.NewTailer(f, logFile)
	if err != nil {
		panic(err)
	}

	ci, err := cmd.Flags().GetInt("collect-interval")
	if err != nil {
		glog.Fatal(err)
	}
	metrics.SetCollectInterval(ci)

	alertThreshold, err := cmd.Flags().GetFloat64("alert-threshold")
	if err != nil {
		glog.Fatal(err)
	}
	alertEvaluateInterval, err := cmd.Flags().GetInt64("alert-evaluate-interval")
	if err != nil {
		glog.Fatal(err)
	}
	httpfilter.SetAlertThreshold(alertThreshold)
	httpfilter.SetAlertEvaluateInterval(alertEvaluateInterval)

	f.RegisterMetrics()
	f.RegisterMonitors()

	p := printer.NewPrinter(out)

	go startRenderLoop(f, p, evalInterval)

	metrics.StartAlertManager()
	go metrics.Gather()

	go startAlertRenderLoop(p)

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-done
	tailer.Stop()
}

func startRenderLoop(f filter.Filter, p *printer.Printer, evalInterval int64) {
	s := f.Summary(evalInterval)
	p.Render(s)
	for {
		select {

		case <-time.After(10 * time.Second):
			s := f.Summary(evalInterval)
			p.Render(s)
		}
	}		
}


func startAlertRenderLoop(p *printer.Printer) {
	alerts := metrics.Alerts()

	for {
		select {

		case a := <- *alerts:
			p.Alert(&a)
		}   
	}	
}

func selectFilter(name string) (filter.Filter, error) {
	switch name {
	case "http-access-log":
		return httpfilter.NewHTTPAccessLogFilter(), nil
	default:
		return nil, fmt.Errorf("invalid filter name; %s", name)
	}
}