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
	Get(objId string) (*Object, error)
	GetChildren(objId string) ([]*Object, error)
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

func (b *FilesystemContentDirectory) Get(objId string) (obj *Object, err error) {
	fsObj, err := b.fs.Get(objId)
	if err == nil {
		obj = newObject(fsObj)
	}
	return
}

func (b *FilesystemContentDirectory) GetChildren(objId string) (children []*Object, err error) {
	fsObj, err := b.fs.Get(objId)
	if err != nil {
		return
	}
	children = make([]*Object, 0, len(fsObj.ChildrenID))
	for _, id := range fsObj.ChildrenID {
		if child, err := b.Get(id); err == nil {
			children = append(children, child)
		} else {
			b.l.Warn(err)
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
