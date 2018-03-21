package cache

import (
	"encoding"
	"fmt"
	"io"

	"go.uber.org/zap/buffer"

	adi_cache "github.com/Adirelle/go-libs/cache"
	"github.com/Adirelle/go-libs/logging"
	"github.com/boltdb/bolt"
	"github.com/ugorji/go/codec"
)

var bufferPool = buffer.NewPool()

// Serializer is used to (un)serialize entry keys and values.
type Serializer interface {
	SerializeKey(io.Writer, interface{})
	SerializeValue(io.Writer, interface{}) error
	UnserializeValue([]byte) (interface{}, error)
}

type boltDBStorage struct {
	*bolt.DB
	Bucket []byte
	Serializer
	logging.Logger

	len int
}

func NewBoltDBStorage(db *bolt.DB, bucket string, ser Serializer, l logging.Logger) (adi_cache.Cache, error) {
	s := &boltDBStorage{
		DB:         db,
		Bucket:     []byte(bucket),
		Serializer: ser,
		Logger:     l,
	}
	err := db.Update(func(tx *bolt.Tx) (err error) {
		b, err := tx.CreateBucketIfNotExists(s.Bucket)
		if err != nil {
			return
		}
		s.len = b.Stats().KeyN
		return
	})
	return s, err
}

func (s *boltDBStorage) Put(key, value interface{}) (err error) {
	return s.DB.Batch(func(tx *bolt.Tx) (err error) {
		bvalue := bufferPool.Get()
		defer bvalue.Free()
		err = s.SerializeValue(bvalue, value)
		if err != nil {
			return
		}

		bkey := bufferPool.Get()
		defer bkey.Free()
		s.SerializeKey(bkey, key)

		b := tx.Bucket(s.Bucket)
		err = b.Put(bkey.Bytes(), bvalue.Bytes())
		if err == nil {
			s.len = b.Stats().KeyN
		}
		return
	})
}

// Get reads directely the entry in the internal buffer.
func (s *boltDBStorage) Get(key interface{}) (value interface{}, err error) {
	err = s.View(func(tx *bolt.Tx) error {
		bkey := bufferPool.Get()
		defer bkey.Free()
		s.SerializeKey(bkey, key)

		bvalue := tx.Bucket(s.Bucket).Get(bkey.Bytes())
		if bvalue == nil {
			return adi_cache.ErrKeyNotFound
		}

		value, err = s.UnserializeValue(bvalue)
		return err
	})
	if err != nil {
		value = nil
	}
	return
}

func (s *boltDBStorage) Remove(key interface{}) bool {
	return nil == s.DB.Batch(func(tx *bolt.Tx) (err error) {
		bkey := bufferPool.Get()
		defer bkey.Free()
		s.SerializeKey(bkey, key)

		b := tx.Bucket(s.Bucket)
		err = b.Delete(bkey.Bytes())
		if err == nil {
			s.len = b.Stats().KeyN
		}
		return
	})
}

func (s *boltDBStorage) Flush() error {
	return s.DB.Sync()
}

func (s *boltDBStorage) Len() int {
	return s.len
}

type FactoryFunc func() interface{}

type codecSerializer struct {
	factory FactoryFunc
	enc     *codec.Encoder
	dec     *codec.Decoder
}

func NewCodecSerializer(f FactoryFunc) Serializer {
	var b []byte
	h := &codec.JsonHandle{}
	return &codecSerializer{
		f,
		codec.NewEncoderBytes(&b, h),
		codec.NewDecoderBytes(nil, h),
	}
}

func (s *codecSerializer) SerializeKey(w io.Writer, key interface{}) {
	if bm, ok := key.(encoding.BinaryMarshaler); ok {
		b, _ := bm.MarshalBinary()
		w.Write(b)
		return
	}
	if se, ok := key.(fmt.Stringer); ok {
		key = se.String()
	}
	w.Write([]byte(key.(string)))
}

func (s *codecSerializer) SerializeValue(w io.Writer, value interface{}) error {
	s.enc.Reset(w)
	return s.enc.Encode(value)
}

func (s *codecSerializer) UnserializeValue(data []byte) (value interface{}, err error) {
	value = s.factory()
	s.dec.ResetBytes(data)
	err = s.dec.Decode(value)
	return
}
