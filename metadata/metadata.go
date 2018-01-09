package metadata

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const MIMETYPE_DIRECTORY = "text/directory"

// Metadata holds informations about file entity
type Metadata interface {
	Name() string
	FilePath() string
	Size() int64
	ModTime() time.Time
	IsRoot() bool
	IsDir() bool
	Children() ([]Metadata, error)
	Parent() (Metadata, error)
	MimeType() string
}

type metadata struct {
	path     string
	size     int64
	modTime  time.Time
	mimeType string
	children []string

	lastUsed  time.Time
	usedCount int64
}

func (md *metadata) refresh() error {
	fi, err := os.Stat(md.path)
	if err != nil {
		return err
	} else if md.isFresh(fi) {
		return nil
	}
	md.modTime = fi.ModTime()
	md.size = fi.Size()

	h, err := os.Open(md.path)
	if err != nil {
		return err
	}
	defer h.Close()

	if fi.IsDir() {
		md.mimeType = MIMETYPE_DIRECTORY
		md.children, err = h.Readdirnames(-1)
	} else {
		md.children = nil
		md.mimeType, err = detectMimeType(h)
	}

	return err
}

func (md *metadata) isFresh(fi os.FileInfo) bool {
	return fi.ModTime().Equal(md.modTime) && fi.Size() == md.size
}

func (md *metadata) weightedAge() int64 {
	return time.Since(md.lastUsed).Nanoseconds() / md.usedCount
}

func detectMimeType(h *os.File) (string, error) {
	buf := make([]byte, 512)
	n, err := h.Read(buf)
	if err != nil {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
}

func (md *metadata) Name() string {
	return filepath.Base(md.path)
}

func (md *metadata) FilePath() string {
	return md.path
}

func (md *metadata) ModTime() time.Time {
	return md.modTime
}

func (md *metadata) Size() int64 {
	return md.size
}

func (md *metadata) IsRoot() bool {
	return filepath.Dir(md.path) == md.path
}

func (md *metadata) IsDir() bool {
	return md.mimeType == MIMETYPE_DIRECTORY
}

func (md *metadata) Children() ([]Metadata, error) {
	if !md.IsDir() {
		return nil, nil
	}
	children := make([]Metadata, len(md.children))
	for _, name := range md.children {
		if child, err := GetMetadata(filepath.Join(md.path, name)); err == nil {
			children = append(children, child)
		} else {
			return nil, err
		}
	}
	return children, nil
}

func (md *metadata) Parent() (Metadata, error) {
	return GetMetadata(filepath.Dir(md.path))
}

func (md *metadata) MimeType() string {
	return md.mimeType
}
