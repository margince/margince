// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

func TestBackoffGrowsThenCaps(t *testing.T) {
	if got := backoff(0); got != 200*time.Millisecond {
		t.Errorf("backoff(0) = %v, want 200ms", got)
	}
	if got := backoff(2); got != 600*time.Millisecond {
		t.Errorf("backoff(2) = %v, want 600ms", got)
	}
	// Grows without bound in attempt but is capped at 2s.
	if got := backoff(100); got != 2*time.Second {
		t.Errorf("backoff(100) = %v, want the 2s cap", got)
	}
}

func TestAnAccessDeniedVersionCallNamesTheMissingPermissions(t *testing.T) {
	denied := minio.ErrorResponse{Code: "AccessDenied"}

	err := versionAccessErr(errors.New("blobstore: listing ws/"), denied)

	if !errors.Is(err, ErrVersionPermissionsMissing) {
		t.Fatalf("err = %v, want ErrVersionPermissionsMissing", err)
	}
	for _, permission := range []string{"s3:ListBucketVersions", "s3:DeleteObjectVersion"} {
		if !strings.Contains(err.Error(), permission) {
			t.Errorf("err = %q does not name %s", err, permission)
		}
	}
}

func TestAnotherVersionFailureIsNotMistakenForMissingPermissions(t *testing.T) {
	other := errors.New("connection reset")
	if err := versionAccessErr(other, minio.ErrorResponse{Code: "InternalError"}); errors.Is(err, ErrVersionPermissionsMissing) {
		t.Errorf("err = %v blames permissions for an unrelated failure", err)
	}
}
