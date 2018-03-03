package cds

import (
	"context"
	"sort"

	"github.com/anacrolix/dms/filesystem"
	"github.com/anacrolix/dms/logging"
)

// Processor adds information to Object
type Processor interface {
	Process(*Object, context.Context)
}

// ProcessingDirectory uses processors to enrich the objects
type ProcessingDirectory struct {
	ContentDirectory
	logging.Logger
	processorList
}

// Get fetchs the Object from the underlying Directory and applies the processors to it
func (d *ProcessingDirectory) Get(id filesystem.ID, ctx context.Context) (obj *Object, err error) {
	obj, err = d.ContentDirectory.Get(id, ctx)
	if err != nil {
		return
	}
	l := logging.FromContext(ctx, d.Logger).Named("processor")
	for _, proc := range d.processorList {
		l.Debugf("Processing %s with %s", obj.ID, proc.Processor)
		p := logging.CatchPanic(func() {
			proc.Process(obj, ctx)
		})
		if p != nil {
			l.Error(p)
		}
	}
	return
}

// Get fetchs the Object from the underlying Directory and applies the processors to it
func (d *ProcessingDirectory) GetChildren(id filesystem.ID, ctx context.Context) ([]*Object, error) {
	return getChildren(d, id, ctx)
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
