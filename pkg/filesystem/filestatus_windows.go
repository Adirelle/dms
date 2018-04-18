//+build windows

package filesystem

import (
	"path/filepath"

	"github.com/Adirelle/dms/pkg/cache"
	"golang.org/x/sys/windows"
)

const hiddenAttributes = windows.FILE_ATTRIBUTE_HIDDEN | windows.FILE_ATTRIBUTE_SYSTEM

var sf = cache.NewSingleFlight()

func isHiddenPath(path string) (bool, error) {
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false, err
	}
	ch := sf.Do(path, doTestHiddenPath)
	return (<-ch).(bool), nil
}

func doTestHiddenPath(key interface{}) (res interface{}, ok bool) {
	path := (key.(string))
	if path == filepath.VolumeName(path)+"\\" {
		// Volumes always have the "SYSTEM" flag, so do not even test them
		return false, true
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
		res, err = isHiddenPath(filepath.Dir(path))
		if err != nil {
			return
		}
	}
	return false, true
}

func isReadablePath(path string) (bool, error) {
	return tryToOpenPath(path)
}
