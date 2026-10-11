package mail

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type stubTransport struct {
	mu      sync.Mutex
	calls   int
	results []error
	block   chan struct{}
	signal  chan struct{}
}

func newStubTransport(results ...error) *stubTransport {
	return &stubTransport{results: results, signal: make(chan struct{}, 64)}
}

func (s *stubTransport) deliver(_ context.Context, _ Message) error {
	s.mu.Lock()
	index := s.calls
	s.calls++
	var err error
	if index < len(s.results) {
		err = s.results[index]
	}
	block := s.block
	signal := s.signal
	s.mu.Unlock()

	if signal != nil {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
	if block != nil {
		<-block
	}
	return err
}

func (s *stubTransport) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func testLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	buffer := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buffer, nil)), buffer
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testMailer(t *testing.T, tr transport, backoff []time.Duration) *mailer {
	t.Helper()
	m := newMailer(tr, discardLogger())
	m.retryBackoff = backoff
	return m
}

func closeMailer(t *testing.T, m *mailer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func waitForCalls(t *testing.T, stub *stubTransport, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-stub.signal:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for delivery call %d", i+1)
		}
	}
}

func assertCallCount(t *testing.T, stub *stubTransport, want int) {
	t.Helper()
	time.Sleep(50 * time.Millisecond)
	if got := stub.callCount(); got != want {
		t.Fatalf("deliver calls = %d, want %d", got, want)
	}
}

func TestSendBlocksWhenQueueFull(t *testing.T) {
	m := newMailer(newStubTransport(), discardLogger())

	ctx := context.Background()
	for i := 0; i < queueSize; i++ {
		if err := m.Send(ctx, Message{To: "a@example.com"}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if err := m.Send(timeoutCtx, Message{To: "a@example.com"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("send on full queue = %v, want context.DeadlineExceeded", err)
	}
}

func TestSendAfterCloseReturnsErrClosed(t *testing.T) {
	m := newMailer(newStubTransport(), discardLogger())
	go m.run()
	closeMailer(t, m)

	for i := 0; i < 100; i++ {
		if err := m.Send(context.Background(), Message{To: "a@example.com"}); !errors.Is(err, ErrClosed) {
			t.Fatalf("send %d after close = %v, want ErrClosed", i, err)
		}
	}
}

func TestTemporaryErrorIsRetried(t *testing.T) {
	stub := newStubTransport(&deliveryError{err: errors.New("451 4.0.0 try later"), temporary: true}, nil)
	m := testMailer(t, stub, []time.Duration{time.Millisecond, time.Millisecond})
	go m.run()

	if err := m.Send(context.Background(), Message{To: "a@example.com"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	waitForCalls(t, stub, 2)
	assertCallCount(t, stub, 2)
}

func TestPermanentErrorIsNotRetried(t *testing.T) {
	stub := newStubTransport(&deliveryError{err: errors.New("550 5.1.1 no such user"), temporary: false})
	m := testMailer(t, stub, []time.Duration{time.Millisecond, time.Millisecond})
	go m.run()

	if err := m.Send(context.Background(), Message{To: "a@example.com"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	waitForCalls(t, stub, 1)
	assertCallCount(t, stub, 1)
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	stub := newStubTransport(
		&deliveryError{err: errors.New("451 try later"), temporary: true},
		&deliveryError{err: errors.New("451 try later"), temporary: true},
		&deliveryError{err: errors.New("451 try later"), temporary: true},
	)
	logger, buffer := testLogger(t)
	m := newMailer(stub, logger)
	m.retryBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	go m.run()

	if err := m.Send(context.Background(), Message{To: "a@example.com", Subject: "s"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	waitForCalls(t, stub, maxAttempts)
	assertCallCount(t, stub, maxAttempts)
	if output := buffer.String(); !strings.Contains(output, "level=ERROR") {
		t.Fatalf("expected an error log after giving up, got: %s", output)
	}
}

func TestCloseDrainsQueue(t *testing.T) {
	stub := newStubTransport()
	m := testMailer(t, stub, []time.Duration{time.Millisecond})
	go m.run()

	for i := 0; i < 2; i++ {
		if err := m.Send(context.Background(), Message{To: "a@example.com"}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	closeMailer(t, m)
	waitForCalls(t, stub, 2)
	if got := stub.callCount(); got != 2 {
		t.Fatalf("deliver calls = %d, want 2", got)
	}
}

func TestCloseRespectsContext(t *testing.T) {
	stub := newStubTransport()
	stub.block = make(chan struct{})
	m := testMailer(t, stub, []time.Duration{time.Millisecond})
	go m.run()

	if err := m.Send(context.Background(), Message{To: "a@example.com"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForCalls(t, stub, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := m.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close = %v, want context.DeadlineExceeded", err)
	}

	close(stub.block)
	select {
	case <-m.done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not exit after the in-flight delivery finished")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	m := newMailer(newStubTransport(), discardLogger())
	go m.run()
	closeMailer(t, m)
	closeMailer(t, m)
}

func TestConcurrentSendCloseDoesNotPanic(t *testing.T) {
	stub := newStubTransport()
	m := testMailer(t, stub, []time.Duration{time.Millisecond})
	go m.run()

	var senders sync.WaitGroup
	results := make(chan error, 80)
	for i := 0; i < 8; i++ {
		senders.Add(1)
		go func() {
			defer senders.Done()
			for j := 0; j < 10; j++ {
				results <- m.Send(context.Background(), Message{To: "a@example.com"})
			}
		}()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	closed := make(chan error, 1)
	go func() { closed <- m.Close(ctx) }()

	senders.Wait()
	close(results)
	for err := range results {
		if err == nil || errors.Is(err, ErrClosed) {
			continue
		}
		t.Fatalf("unexpected send error: %v", err)
	}
	if err := <-closed; err != nil {
		t.Fatalf("close: %v", err)
	}
}
