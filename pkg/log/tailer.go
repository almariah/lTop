package log

import (
	"sync"
	"github.com/hpcloud/tail"
	"github.com/almariah/ltop/pkg/filter"
	"github.com/golang/glog"
)

type tailer struct {
	filter filter.Filter

	path string
	tail *tail.Tail

	posAndSizeMtx sync.Mutex

	quit chan struct{}
	done chan struct{}
}

func NewTailer(filter filter.Filter, path string) (*tailer, error) {
	
	tail, err := tail.TailFile(path, tail.Config{
		Follow: true,
		Poll:   true,
		ReOpen: true,
		Location: &tail.SeekInfo{
			//Offset: pos,
			Whence: 0,
		},
	})
	if err != nil {
		return nil, err
	}

	tailer := &tailer{
		// ability to wrap handler
		filter: filter,

		path: path,
		tail: tail,
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}

	go tailer.run()
	return tailer, nil
}

func (t *tailer) run() {

	defer func() {
		close(t.done)
	}()

	for {
		select {

		case line, ok := <-t.tail.Lines:
			if !ok {
				return
			}

			if line.Err != nil {
				glog.Error(line.Err)
			}

			if err := t.filter.HandleEntry(line.Time, line.Text); err != nil {
				glog.Error(err)
			}
		case <-t.quit:
			return
		}
	}
}

func (t *tailer) Stop() error {
	err := t.tail.Stop()
	close(t.quit)
	<-t.done
	glog.Info("Closing log file")
	return err
}