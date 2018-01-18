package ffmpeg

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/anacrolix/ffprobe"
)

// FFProber is used to extract metadata from media files.
type FFProber interface {
	Probe(path string) (*ffprobe.Info, error)
}

// Cache is used to stored the metadata
type Cache interface {
	Set(key interface{}, value interface{})
	Get(key interface{}) (value interface{}, ok bool)
}

// NewFFProber creates a FFPRober given the configuration parameters
func NewFFProber(noProbe bool, cache Cache) FFProber {
	if noProbe {
		return noFFProber{}
	}
	if cache != nil {
		return ffCachedProber{ffProber{}, cache}
	}
	return ffProber{}
}

type noFFProber struct{}

func (noFFProber) Probe(path string) (*ffprobe.Info, error) {
	return nil, nil
}

type ffProber struct{}

func (ffProber) Probe(path string) (info *ffprobe.Info, err error) {
	info, err = ffprobe.Run(path)
	if err == nil {
		return
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return
	}
	waitStat, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return
	}
	code := waitStat.ExitStatus()
	if runtime.GOOS == "windows" {
		if code == -1094995529 {
			err = nil
		}
	} else if code == 183 {
		err = nil
	}
	return
}

type ffCachedProber struct {
	FFProber
	cache Cache
}

type ffProbeCacheKey struct {
	Path    string
	ModTime int64
}

// FFProbeCacheItem represents a cached item, for the sake of serialization
type FFProbeCacheItem struct {
	Key   ffProbeCacheKey
	Value *ffprobe.Info
}

func (p ffCachedProber) Probe(path string) (info *ffprobe.Info, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return
	}
	key := ffProbeCacheKey{path, fi.ModTime().UnixNano()}
	if value, ok := p.cache.Get(key); ok {
		info = value.(*ffprobe.Info)
	} else {
		info, err = p.FFProber.Probe(path)
		p.cache.Set(key, info)
	}
	return
}
