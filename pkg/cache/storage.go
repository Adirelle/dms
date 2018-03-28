package cache

import (
	"bytes"
	"encoding"
	"encoding/gob"
	"fmt"
	"reflect"
	"sync"

	"github.com/Adirelle/go-libs/logging"

	"go.uber.org/zap/buffer"

	"github.com/boltdb/bolt"
)

var bufferPool = buffer.NewPool()

type Storage interface {
	Store(key, value interface{})
	Fetch(key interface{}) interface{}
	Delete(key interface{})
}

type Flusher interface {
	Flush()
}

type boltDBStorage struct {
	db     *bolt.DB
	bucket []byte
	t      reflect.Type
	l      logging.Logger
}

func NewBoltDBStorage(db *bolt.DB, bucket string, sample interface{}, l logging.Logger) Storage {
	s := &boltDBStorage{db, []byte(bucket), reflect.ValueOf(sample).Type(), l}
	err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(s.bucket)
		if err == nil {
			st := b.Stats()
			s.l.Infof("%d entries in bucket %q", st.KeyN, bucket)
		}
		return err
	})
	if err != nil {
		panic(err)
	}
	return s
}

func (s *boltDBStorage) serializeKey(key interface{}) (bkey []byte) {
	var err error
	switch v := key.(type) {
	case encoding.BinaryMarshaler:
		bkey, err = v.MarshalBinary()
	case encoding.TextMarshaler:
		bkey, err = v.MarshalText()
	case fmt.Stringer:
		bkey = []byte(v.String())
	case string:
		bkey = []byte(v)
	default:
		err = fmt.Errorf("Dunno how to serialize key %v", key)
	}
	if err != nil {
		panic(err)
	}
	return
}

func (s *boltDBStorage) serialize(value interface{}) (buf *buffer.Buffer) {
	buf = bufferPool.Get()
	if err := gob.NewEncoder(buf).Encode(value); err != nil {
		s.l.Error(err, value)
		buf.Free()
		buf = nil
	}
	return
}

func (s *boltDBStorage) unserialize(data []byte) (value interface{}) {
	rval := reflect.New(s.t)
	err := gob.NewDecoder(bytes.NewBuffer(data)).DecodeValue(rval)
	if err != nil {
		s.l.Error(err, data)
		return nil
	}
	return rval.Interface()
}

func (s *boltDBStorage) Store(key, value interface{}) {
	bvalue := s.serialize(value)
	if bvalue == nil {
		return
	}
	defer bvalue.Free()
	bkey := s.serializeKey(key)

	s.batch(func(tx *bolt.Tx) error {
		return tx.Bucket(s.bucket).Put(bkey, bvalue.Bytes())
	})
}

// Get reads directly the entry in the internal buffer.
func (s *boltDBStorage) Fetch(key interface{}) (value interface{}) {
	bkey := s.serializeKey(key)
	s.view(func(tx *bolt.Tx) error {
		bvalue := tx.Bucket(s.bucket).Get(bkey)
		if bvalue != nil {
			value = s.unserialize(bvalue)
		}
		return nil
	})
	return
}

func (s *boltDBStorage) Delete(key interface{}) {
	bkey := s.serializeKey(key)
	s.batch(func(tx *bolt.Tx) error {
		return tx.Bucket(s.bucket).Delete(bkey)
	})
}

func (s *boltDBStorage) view(fn func(tx *bolt.Tx) error) {
	if err := s.db.View(fn); err != nil {
		s.l.Error(err)
	}
}

func (s *boltDBStorage) batch(fn func(tx *bolt.Tx) error) {
	if err := s.db.Batch(fn); err != nil {
		s.l.Error(err)
	}
}

func (s *boltDBStorage) Flush() {
	if err := s.db.Sync(); err != nil {
		s.l.Error(err)
	}
}

type mapStorage struct {
	entries map[interface{}]interface{}
	mu      sync.RWMutex
}

func NewMapStorage() Storage {
	return &mapStorage{entries: make(map[interface{}]interface{})}
}

func (s *mapStorage) Store(key, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = value
}

func (s *mapStorage) Fetch(key interface{}) interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entries[key]
}

func (s *mapStorage) Delete(key interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
}

type CombinedStorage struct {
	FstLevel Storage
	SndLevel Storage
}

func (s *CombinedStorage) Store(key, value interface{}) {
	s.FstLevel.Store(key, value)
	s.SndLevel.Store(key, value)
}

func (s *CombinedStorage) Fetch(key interface{}) (value interface{}) {
	value = s.FstLevel.Fetch(key)
	if value == nil {
		value = s.SndLevel.Fetch(key)
		if value != nil {
			s.FstLevel.Store(key, value)
		}
	}
	return
}

func (s *CombinedStorage) Delete(key interface{}) {
	s.FstLevel.Delete(key)
	s.SndLevel.Delete(key)
}

func (s *CombinedStorage) Flush() {
	if fl, ok := s.FstLevel.(Flusher); ok {
		fl.Flush()
	}
	if fl, ok := s.SndLevel.(Flusher); ok {
		fl.Flush()
	}
}
