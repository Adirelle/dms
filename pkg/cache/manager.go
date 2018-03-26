package cache

import (
	"github.com/Adirelle/go-libs/logging"
	"github.com/boltdb/bolt"
)

type Manager struct {
	DB   *bolt.DB
	Size int
	L    logging.Logger

	storages []Storage
}

func (m *Manager) NewMemo(name string, sample interface{}, l LoaderFunc) (Memo, error) {
	s, err := m.NewStorage(name, sample)
	if err != nil {
		return nil, err
	}
	return &memo{s, l, new(singleFlight), m.L.Named(name)}, nil
}

func (m *Manager) NewStorage(name string, sample interface{}) (Storage, error) {
	mem := NewMapStorage()
	if m.DB == nil {
		m.storages = append(m.storages, mem)
		return mem, nil
	}
	dbs, err := NewBoltDBStorage(m.DB, name, sample, m.L.Named(name))
	if err != nil {
		return nil, err
	}
	cbs := &CombinedStorage{mem, dbs}
	m.storages = append(m.storages, cbs)
	return cbs, nil
}

func (m *Manager) Flush() {
	m.L.Info("flushing")
	for _, s := range m.storages {
		s.Flush()
	}
}
