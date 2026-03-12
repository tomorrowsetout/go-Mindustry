package storage

import "mdt-server/internal/config"

func NewRecorder(cfg config.StorageConfig) (Recorder, error) {
	store, err := NewStore(cfg)
	if err != nil {
		return nil, err
	}
	return store, nil
}
