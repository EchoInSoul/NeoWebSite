package mail

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	gomail "github.com/wneessen/go-mail"

	"website/backend/internal/config"
)

type smtpTransport struct {
	cfg     config.Config
	client  *gomail.Client
	initErr error
}

func newSMTPTransport(cfg config.Config, opts ...gomail.Option) *smtpTransport {
	options := smtpClientOptions(cfg)
	options = append(options, opts...)

	client, err := gomail.NewClient(cfg.SMTPHost, options...)

	return &smtpTransport{cfg: cfg, client: client, initErr: err}
}

func smtpClientOptions(cfg config.Config) []gomail.Option {
	options := []gomail.Option{
		gomail.WithPort(cfg.SMTPPort),
		gomail.WithTimeout(sendTimeout),
	}

	if cfg.SMTPTLS == config.SMTPTLSSSL {
		options = append(options, gomail.WithSSLPort(false))
	} else {
		options = append(options, gomail.WithTLSPortPolicy(gomail.TLSMandatory))
	}

	if cfg.SMTPUsername != "" {
		options = append(options,
			gomail.WithUsername(cfg.SMTPUsername),
			gomail.WithPassword(cfg.SMTPPassword),
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
		)
	} else {
		options = append(options, gomail.WithSMTPAuth(gomail.SMTPAuthNoAuth))
	}

	return options
}

func (t *smtpTransport) deliver(ctx context.Context, msg Message) error {
	if t.initErr != nil {
		return &deliveryError{err: t.initErr, temporary: false}
	}

	message, err := t.buildMessage(msg)
	if err != nil {
		return &deliveryError{err: err, temporary: false}
	}

	if err := t.client.DialAndSendWithContext(ctx, message); err != nil {
		return newSMTPDeliveryError(err)
	}

	return nil
}

func (t *smtpTransport) buildMessage(msg Message) (*gomail.Msg, error) {
	message := gomail.NewMsg()

	if err := message.FromFormat(t.cfg.SMTPFromName, t.cfg.SMTPFrom); err != nil {
		return nil, err
	}
	if err := message.To(msg.To); err != nil {
		return nil, err
	}

	message.Subject(msg.Subject)
	message.SetBodyString(gomail.TypeTextPlain, msg.Body.Text)
	message.AddAlternativeString(gomail.TypeTextHTML, msg.Body.HTML)

	return message, nil
}

func newSMTPDeliveryError(err error) error {
	var sendErr *gomail.SendError
	if errors.As(err, &sendErr) && sendErr.ErrorCode() != 0 && !sendErr.IsTemp() {
		status := strconv.Itoa(sendErr.ErrorCode())
		if enhanced := sendErr.EnhancedStatusCode(); enhanced != "" {
			status = status + " " + enhanced
		}
		return &deliveryError{err: fmt.Errorf("smtp status %s: %w", status, err), temporary: false}
	}

	return &deliveryError{err: err, temporary: true}
}
