package mail

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	smtpserver "github.com/emersion/go-smtp"
	gomail "github.com/wneessen/go-mail"

	"website/backend/internal/config"
)

type testBackend struct {
	mu      sync.Mutex
	from    string
	to      []string
	data    []byte
	dataErr error
}

func (b *testBackend) NewSession(_ *smtpserver.Conn) (smtpserver.Session, error) {
	return &testSession{backend: b}, nil
}

func (b *testBackend) snapshot() (string, []string, []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.from, append([]string(nil), b.to...), append([]byte(nil), b.data...)
}

type testSession struct {
	backend *testBackend
}

func (s *testSession) Reset() {}

func (s *testSession) Logout() error { return nil }

func (s *testSession) Mail(from string, _ *smtpserver.MailOptions) error {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	s.backend.from = from
	return nil
}

func (s *testSession) Rcpt(to string, _ *smtpserver.RcptOptions) error {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	s.backend.to = append(s.backend.to, to)
	return nil
}

func (s *testSession) Data(reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	if s.backend.dataErr != nil {
		return s.backend.dataErr
	}
	s.backend.data = data
	return nil
}

func testCertificate(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func startTestServer(t *testing.T, backend *testBackend) string {
	t.Helper()

	server := smtpserver.NewServer(backend)
	server.Domain = "127.0.0.1"
	server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{testCertificate(t)}}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	served := make(chan struct{})
	go func() {
		_ = server.Serve(listener)
		close(served)
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		<-served
	})

	return listener.Addr().String()
}

func testSMTPConfig(t *testing.T, addr string) config.Config {
	t.Helper()

	host, portValue, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	return config.Config{
		SMTPHost:     host,
		SMTPPort:     port,
		SMTPTLS:      config.SMTPTLSStartTLS,
		SMTPFrom:     "noreply@example.com",
		SMTPFromName: "NeoBlog",
	}
}

func testTransport(t *testing.T, addr string) *smtpTransport {
	t.Helper()
	return newSMTPTransport(testSMTPConfig(t, addr), gomail.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}))
}

func deliverWithTimeout(t *testing.T, tr *smtpTransport, msg Message) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return tr.deliver(ctx, msg)
}

func testMessage() Message {
	return Message{To: "user@example.com", Subject: "邮件配置自检", Body: Body{Text: "纯文本正文", HTML: "<p>HTML 正文</p>"}}
}

func decodeTransferEncoding(encoding string, content []byte) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(bytes.NewReader(content)))
	case "base64":
		return io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(content)))
	default:
		return content, nil
	}
}

func decodeParts(message *netmail.Message) (map[string]string, error) {
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, fmt.Errorf("content type %q is not multipart", mediaType)
	}

	decoded := map[string]string{}
	reader := multipart.NewReader(message.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		content, err := io.ReadAll(part)
		if err != nil {
			return nil, err
		}
		content, err = decodeTransferEncoding(part.Header.Get("Content-Transfer-Encoding"), content)
		if err != nil {
			return nil, err
		}

		partType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			continue
		}
		decoded[partType] = string(content)
	}

	return decoded, nil
}

func TestSMTPTransportDeliversMultipartWithChineseSubject(t *testing.T) {
	backend := &testBackend{}
	addr := startTestServer(t, backend)
	tr := testTransport(t, addr)

	if err := deliverWithTimeout(t, tr, testMessage()); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	from, to, data := backend.snapshot()
	if from != "noreply@example.com" {
		t.Fatalf("from = %q, want noreply@example.com", from)
	}
	if len(to) != 1 || to[0] != "user@example.com" {
		t.Fatalf("to = %v, want [user@example.com]", to)
	}

	parsed, err := netmail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("read message: %v", err)
	}

	subject, err := (&mime.WordDecoder{}).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil {
		t.Fatalf("decode subject: %v", err)
	}
	if subject != "邮件配置自检" {
		t.Fatalf("subject = %q, want 邮件配置自检", subject)
	}

	parts, err := decodeParts(parsed)
	if err != nil {
		t.Fatalf("decode parts: %v", err)
	}
	if !strings.Contains(parts["text/plain"], "纯文本正文") {
		t.Fatalf("text part = %q, want it to contain 纯文本正文", parts["text/plain"])
	}
	if !strings.Contains(parts["text/html"], "<p>HTML 正文</p>") {
		t.Fatalf("html part = %q, want it to contain the HTML body", parts["text/html"])
	}
}

func TestServer5xxIsPermanent(t *testing.T) {
	backend := &testBackend{dataErr: &smtpserver.SMTPError{Code: 550, EnhancedCode: smtpserver.EnhancedCode{5, 1, 1}, Message: "no such user"}}
	addr := startTestServer(t, backend)
	tr := testTransport(t, addr)

	err := deliverWithTimeout(t, tr, testMessage())
	if err == nil {
		t.Fatal("expected an error for a 550 response")
	}

	var delivery *deliveryError
	if !errors.As(err, &delivery) {
		t.Fatalf("error = %T, want *deliveryError", err)
	}
	if delivery.temporary {
		t.Fatalf("550 response must be permanent, got: %v", err)
	}
}

func TestServer4xxIsTemporary(t *testing.T) {
	backend := &testBackend{dataErr: &smtpserver.SMTPError{Code: 451, EnhancedCode: smtpserver.EnhancedCode{4, 0, 0}, Message: "try later"}}
	addr := startTestServer(t, backend)
	tr := testTransport(t, addr)

	err := deliverWithTimeout(t, tr, testMessage())
	if err == nil {
		t.Fatal("expected an error for a 451 response")
	}

	var delivery *deliveryError
	if !errors.As(err, &delivery) {
		t.Fatalf("error = %T, want *deliveryError", err)
	}
	if !delivery.temporary {
		t.Fatalf("451 response must be temporary, got: %v", err)
	}
}

func TestDialFailureIsTemporary(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	tr := newSMTPTransport(testSMTPConfig(t, addr))

	err = deliverWithTimeout(t, tr, testMessage())
	if err == nil {
		t.Fatal("expected a dial error")
	}

	var delivery *deliveryError
	if !errors.As(err, &delivery) {
		t.Fatalf("error = %T, want *deliveryError", err)
	}
	if !delivery.temporary {
		t.Fatalf("dial failures must be retryable, got: %v", err)
	}
}

func TestNewSelectsLogTransportWhenHostEmpty(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(discardLogger())
	t.Cleanup(func() { slog.SetDefault(previous) })

	mailer := New(config.Config{}).Mailer
	if err := mailer.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("send: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := mailer.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}
}
