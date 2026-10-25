package twofactor

import (
	"context"
	"testing"

	"github.com/alexedwards/scs/v2"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/modules/session"
)

type fakeSettings struct {
	enabled bool
	err     error
}

func (f *fakeSettings) GetBool(context.Context, string, string) (bool, error) {
	return f.enabled, f.err
}

func TestNewRejectsNilSession(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New with nil session service must panic")
		}
	}()

	_ = New(nil, nil, nil, ratelimit.New(), nil)
}

func TestNewRejectsNilSettings(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New with nil settings must panic")
		}
	}()

	_ = New(nil, session.New(nil, scs.New()), nil, ratelimit.New(), nil)
}

func TestNewAcceptsWiredDependencies(t *testing.T) {
	db := testDB(t)
	svc := New(db, session.New(nil, scs.New()), &fakeSettings{}, ratelimit.New(), []byte("0123456789abcdef0123456789abcdef"))
	if svc == nil {
		t.Fatal("New returned nil for wired dependencies")
	}
	if svc.repo == nil || svc.sessions == nil || svc.settings == nil || svc.limiter == nil {
		t.Fatal("New left a required dependency unset")
	}
}
