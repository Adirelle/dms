package cds

import (
	"context"
	"sort"
	"time"

	"github.com/Adirelle/dms/pkg/filesystem"
)

// RootID is the identifier of the root of any ContentDirectory
const RootID = filesystem.RootID

// ContentDirectory is the generic ContentDirectory interface (no s**t, sherlock !).
type ContentDirectory interface {
	Get(filesystem.ID, context.Context) (*Object, error)
	GetChildren(filesystem.ID, context.Context) ([]*Object, error)
	LastModTime() time.Time
}

// FilesystemContentDirectory is a filesystem-based ContentDirectory with processors
type FilesystemContentDirectory struct {
	FS *filesystem.Filesystem
}

func (d *FilesystemContentDirectory) Get(id filesystem.ID, ctx context.Context) (obj *Object, err error) {
	select {
	case <-ctx.Done():
		err = ctx.Err()
		return
	default:
	}
	fsObj, err := d.FS.Get(id)
	if err != nil {
		return
	}
	return newObject(fsObj)
}

func (d *FilesystemContentDirectory) GetChildren(id filesystem.ID, ctx context.Context) ([]*Object, error) {
	return getChildren(d, id, ctx)
}

func (d *FilesystemContentDirectory) LastModTime() time.Time {
	return d.FS.LastModTime()
}

func getChildren(d ContentDirectory, id filesystem.ID, ctx context.Context) (children []*Object, err error) {
	parent, err := d.Get(id, ctx)
	if err != nil {
		return
	}
	childrenIDs := parent.GetChildrenID()
	for _, id := range childrenIDs {
		var child *Object
		child, err = d.Get(id, ctx)
		if err != nil {
			return
		}
		children = append(children, child)
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
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
