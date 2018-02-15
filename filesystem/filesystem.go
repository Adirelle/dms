package filesystem

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
)

const RootID = "/"

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
	ID         string   `xml:"id,attr"`
	ParentID   string   `xml:"parentID,attr"`
	ChildCount int      `xml:"childCount,attr"`
	FilePath   string   `xml:"-"`
	IsRoot     bool     `xml:"-"`
	ChildrenID []string `xml:"-"`
	os.FileInfo
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
	fi, err := os.Stat(fp)
	if err != nil {
		return
	}
	if !fi.IsDir() && !fi.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	ret = &Object{
		ID:       id,
		IsRoot:   id == RootID,
		ParentID: "-1",
		FilePath: fp,
		FileInfo: fi,
	}
	if fi.IsDir() {
		err = ret.readChildren()
	}
	return
}

func (o *Object) readChildren() (err error) {
	fh, err := os.Open(o.FilePath)
	if err != nil {
		return
	}
	names, err := fh.Readdirnames(-1)
	if err != nil {
		return
	}
	o.ChildCount = len(names)
	o.ChildrenID = make([]string, o.ChildCount)
	for i, name := range names {
		o.ChildrenID[i] = path.Join(o.ID, name)
	}
	return
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
