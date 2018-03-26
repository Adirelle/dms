package filesystem

import (
	"encoding/gob"
	"os"
	"time"
)

func init() {
	gob.Register(Object{})
	gob.Register(FileItem{})
	gob.Register(time.Time{})
}

// Object represents either an item or a container of the content directory
type Object struct {
	ID ID
	FileItem

	Name  string
	IsDir bool
	Size  int64

	ChildrenID []ID
}

type FileItem struct {
	FilePath string
	ModTime  time.Time

	isFresh     bool
	lastChecked time.Time
}

func ItemFromPath(filePath string) (item FileItem, err error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return
	}
	return ItemFromInfo(filePath, fi), nil
}

func ItemFromInfo(filePath string, fi os.FileInfo) FileItem {
	return FileItem{filePath, fi.ModTime(), true, time.Now()}
}

func (i FileItem) IsFresh() bool {
	if time.Since(i.lastChecked) > 10*time.Second {
		i.lastChecked = time.Now()
		fi, err := os.Stat(i.FilePath)
		i.isFresh = err == nil && fi.ModTime().Equal(i.ModTime)
	}
	return i.isFresh
}
