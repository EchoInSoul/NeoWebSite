package access

import (
	"errors"
	"fmt"
	"testing"

	"website/backend/internal/common/apierror"
)

func TestMapRepoError(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"record not found", errRecordNotFound, apierror.ErrNotFound},
		{"group name taken", errGroupNameTaken, apierror.ErrGroupNameTaken},
		{"group not empty", errGroupNotEmpty, apierror.ErrGroupNotEmpty},
		{"last owner", errLastOwner, apierror.ErrLastOwner},
		{"wrapped sentinel", fmt.Errorf("query: %w", errRecordNotFound), apierror.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapRepoError(tc.in)
			if !errors.Is(got, tc.want) {
				t.Fatalf("mapRepoError(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}

	boom := errors.New("boom")
	if got := mapRepoError(boom); got != boom {
		t.Fatalf("unknown error = %v, want the original error", got)
	}
}
