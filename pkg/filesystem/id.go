package filesystem

import (
	"errors"
	"fmt"
	"path"
)

// ID is an "opaque" identifier for filesystem objects
type ID interface {
	fmt.Stringer
	BaseName() string
	IsRoot() bool
	ParentID() ID
	ChildID(name string) ID
}

const (
	// NullID represents the nil value of object identifiers
	NullID = nullID("")

	// RootID is the object identifier of the root of the filesystem
	RootID = rootID("/")
)

type nullID string

func (nullID) String() string    { return "-1" }
func (nullID) BaseName() string  { return "" }
func (nullID) IsRoot() bool      { return false }
func (nullID) ParentID() ID      { return NullID }
func (nullID) ChildID(string) ID { return NullID }

type rootID string

func (rootID) String() string         { return "/" }
func (rootID) BaseName() string       { return "/" }
func (rootID) IsRoot() bool           { return true }
func (rootID) ParentID() ID           { return NullID }
func (rootID) ChildID(name string) ID { return objID(path.Join("/", name)) }

type objID string

func (o objID) String() string   { return string(o) }
func (o objID) BaseName() string { return path.Base(string(o)) }
func (objID) IsRoot() bool       { return false }

func (o objID) ParentID() ID {
	p := path.Dir(string(o))
	if p == "." || p == "/" {
		return RootID
	}
	return objID(p)
}

func (o objID) ChildID(name string) ID {
	return objID(path.Join(string(o), name))
}

// ErrInvalidObjectID is returned by ParseObjectID for invalid input
var ErrInvalidObjectID = errors.New("invalid ID")

// ParseObjectID parses an ID out of a string
func ParseObjectID(s string) (ID, error) {
	if s == "0" {
		return RootID, nil
	} else if s == "-1" {
		return NullID, nil
	}
	s = path.Clean(s)
	if !path.IsAbs(s) {
		return NullID, ErrInvalidObjectID
	}
	if s == "/" {
		return RootID, nil
	}
	return objID(s), nil
}
