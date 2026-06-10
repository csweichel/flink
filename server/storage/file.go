package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type FileBackend struct {
	root string
}

func NewFileBackend(root string) *FileBackend {
	return &FileBackend{root: root}
}

func (b *FileBackend) Init(context.Context) error {
	return os.MkdirAll(b.root, 0755)
}

func (b *FileBackend) Close() error {
	return nil
}

func (b *FileBackend) Get(ctx context.Context, collection, key string) ([]byte, error) {
	full, err := b.path(collection, key)
	if err != nil {
		return nil, err
	}
	out, err := os.ReadFile(full)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return out, err
}

func (b *FileBackend) Put(ctx context.Context, collection, key string, value []byte) error {
	full, err := b.path(collection, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	return os.WriteFile(full, value, 0644)
}

func (b *FileBackend) Delete(ctx context.Context, collection, key string) error {
	full, err := b.path(collection, key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (b *FileBackend) List(ctx context.Context, collection, prefix string) ([]Entry, error) {
	dir, err := b.collectionPath(collection)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	err = filepath.WalkDir(dir, func(full string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, full)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		value, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		entries = append(entries, Entry{Key: key, Value: value})
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return entries, err
}

func (b *FileBackend) DeleteCollection(ctx context.Context, collection string) error {
	dir, err := b.collectionPath(collection)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (b *FileBackend) path(collection, key string) (string, error) {
	dir, err := b.collectionPath(collection)
	if err != nil {
		return "", err
	}
	key, err = cleanStoragePath(key)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.FromSlash(key)), nil
}

func (b *FileBackend) collectionPath(collection string) (string, error) {
	collection, err := cleanStoragePath(collection)
	if err != nil {
		return "", err
	}
	return filepath.Join(b.root, filepath.FromSlash(collection)), nil
}

func cleanStoragePath(p string) (string, error) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", nil
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", errors.New("invalid storage path")
		}
	}
	return strings.TrimPrefix(path.Clean("/"+p), "/"), nil
}
