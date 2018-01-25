package filesystem

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// FsConfig holds the configuration parameters
type FsConfig struct {
	// Root of the filesystem, a.k.a. directory to serve
	Root string
	// Do not list hidden files/directories ?
	IgnoreHidden bool
	// Do not list files/directories that cannot be open ?
	IgnoreUnreadable bool
}

// Filesystem abstracts the content directory.
type Filesystem interface {
	GetRootObject() (Object, error)
	GetObjectByID(id string) (Object, error)
}

// Object is an abstraction for a file or directory of the content directory
type Object interface {
	ID() string
	FilePath() string
	IsRoot() bool
	ParentID() string
	Parent() (Object, error)
	HasChildren() bool
	Children() ([]Object, error)
	os.FileInfo
}

type filesystem struct {
	FsConfig
}

// New creates a new Filesystem based on the passed configuration
func New(conf FsConfig) (fs Filesystem, err error) {
	conf.Root, err = filepath.Abs(conf.Root)
	if err == nil {
		fs = &filesystem{conf}
	}
	return
}

func (fs *filesystem) GetRootObject() (Object, error) {
	return fs.GetObjectByID("/")
}

func (fs *filesystem) GetObjectByID(id string) (ret Object, err error) {
	if id == "0" {
		id = "/"
	}
	id = path.Clean(id)
	if !path.IsAbs(id) {
		err = fmt.Errorf("Invalid object ID %q", id)
		return
	}
	o := object{id: id, fs: fs}
	o.FileInfo, err = os.Stat(o.FilePath())
	if err != nil {
		return
	}
	if o.IsDir() {
		ret, err = newDirectory(o)
	} else if o.Mode().IsRegular() {
		ret, err = newFile(o)
	} else {
		err = os.ErrNotExist
	}
	return
}

type object struct {
	id string
	fs *filesystem
	os.FileInfo
}

func (o *object) ID() string {
	return o.id
}

func (o *object) FilePath() string {
	return filepath.FromSlash(path.Join(o.fs.Root, o.id))
}

func (o *object) IsRoot() bool {
	return o.id == "/"
}

func (o *object) ParentID() string {
	if o.IsRoot() {
		return "-1"
	}
	return path.Dir(o.id)
}

func (o *object) Parent() (Object, error) {
	if o.IsRoot() {
		return nil, nil
	}
	return o.fs.GetObjectByID(o.ParentID())
}

type file struct {
	object
}

func newFile(o object) (f *file, err error) {
	return &file{o}, nil
}

func (f *file) HasChildren() bool {
	return false
}

func (f *file) Children() ([]Object, error) {
	return nil, nil
}

type directory struct {
	object
	children []string
}

func newDirectory(o object) (d *directory, err error) {
	d = &directory{object: o}
	if fh, err := os.Open(o.FilePath()); err == nil {
		d.children, err = fh.Readdirnames(-1)
	}
	return
}

func (d *directory) HasChildren() bool {
	return len(d.children) > 0
}

func (d *directory) Children() (children []Object, err error) {
	children = make([]Object, len(d.children))
	for _, name := range d.children {
		childID := path.Join(d.id, name)
		if child, err := d.fs.GetObjectByID(childID); err == nil {
			children = append(children, child)
		}
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
