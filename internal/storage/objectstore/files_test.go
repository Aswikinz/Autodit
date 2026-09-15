package objectstore

import (
	"context"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"testing"
)

func TestImmutableSnapshot(t *testing.T) {
	t.Parallel()
	f := Files{Root: t.TempDir()}
	tenant, id := uuid.NewString(), uuid.NewString()
	records := []domain.Record{{Entity: "payment", ID: "P1", Amount: "0.1000", Currency: "USD"}}
	ref, hash, e := f.WriteSnapshot(context.Background(), tenant, id, records)
	if e != nil {
		t.Fatal(e)
	}
	ref2, hash2, e := f.WriteSnapshot(context.Background(), tenant, id, records)
	if e != nil || ref2 != ref || hash2 != hash {
		t.Fatal("retry changed snapshot")
	}
	b, e := f.ReadSnapshot(context.Background(), tenant, ref, hash)
	if e != nil || string(b[:4]) != "PAR1" {
		t.Fatal("not a valid Parquet object", e)
	}
	if _, e = f.ReadSnapshot(context.Background(), tenant, "../outside", hash); e == nil {
		t.Fatal("path traversal accepted")
	}
	if _, e = f.ReadSnapshot(context.Background(), uuid.NewString(), ref, hash); e == nil {
		t.Fatal("cross-tenant object accepted")
	}
	if e = os.WriteFile(filepath.Join(f.Root, ref), []byte("tampered"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ReadSnapshot(context.Background(), tenant, ref, hash); e == nil {
		t.Fatal("tampering undetected")
	}
	if _, _, e = f.WriteSnapshot(context.Background(), tenant, id, records); e == nil {
		t.Fatal("tampered evidence overwritten")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, e = f.WriteSnapshot(ctx, tenant, id, records); e == nil {
		t.Fatal("cancellation ignored")
	}
	if _, _, e = f.WriteSnapshot(context.Background(), "bad", id, nil); e == nil {
		t.Fatal("bad tenant accepted")
	}
	if _, _, e = f.WriteSnapshot(context.Background(), tenant, "bad", nil); e == nil {
		t.Fatal("bad snapshot identity accepted")
	}
}
