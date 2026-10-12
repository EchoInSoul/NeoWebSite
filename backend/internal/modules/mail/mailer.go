package mail

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"website/backend/internal/config"
)

const (
	queueSize   = 32
	sendTimeout = 20 * time.Second
	maxAttempts = 3
)

var ErrClosed = errors.New("mailer closed")

type Body struct {
	Text string
	HTML string
}

type Message struct {
	To      string
	Subject string
	Body    Body
}

type Mailer interface {
	Send(ctx context.Context, msg Message) error
	SendSync(ctx context.Context, msg Message) error
	Close(ctx context.Context) error
}

type transport interface {
	deliver(ctx context.Context, msg Message) error
}

type deliveryError struct {
	err       error
	temporary bool
}

func (e *deliveryError) Error() string {
	return e.err.Error()
}

func (e *deliveryError) Unwrap() error {
	return e.err
}

type mailer struct {
	queue        chan Message
	transport    transport
	logger       *slog.Logger
	closing      chan struct{}
	done         chan struct{}
	closeOnce    sync.Once
	retryBackoff []time.Duration
}

func newMailer(tr transport, logger *slog.Logger) *mailer {
	return &mailer{
		queue:        make(chan Message, queueSize),
		transport:    tr,
		logger:       logger,
		closing:      make(chan struct{}),
		done:         make(chan struct{}),
		retryBackoff: []time.Duration{time.Second, 5 * time.Second},
	}
}

func newTransport(cfg config.Config, logger *slog.Logger) transport {
	if cfg.SMTPHost == "" {
		return newLogSender(cfg, logger)
	}
	return newSMTPTransport(cfg)
}

func (m *mailer) Send(ctx context.Context, msg Message) error {
	select {
	case <-m.closing:
		return ErrClosed
	default:
	}

	select {
	case m.queue <- msg:
		return nil
	case <-m.closing:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *mailer) SendSync(ctx context.Context, msg Message) error {
	select {
	case <-m.closing:
		return ErrClosed
	default:
	}

	deliverCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	err := m.transport.deliver(deliverCtx, msg)
	if err != nil {
		m.logger.Error("mail delivery failed", "to", msg.To, "subject", msg.Subject, "attempt", 1, "err", err)
		return err
	}
	return nil
}

func (m *mailer) Close(ctx context.Context) error {
	m.closeOnce.Do(func() { close(m.closing) })

	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *mailer) run() {
	for {
		select {
		case msg := <-m.queue:
			m.deliverWithRetry(msg)
		case <-m.closing:
			m.drain()
			return
		}
	}
}

func (m *mailer) drain() {
	for {
		select {
		case msg := <-m.queue:
			m.deliverWithRetry(msg)
		default:
			close(m.done)
			return
		}
	}
}

func (m *mailer) deliverWithRetry(msg Message) {
	for attempt := 1; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		err := m.transport.deliver(ctx, msg)
		cancel()

		if err == nil {
			return
		}

		var delivery *deliveryError
		retryable := errors.As(err, &delivery) && delivery.temporary

		if !retryable || attempt >= maxAttempts {
			m.logger.Error("mail delivery failed", "to", msg.To, "subject", msg.Subject, "attempt", attempt, "err", err)
			return
		}

		backoff := m.retryBackoff[min(attempt-1, len(m.retryBackoff)-1)]
		m.logger.Warn("mail delivery retrying", "to", msg.To, "subject", msg.Subject, "attempt", attempt, "backoff", backoff)
		time.Sleep(backoff)
	}
}
