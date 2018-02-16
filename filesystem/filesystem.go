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
	ID       string `xml:"id,attr"`
	ParentID string `xml:"parentID,attr"`
	FilePath string `xml:"-"`
	os.FileInfo

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
	return
}

func (o *Object) IsRoot() bool {
	return o.ID == RootID
}

func (o *Object) GetChildrenID() (ids []string, err error) {
	if !o.IsDir() {
		return
	}
	ids = o.childrenID
	if ids != nil {
		return
	}
	fh, err := os.Open(o.FilePath)
	if err != nil {
		return
	}
	names, err := fh.Readdirnames(-1)
	if err != nil {
		return
	}
	o.childrenID = make([]string, len(names))
	for i, name := range names {
		o.childrenID[i] = path.Join(o.ID, name)
	}
	return o.childrenID, nil
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
