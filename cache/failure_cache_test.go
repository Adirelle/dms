package cache

import (
	"errors"
	"fmt"
	"time"

	"github.com/bluele/gcache"
)

func ExampleFailureCache() {

	clock := gcache.NewFakeClock()

	builder := gcache.New(100).Clock(clock).Simple()

	cache := FailureCache(
		builder,
		wrapSimpleLoader(func(k interface{}) (interface{}, error) {
			fmt.Println("Load", k)
			if k == nil {
				return nil, errors.New("ERROR")
			}
			return k, nil
		}),
		30*time.Second,
	)

	fmt.Println(cache.Get(5))
	fmt.Println(cache.Get(nil))

	clock.Advance(time.Second)

	fmt.Println(cache.Get(5))
	fmt.Println(cache.Get(nil))

	clock.Advance(time.Minute)

	fmt.Println(cache.Get(5))
	fmt.Println(cache.Get(nil))

	// Output:
	// Load 5
	// 5 <nil>
	// Load <nil>
	// <nil> ERROR
	// 5 <nil>
	// <nil> ERROR
	// 5 <nil>
	// Load <nil>
	// <nil> ERROR
}
