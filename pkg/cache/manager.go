package cache

import (
	"github.com/Adirelle/go-libs/logging"
	bolt "github.com/coreos/bbolt"
)

type Manager struct {
	DB   *bolt.DB
	Size int
	L    logging.Logger

	storages []Storage
}

func (m *Manager) NewMemo(name string, sample interface{}, l LoaderFunc) Memo {
	s := m.NewStorage(name, sample)
	return &memo{s, l, new(SingleFlight), m.L.Named(name)}
}

func (m *Manager) NewStorage(name string, sample interface{}) Storage {
	mem := NewMapStorage()
	if m.DB == nil {
		m.storages = append(m.storages, mem)
		return mem
	}
	dbs := NewBoltDBStorage(m.DB, name, sample, m.L.Named(name))
	cbs := &CombinedStorage{mem, dbs}
	m.storages = append(m.storages, cbs)
	return cbs
}

func (m *Manager) Flush() {
	m.L.Info("flushing")
	for _, s := range m.storages {
		if fl, ok := s.(Flusher); ok {
			fl.Flush()
		}
	}
}
