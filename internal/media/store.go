package media

import (
	"context"
	"errors"
	"io"
)

var (
	ErrNotFound            = errors.New("media: not found")
	ErrRemoteUnavailable   = errors.New("media: remote unavailable")
	ErrCategoryNotAllowed  = errors.New("media: category not registered")
)

// File describes a media object on the configured remote.
type File struct {
	Path     string // remote-relative path (guildID/category/filename)
	Name     string // display filename
	Category string
}

// Store abstracts encrypted media storage (backed by rclone crypt remote).
type Store interface {
	List(ctx context.Context, guildID, category string) ([]File, error)
	Read(ctx context.Context, path string) (io.ReadCloser, error)
	Write(ctx context.Context, guildID, category, filename string, r io.Reader) error
	Mkdir(ctx context.Context, guildID, category string) error
	Ping(ctx context.Context) error
}
