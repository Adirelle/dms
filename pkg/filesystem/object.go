package filesystem

import (
	"time"
)

// Object represents either an item or a container of the content directory
type Object struct {
	ID       ID
	FilePath string

	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time

	ChildrenID []ID
}
