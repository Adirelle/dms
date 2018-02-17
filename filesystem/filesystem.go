package filesystem

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// RootID is the object identifier of the root of the filesystem
const RootID = "/"

// NullID represents the null value of object identifiers
const NullID = "-1"

// Config holds the configuration parameters
type Config struct {
	// Root of the filesystem, a.k.a. directory to serve
	Root string
	// Do not list hidden files/directories ?
	IgnoreHidden bool
	// Do not list files/directories that cannot be open ?
	IgnoreUnreadable bool
}

// Filesystem is the main entry point
type Filesystem struct {
	c Config
}

// Object is an abstraction for a file or directory of the content directory
type Object struct {
	ID          string `xml:"id,attr"`
	ParentID    string `xml:"parentID,attr"`
	FilePath    string `xml:"-"`
	os.FileInfo `xml:"-"`

	childrenID []string
}

// New creates a new Filesystem based on the passed configuration
func New(conf Config) (fs *Filesystem, err error) {
	conf.Root, err = filepath.Abs(conf.Root)
	if err == nil {
		fs = &Filesystem{conf}
	}
	return
}

func (fs *Filesystem) Get(id string) (ret *Object, err error) {
	id = path.Clean(id)
	if !path.IsAbs(id) {
		err = fmt.Errorf("Invalid object ID %q", id)
		return
	}
	fp := filepath.FromSlash(path.Join(fs.c.Root, id))
	accept, err := fs.filter(fp)
	if err == nil && !accept {
		err = os.ErrNotExist
	}
	if err != nil {
		return
	}
	fi, err := os.Stat(fp)
	if err != nil {
		return
	}
	if !fi.IsDir() && !fi.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	ret = &Object{
		ID:       id,
		FilePath: fp,
		FileInfo: fi,
	}
	if ret.IsRoot() {
		ret.ParentID = NullID
	} else {
		ret.ParentID = path.Dir(id)
	}
	if ret.IsDir() {
		ret.childrenID, err = fs.readChildren(id, fp)
	}
	return
}

func (fs *Filesystem) readChildren(id, dir string) (ret []string, err error) {
	fh, err := os.Open(dir)
	if err != nil {
		return
	}
	defer fh.Close()
	names, err := fh.Readdirnames(-1)
	if err != nil {
		return
	}
	ret = make([]string, 0, len(names))
	for _, name := range names {
		childPath := path.Join(dir, name)
		var accept bool
		if accept, err = fs.filter(childPath); err != nil {
			return
		} else if accept {
			ret = append(ret, path.Join(id, name))
		}
	}
	return
}

func (fs *Filesystem) filter(path string) (accept bool, err error) {
	var isHidden, isUnreadable bool
	if fs.c.IgnoreHidden {
		if isHidden, err = isHiddenPath(path); err != nil {
			return
		}
	}
	if fs.c.IgnoreUnreadable {
		if isUnreadable, err = isReadablePath(path); err != nil {
			return
		}
	}
	accept = !isHidden && !isUnreadable
	return
}

func (o *Object) IsRoot() bool {
	return o.ID == RootID
}

func (o *Object) GetChildrenID() []string {
	return o.childrenID
}

func tryToOpenPath(path string) (readable bool, err error) {
	fh, err := os.Open(path)
	fh.Close()
	readable = err == nil
	if os.IsPermission(err) {
		err = nil
	}
	return
}
