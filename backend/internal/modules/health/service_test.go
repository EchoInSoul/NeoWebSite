package health

import (
	"context"
	"errors"
	"testing"
	"time"

	"website/backend/internal/config"
)

type fakePinger struct {
	err   error
	delay time.Duration
}

func (f fakePinger) Ping(ctx context.Context) error {
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(f.delay):
		}
	}
	return f.err
}

func TestCheckReportsDatabaseOK(t *testing.T) {
	st := newService(config.Config{ServiceName: "s"}, fakePinger{}).Check(context.Background())
	if st.Status != "ok" || st.Database != "ok" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestCheckReportsDatabaseErrorAndDegrades(t *testing.T) {
	st := newService(config.Config{ServiceName: "s"}, fakePinger{err: errors.New("boom")}).Check(context.Background())
	if st.Status != "degraded" || st.Database != "error" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestCheckTimesOutSlowDatabase(t *testing.T) {
	start := time.Now()
	st := newService(config.Config{ServiceName: "s"}, fakePinger{delay: 10 * time.Second}).Check(context.Background())
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("check took %s, expected ~2s timeout", elapsed)
	}
	if st.Status != "degraded" || st.Database != "error" {
		t.Fatalf("unexpected status: %+v", st)
	}
}
