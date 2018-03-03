package dic

import (
	"encoding/xml"
	"reflect"
	"testing"
)

type emptyContainer struct{}

func (emptyContainer) Has(interface{}) bool                 { return false }
func (emptyContainer) Get(interface{}) (interface{}, error) { return nil, ErrKeyNotFound }
func (emptyContainer) get(interface{}) (reflect.Value, error) {
	return reflect.ValueOf(nil), ErrKeyNotFound
}

func TestConstant(t *testing.T) {

	c := Constant(1)
	if c.ValueType() != reflect.TypeOf(1) {
		t.Errorf("ValueType returns wrong type: %s", c.ValueType())
		return
	}
	v, e := c.Provide(emptyContainer{})
	if e != nil {
		t.Errorf("ValueType returns error: %#v", e)
	}
	if v.(int) != 1 {
		t.Errorf("ValueType returns wrong value: %#v", v)
	}
}

type frozenContainer struct {
	v map[interface{}]interface{}
	t *testing.T
}

func (f *frozenContainer) Has(k interface{}) (ok bool) {
	_, ok = f.v[k]
	f.t.Logf("c.Has(%v): %v", k, ok)
	return
}

func (f *frozenContainer) Get(k interface{}) (v interface{}, err error) {
	v, ok := f.v[k]
	if !ok {
		err = ErrKeyNotFound
	}
	f.t.Logf("c.Get(%v): %#v, %v", k, v, err)
	return
}

func (f *frozenContainer) get(k interface{}) (rv reflect.Value, err error) {
	v, err := f.Get(k)
	if err == nil {
		rv = reflect.ValueOf(v)
	}
	f.t.Logf("c.get(%v): %#v, %v", k, rv, err)
	return
}

type testStruct struct {
	X     string
	Y     string
	Z     uint
	unset string
}

func TestStruct(t *testing.T) {

	sp := Struct(testStruct{})
	if sp.ValueType() != reflect.TypeOf(testStruct{}) {
		t.Errorf("ValueType returns wrong type: %s", sp.ValueType())
	}

	ctn := &frozenContainer{map[interface{}]interface{}{
		reflect.TypeOf(""): "foo",
		"Y":                "bar",
		"Z":                uint(5),
	}, t}

	val, err := sp.Provide(ctn)
	if err != nil {
		t.Errorf("Provide returns err: %s", err)
	}
	a, ok := val.(testStruct)
	if !ok {
		t.Errorf("Provide returns wrong type: %s", reflect.TypeOf(a))
		return
	}
	e := testStruct{"foo", "bar", 5, ""}
	if e != a {
		t.Errorf("Values mismatch")
	}
	t.Logf("Expected: %#v", e)
	t.Logf("Actual: %#v", a)
}

func toTest1(x xml.Name) string {
	return x.Local
}

func TestFunc(t *testing.T) {

	sp := Func(toTest1)
	if sp.ValueType() != reflect.TypeOf("") {
		t.Errorf("ValueType returns wrong type: %s", sp.ValueType())
	}

	n := xml.Name{Local: "truc"}
	ctn := &frozenContainer{map[interface{}]interface{}{
		reflect.TypeOf(n): n,
	}, t}

	val, err := sp.Provide(ctn)
	if err != nil {
		t.Errorf("Provide returns err: %s", err)
	}
	a, ok := val.(string)
	if !ok {
		t.Errorf("Provide returns wrong type: %s", reflect.TypeOf(a))
		return
	}
	e := n.Local
	if e != a {
		t.Errorf("Values mismatch")
	}
	t.Logf("Expected: %#v", "truc")
	t.Logf("Actual: %#v", "truc")
}
