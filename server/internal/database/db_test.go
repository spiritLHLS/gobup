package database

import (
	"errors"
	"testing"
)

func TestWithRetryDetectsSQLiteLockedError(t *testing.T) {
	calls := 0
	err := WithRetry(func() error {
		calls++
		if calls == 1 {
			return errors.New("SQLITE_BUSY: database is locked")
		}
		return nil
	}, 2)
	if err != nil {
		t.Fatalf("WithRetry() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}
