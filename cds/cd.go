package cds

import (
	"sort"

	"github.com/anacrolix/dms/logging"

	"github.com/anacrolix/dms/filesystem"
)

// RootID is the identifier of the root of any ContentDirectory
const RootID = filesystem.RootID

// ContentDirectory is the generic ContentDirectory interface (no s**t, sherlock !).
type ContentDirectory interface {
	Get(id string) (*Object, error)
}

// FilesystemContentDirectory is a filesystem-based ContentDirectory with processors
type FilesystemContentDirectory struct {
	fs *filesystem.Filesystem
	l  logging.Logger
	processorList
}

// Processor adds information to Object
type Processor interface {
	Process(Object) error
}

// NewFilesystemContentDirectory creates FilesystemContentDirectory
func NewFilesystemContentDirectory(fs *filesystem.Filesystem, logger logging.Logger) *FilesystemContentDirectory {
	return &FilesystemContentDirectory{fs: fs, l: logger}
}

func (d *FilesystemContentDirectory) Get(id string) (obj *Object, err error) {
	fsObj, err := d.fs.Get(id)
	if err == nil {
		obj = newObject(fsObj)
	}
	return
}

func GetChildren(d ContentDirectory, id string) (children []*Object, err error) {
	obj, err := d.Get(id)
	if err != nil {
		return
	}
	children = make([]*Object, 0, len(obj.ChildrenID))
	for _, id := range obj.ChildrenID {
		if child, err := d.Get(id); err == nil {
			children = append(children, child)
		} else {
			return nil, err
		}
	}
	return
}

type processor struct {
	Processor
	priority int
}

type processorList []processor

func (pl *processorList) AddProcessor(priority int, p Processor) {
	*pl = append(*pl, processor{p, priority})
	sort.Sort(pl)
}

func (pl processorList) Len() int           { return len(pl) }
func (pl processorList) Less(i, j int) bool { return pl[i].priority > pl[j].priority }
func (pl processorList) Swap(i, j int)      { pl[j], pl[i] = pl[i], pl[j] }
