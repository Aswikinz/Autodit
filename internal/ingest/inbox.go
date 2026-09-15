package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Aswikinz/Autodit/internal/domain"
)

// Landed is a complete, atomically published file with a content-addressed receipt.
type Landed struct {
	Population domain.Population
	Receipt    string
}

// Inbox reads .json files only. Producers must write to .partial then rename
// after the extract and independent controls are complete. Files are never deleted.
func Inbox(ctx context.Context, dir string) ([]Landed, error) {
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, errors.New("inbox unavailable")
	}
	out := []Landed{}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, e := entry.Info()
		if e != nil || info.Size() > 32*1024*1024 {
			return nil, errors.New("inbox file exceeds size limit")
		}
		b, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			return nil, errors.New("inbox read failed")
		}
		var p domain.Population
		if json.Unmarshal(b, &p) != nil || p.Validate() != nil {
			return nil, errors.New("inbox extraction contract invalid")
		}
		out = append(out, Landed{Population: p, Receipt: domain.Hash(append([]byte(entry.Name()+"\x00"), b...))})
		if len(out) >= 100 {
			break
		}
	}
	return out, nil
}
