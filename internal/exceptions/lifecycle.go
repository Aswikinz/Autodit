// Package exceptions owns disposition transitions and run reconciliation.
package exceptions

import (
	"errors"
	"time"
)

// State is an auditor disposition or a system-driven queue state.
type State string

const (
	Open       State = "open"
	InReview   State = "in_review"
	Accepted   State = "accepted"
	Dismissed  State = "dismissed"
	Suppressed State = "suppressed"
	Reopened   State = "reopened"
	Resolved   State = "resolved_in_source"
)

// ErrTransition rejects invalid workflow changes without losing prior history.
var ErrTransition = errors.New("invalid disposition transition")

// Transition validates human-controlled transitions. Auto-resolution is system-only.
func Transition(from, to State, reason string, expiry *time.Time, now time.Time) error {
	if len(reason) < 1 || len(reason) > 2000 {
		return ErrTransition
	}
	valid := false
	switch from {
	case Open, Reopened:
		valid = to == InReview
	case InReview:
		valid = to == Accepted || to == Dismissed || to == Suppressed
	}
	if !valid {
		return ErrTransition
	}
	if to == Suppressed && (expiry == nil || !expiry.After(now)) {
		return ErrTransition
	}
	if to != Suppressed && expiry != nil {
		return ErrTransition
	}
	return nil
}

// Reconcile preserves dispositions and returns the classification for this run.
func Reconcile(prior State, present bool, oldHash, newHash string, expiry *time.Time, now time.Time) (State, string) {
	if !present {
		switch prior {
		case Open, InReview, Reopened:
			return Resolved, "resolved_in_source"
		}
		return prior, "absent"
	}
	if prior == "" {
		return Open, "new"
	}
	if prior == Suppressed {
		if oldHash != newHash {
			return Reopened, "reopened_changed"
		}
		if expiry == nil || !expiry.After(now) {
			return Reopened, "reopened"
		}
		return Suppressed, "suppressed"
	}
	if prior == Resolved {
		return Reopened, "reopened"
	}
	return prior, "still_open"
}
