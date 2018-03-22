package cache

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"go.uber.org/zap/buffer"

	adi_cache "github.com/Adirelle/go-libs/cache"
	"github.com/boltdb/bolt"
)

var bufferPool = buffer.NewPool()

type FactoryFunc func() interface{}

type boltDBStorage struct {
	db     *bolt.DB
	bucket []byte
	f      FactoryFunc
	len    int
}

func NewBoltDBStorage(db *bolt.DB, bucket string, f FactoryFunc) (adi_cache.Cache, error) {
	s := &boltDBStorage{db, []byte(bucket), f, 0}
	err := db.Update(func(tx *bolt.Tx) (err error) {
		b, err := tx.CreateBucketIfNotExists(s.bucket)
		if err != nil {
			return
		}
		s.len = b.Stats().KeyN
		return
	})
	v := f()
	gob.Register(v)
	return s, err
}

func (s *boltDBStorage) serialize(key interface{}) []byte {
	if stringer, ok := key.(fmt.Stringer); ok {
		key = stringer.String()
	}
	return []byte(key.(string))
}

func (s *boltDBStorage) Put(key, value interface{}) (err error) {
	bkey := s.serialize(key)
	bvalue := bufferPool.Get()
	defer bvalue.Free()
	err = gob.NewEncoder(bvalue).Encode(value)
	if err != nil {
		return
	}

	return s.db.Batch(func(tx *bolt.Tx) (err error) {
		b := tx.Bucket(s.bucket)
		err = b.Put(bkey, bvalue.Bytes())
		if err == nil {
			s.len = b.Stats().KeyN
		}
		return
	})
}

// Get reads directely the entry in the internal buffer.
func (s *boltDBStorage) Get(key interface{}) (value interface{}, err error) {
	bkey := s.serialize(key)
	err = s.db.View(func(tx *bolt.Tx) error {
		bvalue := tx.Bucket(s.bucket).Get(bkey)
		if bvalue == nil {
			return adi_cache.ErrKeyNotFound
		}

		value = s.f()
		return gob.NewDecoder(bytes.NewBuffer(bvalue)).Decode(value)
	})
	if err != nil {
		value = nil
	}
	return
}

func (s *boltDBStorage) Remove(key interface{}) bool {
	bkey := s.serialize(key)
	return nil == s.db.Batch(func(tx *bolt.Tx) (err error) {
		b := tx.Bucket(s.bucket)
		err = b.Delete(bkey)
		if err == nil {
			s.len = b.Stats().KeyN
		}
		return
	})
}

func (s *boltDBStorage) Flush() error {
	return s.db.Sync()
}

func (s *boltDBStorage) Len() int {
	return s.len
}

func (s *boltDBStorage) String() string {
	return fmt.Sprintf("Bolt(%q,%q)", s.db.Path(), s.bucket)
}
