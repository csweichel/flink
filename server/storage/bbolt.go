package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

type BoltBackend struct {
	path string
	db   *bolt.DB
}

func NewBoltBackend(path string) *BoltBackend {
	return &BoltBackend{path: path}
}

func (b *BoltBackend) Init(context.Context) error {
	if err := os.MkdirAll(filepath.Dir(b.path), 0755); err != nil {
		return err
	}
	db, err := bolt.Open(b.path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return err
	}
	b.db = db
	return nil
}

func (b *BoltBackend) Close() error {
	if b.db == nil {
		return nil
	}
	return b.db.Close()
}

func (b *BoltBackend) Get(ctx context.Context, collection, key string) ([]byte, error) {
	collection, key, err := cleanBucketAndKey(collection, key)
	if err != nil {
		return nil, err
	}
	var out []byte
	err = b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(collection))
		if bucket == nil {
			return ErrNotFound
		}
		value := bucket.Get([]byte(key))
		if value == nil {
			return ErrNotFound
		}
		out = append([]byte(nil), value...)
		return nil
	})
	return out, err
}

func (b *BoltBackend) Put(ctx context.Context, collection, key string, value []byte) error {
	collection, key, err := cleanBucketAndKey(collection, key)
	if err != nil {
		return err
	}
	return b.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(collection))
		if err != nil {
			return err
		}
		return bucket.Put([]byte(key), value)
	})
}

func (b *BoltBackend) Delete(ctx context.Context, collection, key string) error {
	collection, key, err := cleanBucketAndKey(collection, key)
	if err != nil {
		return err
	}
	return b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(collection))
		if bucket == nil {
			return nil
		}
		return bucket.Delete([]byte(key))
	})
}

func (b *BoltBackend) List(ctx context.Context, collection, prefix string) ([]Entry, error) {
	collection, err := cleanStoragePath(collection)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	err = b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(collection))
		if bucket == nil {
			return nil
		}
		cursor := bucket.Cursor()
		prefixBytes := []byte(prefix)
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			if len(prefixBytes) > 0 && !bytes.HasPrefix(key, prefixBytes) {
				continue
			}
			entries = append(entries, Entry{
				Key:   string(key),
				Value: append([]byte(nil), value...),
			})
		}
		return nil
	})
	return entries, err
}

func (b *BoltBackend) DeleteCollection(ctx context.Context, collection string) error {
	collection, err := cleanStoragePath(collection)
	if err != nil {
		return err
	}
	return b.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte(collection)); err != nil && !errors.Is(err, bolt.ErrBucketNotFound) {
			return err
		}
		return nil
	})
}

func cleanBucketAndKey(collection, key string) (string, string, error) {
	collection, err := cleanStoragePath(collection)
	if err != nil {
		return "", "", err
	}
	key, err = cleanStoragePath(key)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(collection) == "" || strings.TrimSpace(key) == "" {
		return "", "", errors.New("collection and key are required")
	}
	return collection, key, nil
}
