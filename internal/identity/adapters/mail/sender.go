package mailadapter

import (
	"context"
	"fmt"
	"time"

	platformconfig "admin/internal/platform/config"
	goMail "github.com/wneessen/go-mail"
)

// Sender sends verification messages over the configured synchronous SMTP
// connection. It deliberately exposes no SMTP details to the application
// layer.
type Sender struct {
	client  *goMail.Client
	from    string
	timeout time.Duration
}

func NewSender(config platformconfig.SMTPConfig) (*Sender, error) {
	options := []goMail.Option{
		goMail.WithPort(config.Port),
		goMail.WithTimeout(time.Duration(config.TimeoutSeconds) * time.Second),
		goMail.WithUsername(config.Username),
		goMail.WithPassword(config.Password),
		goMail.WithSMTPAuth(goMail.SMTPAuthPlain),
	}
	switch config.TLSMode {
	case "disabled":
		options = append(options, goMail.WithTLSPolicy(goMail.NoTLS))
	case "starttls_required":
		options = append(options, goMail.WithTLSPolicy(goMail.TLSMandatory))
	case "implicit":
		options = append(options, goMail.WithSSL())
	default:
		return nil, fmt.Errorf("unsupported smtp tls mode")
	}
	client, err := goMail.NewClient(config.Host, options...)
	if err != nil {
		return nil, fmt.Errorf("create smtp client: %w", err)
	}
	return &Sender{client: client, from: config.From, timeout: time.Duration(config.TimeoutSeconds) * time.Second}, nil
}

func (sender *Sender) SendVerification(ctx context.Context, recipient, token string) error {
	message := goMail.NewMsg()
	if err := message.From(sender.from); err != nil {
		return fmt.Errorf("set verification sender: %w", err)
	}
	if err := message.To(recipient); err != nil {
		return fmt.Errorf("set verification recipient: %w", err)
	}
	message.Subject("Verify your email")
	message.SetBodyString(goMail.TypeTextPlain, "Use this one-time verification token to verify your email: "+token)
	if sender.timeout <= 0 {
		return sender.client.DialAndSendWithContext(ctx, message)
	}
	sendContext, cancel := context.WithTimeout(ctx, sender.timeout)
	defer cancel()
	return sender.client.DialAndSendWithContext(sendContext, message)
}

var _ interface {
	SendVerification(context.Context, string, string) error
} = (*Sender)(nil)
