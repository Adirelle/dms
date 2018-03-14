package cache

import "fmt"

func ExampleMultiLoaderSimple() {

	l := NewMultiLoader()

	var x int
	l.RegisterLoader(&x, func(k interface{}) (interface{}, error) {
		return k.(int) + 5, nil
	})
	var y string
	l.RegisterLoader(&y, func(k interface{}) (interface{}, error) {
		return k.(string) + "Bar", nil
	})

	data, ttl, err := l.Load(5)
	fmt.Printf("%v %v %v\n", data, ttl, err)

	data, ttl, err = l.Load("Foo")
	fmt.Printf("%v %v %v", data, ttl, err)

	// Output:
	// 10 <nil> <nil>
	// FooBar <nil> <nil>
}

type StringerImpl string

func (s StringerImpl) String() string {
	return string(s)
}

func ExampleMultiLoaderInterface() {

	l := NewMultiLoader()

	var x fmt.Stringer
	l.RegisterLoader(&x, func(k interface{}) (interface{}, error) {
		return k.(fmt.Stringer).String() + "Bar", nil
	})

	data, ttl, err := l.Load(StringerImpl("Foo"))
	fmt.Printf("%v %v %v\n", data, ttl, err)

	// Output:
	// FooBar <nil> <nil>
}
