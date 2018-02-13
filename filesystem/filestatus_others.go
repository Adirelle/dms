//+build !unix,!windows

package filesystem

func isHiddenPath(path string) (bool, error) {
	return false, nil
}

func isReadablePath(path string) (bool, error) {
	return tryToOpenPath(path)
}
