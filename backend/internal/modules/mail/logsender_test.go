package mail

import (
	"context"
	"strings"
	"testing"
	"time"

	"website/backend/internal/config"
)

func TestLogSenderWritesMailContent(t *testing.T) {
	logger, buffer := testLogger(t)
	sender := newLogSender(config.Config{SMTPFrom: "a@example.com", SMTPFromName: "NeoBlog"}, logger)
	m := newMailer(sender, logger)
	m.retryBackoff = []time.Duration{time.Millisecond}
	go m.run()

	message := Message{To: "b@example.com", Subject: "邮件配置自检", Body: Body{Text: "正文内容", HTML: "<p>正文内容</p>"}}
	if err := m.Send(context.Background(), message); err != nil {
		t.Fatalf("send: %v", err)
	}
	closeMailer(t, m)

	output := buffer.String()
	for _, want := range []string{"a@example.com", "b@example.com", "邮件配置自检", "正文内容", "to=", "subject="} {
		if !strings.Contains(output, want) {
			t.Fatalf("log output missing %q: %s", want, output)
		}
	}
}

func TestLogSenderDefaultsToLocalFrom(t *testing.T) {
	logger, buffer := testLogger(t)
	sender := newLogSender(config.Config{}, logger)
	m := newMailer(sender, logger)
	m.retryBackoff = []time.Duration{time.Millisecond}
	go m.run()

	if err := m.Send(context.Background(), Message{To: "b@example.com", Subject: "s", Body: Body{Text: "t"}}); err != nil {
		t.Fatalf("send: %v", err)
	}
	closeMailer(t, m)

	if output := buffer.String(); !strings.Contains(output, "noreply@localhost") {
		t.Fatalf("log output missing default from: %s", output)
	}
}
