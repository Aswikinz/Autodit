// Package objectstore provides immutable, content-verified snapshot storage on a persistent volume.
package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/google/uuid"
	"github.com/parquet-go/parquet-go"
)

// Files is an append-only object store suitable for a local data volume or NAS.
type Files struct{ Root string }

// WriteSnapshot commits an immutable Parquet file via atomic rename after fsync.
func (f Files) WriteSnapshot(ctx context.Context, tenant, id string, records []domain.Record) (string, string, error) {
	if _, e := uuid.Parse(tenant); e != nil {
		return "", "", domain.ErrInvalid
	}
	if _, e := uuid.Parse(id); e != nil {
		return "", "", domain.ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return "", "", e
	}
	var buf bytes.Buffer
	w := parquet.NewGenericWriter[domain.Record](&buf)
	if _, e := w.Write(records); e != nil {
		return "", "", errors.New("snapshot encode failed")
	}
	if e := w.Close(); e != nil {
		return "", "", errors.New("snapshot close failed")
	}
	hash := domain.Hash(buf.Bytes())
	dir := filepath.Join(f.Root, tenant)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", "", errors.New("snapshot directory unavailable")
	}
	ref := tenant + "/" + id + "-" + hash + ".parquet"
	dest := filepath.Join(f.Root, filepath.FromSlash(ref))
	existing, e := os.ReadFile(dest)
	if e == nil {
		if domain.Hash(existing) != hash {
			return "", "", errors.New("snapshot integrity failure")
		}
		return ref, hash, nil
	}
	if !os.IsNotExist(e) {
		return "", "", e
	}
	tmp, e := os.CreateTemp(dir, ".snapshot-")
	if e != nil {
		return "", "", e
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	if _, e = io.Copy(tmp, &buf); e != nil {
		return "", "", e
	}
	if e = tmp.Sync(); e != nil {
		return "", "", e
	}
	if e = tmp.Close(); e != nil {
		return "", "", e
	}
	if e = ctx.Err(); e != nil {
		return "", "", e
	}
	// Link fails if the destination already exists. No operation overwrites evidence.
	if e = os.Link(tmp.Name(), dest); e != nil {
		existing, readErr := os.ReadFile(dest)
		if readErr != nil || domain.Hash(existing) != hash {
			return "", "", errors.New("snapshot commit failed")
		}
	}
	return ref, hash, nil
}

// ReadSnapshot validates the object reference and its stored checksum before replay.
func (f Files) ReadSnapshot(ctx context.Context, tenant, ref, hash string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if _, e := uuid.Parse(tenant); e != nil {
		return nil, domain.ErrInvalid
	}
	if filepath.ToSlash(filepath.Clean(ref)) != ref || filepath.IsAbs(ref) || filepath.Dir(filepath.FromSlash(ref)) != tenant {
		return nil, domain.ErrInvalid
	}
	b, e := os.ReadFile(filepath.Join(f.Root, filepath.FromSlash(ref)))
	if e != nil {
		return nil, errors.New("snapshot unavailable")
	}
	if domain.Hash(b) != hash {
		return nil, errors.New("snapshot integrity failure")
	}
	return b, nil
}
