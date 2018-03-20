package cache

import (
	"time"

	adi_cache "github.com/Adirelle/go-libs/cache"
	"github.com/boltdb/bolt"
)

// Serializer is used to (un)serialize entry keys and values.
type Serializer interface {
	SerializeKey(key interface{}) []byte
	SerializeValue(value interface{}) ([]byte, error)
	UnserializeValue(data []byte) (interface{}, error)
}

type boltDBStorage struct {
	*bolt.DB
	Bucket []byte
	Serializer

	len  int
	c    chan operation
	t    *time.Ticker
	stop chan struct{}
}

type operation struct {
	Key, Value interface{}
}

func NewBoltDBStorage(db *bolt.DB, bucket string, ser Serializer) adi_cache.Cache {
	s := &boltDBStorage{
		DB:         db,
		Bucket:     []byte(bucket),
		Serializer: ser,
		c:          make(chan operation, 100),
	}
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.Bucket)
		if b != nil {
			s.len = b.Stats().KeyN
		}
		return nil
	})

	return s
}

func (s *boltDBStorage) Put(key, value interface{}) (err error) {
	op := operation{key, value}
	for err == nil {
		select {
		case s.c <- op:
			break
		default:
			err = s.Flush()
		}
	}
	return
}

// Get reads directely the entry in the internal buffer.
func (s *boltDBStorage) Get(key interface{}) (value interface{}, err error) {
	err = s.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.Bucket)
		if b == nil {
			return adi_cache.ErrKeyNotFound
		}
		buf := b.Get(s.SerializeKey(key))
		if buf == nil {
			return adi_cache.ErrKeyNotFound
		}
		value, err = s.UnserializeValue(buf)
		return err
	})
	if err != nil {
		value = nil
	}
	return
}

func (s *boltDBStorage) Remove(key interface{}) bool {
	s.Put(key, nil)
	return false
}

func (s *boltDBStorage) Flush() error {
	return s.DB.Update(func(tx *bolt.Tx) (err error) {
		b, err := tx.CreateBucketIfNotExists(s.Bucket)
		for err == nil {
			select {
			case op := <-s.c:
				if op.Value == nil {
					b.Delete(s.SerializeKey(op.Key))
				} else {
					var bv []byte
					if bv, err = s.SerializeValue(op.Value); err == nil {
						b.Put(s.SerializeKey(op.Key), bv)
					}
				}
			default:
				break
			}
		}
		s.len = b.Stats().KeyN
		return
	})
}

func (s *boltDBStorage) Len() int {
	return s.len
}

func (s *boltDBStorage) Serve() {
	s.stop = make(chan struct{})
	s.t = time.NewTicker(5 * time.Second)
	defer s.t.Stop()
	for true {
		select {
		case <-s.stop:
			return
		case <-s.t.C:
			s.Flush()
		}
	}
}

func (s *boltDBStorage) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	s.Flush()
}
