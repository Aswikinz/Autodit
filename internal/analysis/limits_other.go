//go:build !linux

package analysis

import "errors"

func limitWorker() error { return errors.New("analysis workers require the Linux Podman runtime") }
