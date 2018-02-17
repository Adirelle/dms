package cds

import (
	"sort"

	"github.com/anacrolix/dms/filesystem"
	"github.com/anacrolix/dms/logging"
	types "gopkg.in/h2non/filetype.v1/types"
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
}

// NewFilesystemContentDirectory creates FilesystemContentDirectory
func NewFilesystemContentDirectory(fs *filesystem.Filesystem, logger logging.Logger) *FilesystemContentDirectory {
	return &FilesystemContentDirectory{fs: fs, l: logger}
}

var FolderType = types.NewMIME("application/vnd.container")

func (d *FilesystemContentDirectory) Get(id string) (obj *Object, err error) {
	fsObj, err := d.fs.Get(id)
	if err != nil {
		return
	}
	obj, err = newObject(fsObj)
	if err != nil {
		return
	}
	return
}

func GetChildren(d ContentDirectory, id string) (children []*Object, err error) {
	obj, err := d.Get(id)
	if err != nil {
		return
	}
	childrenIDs := obj.GetChildrenID()
	if err != nil {
		return
	}
	children = make([]*Object, 0, len(childrenIDs))
	for _, id := range childrenIDs {
		// TODO: fetch the children asynchronously
		if child, err := d.Get(id); err == nil {
			children = append(children, child)
		} else {
			return nil, err
		}
	}
	sort.Sort(sortableObjectList(children))
	return
}

type sortableObjectList []*Object

func (l sortableObjectList) Len() int      { return len(l) }
func (l sortableObjectList) Swap(i, j int) { l[j], l[i] = l[i], l[j] }
func (l sortableObjectList) Less(i, j int) bool {
	if l[i].IsDir() != l[j].IsDir() {
		return l[i].IsDir()
	}
	return l[i].Name() < l[j].Name()
}