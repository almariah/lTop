package printer

import (
	"io"
	"fmt"
	"github.com/guptarohit/asciigraph"
	"github.com/olekukonko/tablewriter"
	"github.com/almariah/ltop/pkg/metrics"
)

const (
	noticeColor = "\033[1;36m%s\033[0m"
	errorColor  = "\033[1;31m%s\033[0m"
)

type Table struct {
	Title  string
	Header []string
	Data   [][]string
} 

type Graph struct {
	Title string
	Data  []float64
}

type Summary struct {
	Tables []Table
	Graphs []Graph
}

type Printer struct {
	Out io.Writer
}

func NewPrinter(out io.Writer) *Printer {
	return &Printer{
		Out: out,
	}
}

func (p *Printer) Render(s Summary) {

	for _, t := range s.Tables {
		p.Out.Write([]byte("\n"))
		p.Out.Write([]byte(t.Title))
		p.Out.Write([]byte("\n\n"))
		table := tablewriter.NewWriter(p.Out)
		table.SetHeader(t.Header)
		table.AppendBulk(t.Data)
		table.Render()
		p.Out.Write([]byte("\n"))
	}

	for _, g := range s.Graphs {
		p.Out.Write([]byte("\n"))
		p.Out.Write([]byte(g.Title))
		p.Out.Write([]byte("\n\n"))
		graph := asciigraph.Plot(g.Data, asciigraph.Height(10))
		p.Out.Write([]byte(graph))
		p.Out.Write([]byte("\n"))
	}

}

func (p *Printer) Alert(m *metrics.Monitor) {
	status := m.Status()
	var msg string
	if status == "TRIGGERED" {
		msg = fmt.Sprintf(errorColor, m)
	} else if status == "RECOVERED" {
		msg = fmt.Sprintf(noticeColor, m)
	}
	p.Out.Write([]byte("\n"))
	p.Out.Write([]byte(msg))
	p.Out.Write([]byte("\n"))
}