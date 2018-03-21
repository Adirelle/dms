package filesystem

import (
	"os"
	"path"
	"path/filepath"
	"time"
)

// Config holds the configuration parameters
type Config struct {
	// Root of the filesystem, a.k.a. directory to serve
	Root string
}

// Filesystem is the main entry point
type Filesystem struct {
	root        string
	lastModTime time.Time
}

// New creates a new Filesystem based on the passed configuration
func New(conf Config) (fs *Filesystem, err error) {
	root, err := filepath.Abs(filepath.Clean(conf.Root))
	if err == nil {
		fs = &Filesystem{root: root}
	}
	return
}

func (fs *Filesystem) LastModTime() time.Time {
	return fs.lastModTime
}

func (fs *Filesystem) Get(id ID) (ret *Object, err error) {
	fp := filepath.Join(fs.root, filepath.FromSlash(id.String()))
	accept, err := fs.filter(fp)
	if err == nil && !accept {
		err = os.ErrNotExist
	} else if err != nil {
		return
	}
	fi, err := os.Stat(fp)
	if err != nil {
		return
	}
	if fi.ModTime().After(fs.lastModTime) {
		fs.lastModTime = fi.ModTime()
	}
	if !fi.IsDir() && !fi.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	ret = &Object{
		ID:       id,
		FilePath: fp,
		Name:     fi.Name(),
		IsDir:    fi.IsDir(),
		Size:     fi.Size(),
		ModTime:  fi.ModTime(),
	}
	if ret.IsDir {
		ret.ChildrenID, err = fs.readChildren(id, fp)
	}
	return
}

func (fs *Filesystem) readChildren(id ID, dir string) (ret []ID, err error) {
	fh, err := os.Open(dir)
	if err != nil {
		return
	}
	defer fh.Close()
	names, err := fh.Readdirnames(-1)
	if err != nil {
		return
	}
	ret = make([]ID, 0, len(names))
	for _, name := range names {
		childPath := path.Join(dir, name)
		var accept bool
		if accept, err = fs.filter(childPath); err != nil {
			return
		} else if accept {
			ret = append(ret, id.ChildID(name))
		}
	}
	return
}

func (fs *Filesystem) filter(path string) (accept bool, err error) {
	accept = true
	if isHidden, err := isHiddenPath(path); err != nil {
		return false, err
	} else if isHidden {
		accept = false
	}
	if isReadable, err := isReadablePath(path); err != nil {
		return false, err
	} else if !isReadable {
		accept = false
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
