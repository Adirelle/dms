package filesystem

import (
	"encoding/gob"
	"errors"
	"path"
)

func init() {
	gob.Register(ID(""))
}

// ID is an "opaque" identifier for filesystem objects
type ID string

const (
	// NullID represents the nil value of object identifiers
	NullID = ID("-1")

	// RootID is the object identifier of the root of the filesystem
	RootID = ID("/")
)

func (id ID) String() string { return string(id) }
func (id ID) IsRoot() bool   { return string(id) == "/" }
func (id ID) IsNull() bool   { return string(id) == "-1" }

func (id ID) BaseName() string {
	if id.IsNull() || id.IsRoot() {
		return ""
	}
	return path.Base(string(id))
}

func (id ID) ParentID() ID {
	if id.IsNull() || id.IsRoot() {
		return NullID
	}
	p := path.Dir(string(id))
	if p == "." || p == "/" {
		return RootID
	}
	return ID(p)
}

func (id ID) ChildID(name string) ID {
	if id.IsNull() {
		return NullID
	}
	return ID(path.Join(string(id), name))
}

// ErrInvalidObjectID is returned by ParseObjectID for invalid input
var ErrInvalidObjectID = errors.New("invalid ID")

// ParseObjectID parses an ID out of a string
func ParseObjectID(s string) (ID, error) {
	if s == "0" || s == "/" {
		return RootID, nil
	} else if s == "-1" || s == "" {
		return NullID, nil
	}
	s = path.Clean(s)
	if !path.IsAbs(s) {
		return NullID, ErrInvalidObjectID
	}
	if s == "/" {
		return RootID, nil
	}
	return ID(s), nil
}
