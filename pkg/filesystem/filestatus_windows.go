//+build windows

package filesystem

import (
	"path/filepath"
	"time"

	"github.com/Adirelle/go-libs/cache"
	"golang.org/x/sys/windows"
)

const hiddenAttributes = windows.FILE_ATTRIBUTE_HIDDEN | windows.FILE_ATTRIBUTE_SYSTEM

var hiddenCache cache.Cache

func init() {
	hiddenCache = cache.NewMemoryStorage(
		cache.Loader(doTestHiddenPath),
		cache.Expiration(time.Minute),
	)
}

func isHiddenPath(path string) (hidden bool, err error) {
	val, err := hiddenCache.Get(filepath.Clean(path))
	if err == nil {
		hidden = val.(bool)
	}
	return
}

func doTestHiddenPath(key interface{}) (res interface{}, err error) {
	path := (key.(string))
	if path == filepath.VolumeName(path)+"\\" {
		// Volumes always have the "SYSTEM" flag, so do not even test them
		return false, nil
	}
	winPath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	attrs, err := windows.GetFileAttributes(winPath)
	if err != nil {
		return
	}
	if attrs&hiddenAttributes == 0 {
		return isHiddenPath(filepath.Dir(path))
	}
	return false, nil
}

func isReadablePath(path string) (bool, error) {
	return tryToOpenPath(path)
}
