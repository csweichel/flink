package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
)

var ErrNotFound = errors.New("not found")

type Entry struct {
	Key   string
	Value []byte
}

type Backend interface {
	io.Closer
	Init(context.Context) error
	Get(context.Context, string, string) ([]byte, error)
	Put(context.Context, string, string, []byte) error
	Delete(context.Context, string, string) error
	List(context.Context, string, string) ([]Entry, error)
	DeleteCollection(context.Context, string) error
}

func Open(driver, dataDir string) (Backend, error) {
	switch driver {
	case "", "file", "files":
		return NewFileBackend(dataDir), nil
	case "bbolt", "bolt":
		return NewBoltBackend(filepath.Join(dataDir, "flink.db")), nil
	case "dynamodb":
		return nil, fmt.Errorf("dynamodb storage driver is not wired yet; implement storage.Backend with AWS SDK without changing server/api")
	case "firebase":
		return nil, fmt.Errorf("firebase storage driver is not wired yet; implement storage.Backend with Firebase SDK without changing server/api")
	default:
		return nil, fmt.Errorf("unknown storage driver %q", driver)
	}
}
