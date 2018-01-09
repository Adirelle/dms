package metadata

import (
	"math/rand"
	"sort"
	"sync"
	"time"
)

const INITIAL_CACHE_CAPACITY = 1000
const CACHE_TOP_SIZE = 2000
const GC_INTERVAL = 5 * time.Second

var cache map[string]*metadata
var mutex sync.RWMutex

func init() {
	cache = make(map[string]*metadata, INITIAL_CACHE_CAPACITY)
}

// GetMetadata fetchs the (possibly cached) metadata about a file entity
func GetMetadata(path string) (Metadata, error) {
	mutex.Lock()
	defer mutex.Unlock()
	meta, found := cache[path]
	if !found {
		meta = &metadata{path: path}
		cache[path] = meta
	}
	if err := meta.refresh(); err != nil {
		return nil, err
	}
	meta.lastUsed = time.Now()
	meta.usedCount++
	return meta, nil
}

// StartGC starts the period cache garbage collector
func StartGC() {
	go runGC()
}

func runGC() {
	for {
		n := len(cache) - INITIAL_CACHE_CAPACITY
		prob := float64(n) / (CACHE_TOP_SIZE - INITIAL_CACHE_CAPACITY)
		if rand.Float64() < prob {
			cleanCache(n)
		}
		time.Sleep(GC_INTERVAL)
	}
}

func cleanCache(n int) {
	mutex.Lock()
	defer mutex.Unlock()

	p := make([]string, len(cache))
	for path := range cache {
		p = append(p, path)
	}
	sort.Sort(sortableCache(p))
	for i := 0; i < n; i++ {
		delete(cache, p[i])
	}
}

type sortableCache []string

func (sc sortableCache) Len() int {
	return len(sc)
}

func (sc sortableCache) Less(i, j int) bool {
	return cache[sc[j]].weightedAge() < cache[sc[i]].weightedAge()
}

func (sc sortableCache) Swap(i, j int) {
	sc[i], sc[j] = sc[j], sc[i]
}
