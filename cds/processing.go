package cds

import (
	"sort"

	"github.com/anacrolix/dms/logging"
)

// Processor adds information to Object
type Processor interface {
	Process(*Object)
}

// ProcessingDirectory uses processors to enrich the objects
type ProcessingDirectory struct {
	ContentDirectory
	logging.Logger
	processorList
}

// Get fetchs the Object from the underlying Directory and applies the processors to it
func (d *ProcessingDirectory) Get(id string) (obj *Object, err error) {
	obj, err = d.ContentDirectory.Get(id)
	if err != nil {
		return
	}
	for _, proc := range d.processorList {
		if err := logging.CatchPanic(func() { proc.Process(obj) }); err != nil {
			d.Warn(err)
		}
	}
	return
}

type processorList []processor

type processor struct {
	Processor
	priority int
}

func (pl *processorList) AddProcessor(priority int, p Processor) {
	*pl = append(*pl, processor{p, priority})
	sort.Sort(pl)
}

func (pl processorList) Len() int           { return len(pl) }
func (pl processorList) Less(i, j int) bool { return pl[i].priority > pl[j].priority }
func (pl processorList) Swap(i, j int)      { pl[j], pl[i] = pl[i], pl[j] }
