package application

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrExternalURLInvalid        = errors.New("external URL is invalid")
	ErrExternalDestinationUnsafe = errors.New("external destination is unsafe")
	ErrExternalRedirectRejected  = errors.New("external redirect is rejected")
	ErrExternalResponseInvalid   = errors.New("external response is invalid")
	ErrExternalResponseTooLarge  = errors.New("external response is too large")
)

type ExternalIPResolver interface {
	LookupIP(context.Context, string) ([]net.IP, error)
}

type ExternalProxyConfig struct {
	Client   *http.Client
	Resolver ExternalIPResolver
	Timeout  time.Duration
	MaxBytes int64
}

type ExternalResourceRequest struct {
	URL          string
	AllowedMIMEs []string
}

type ExternalResource struct {
	ContentType string
	Body        []byte
}

type ExternalProxy struct {
	client   *http.Client
	resolver ExternalIPResolver
	timeout  time.Duration
	maxBytes int64
}

func NewExternalProxy(config ExternalProxyConfig) *ExternalProxy {
	client := config.Client
	if client == nil {
		client = &http.Client{}
	}
	resolver := config.Resolver
	if resolver == nil {
		resolver = systemExternalIPResolver{}
	}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = 5 * 1024 * 1024
	}
	return &ExternalProxy{client: client, resolver: resolver, timeout: config.Timeout, maxBytes: config.MaxBytes}
}

func (proxy *ExternalProxy) Fetch(ctx context.Context, request ExternalResourceRequest) (ExternalResource, error) {
	if proxy == nil {
		return ExternalResource{}, ErrExternalURLInvalid
	}
	parsed, err := url.Parse(strings.TrimSpace(request.URL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ExternalResource{}, ErrExternalURLInvalid
	}
	if err := proxy.validateDestination(ctx, parsed.Hostname()); err != nil {
		return ExternalResource{}, err
	}
	allowed := make(map[string]struct{}, len(request.AllowedMIMEs))
	for _, value := range request.AllowedMIMEs {
		value = strings.ToLower(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]))
		if value != "" {
			allowed[value] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return ExternalResource{}, ErrExternalResponseInvalid
	}

	if ctx == nil {
		ctx = context.Background()
	}
	requestContext, cancel := context.WithTimeout(ctx, proxy.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return ExternalResource{}, ErrExternalURLInvalid
	}
	client := *proxy.client
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(httpRequest)
	if err != nil {
		return ExternalResource{}, ErrExternalResponseInvalid
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < 400 {
			return ExternalResource{}, ErrExternalRedirectRejected
		}
		return ExternalResource{}, ErrExternalResponseInvalid
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(response.Header.Get("Content-Type"), ";", 2)[0]))
	if _, ok := allowed[contentType]; !ok {
		return ExternalResource{}, ErrExternalResponseInvalid
	}
	if response.ContentLength > proxy.maxBytes {
		return ExternalResource{}, ErrExternalResponseTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, proxy.maxBytes+1))
	if err != nil {
		return ExternalResource{}, ErrExternalResponseInvalid
	}
	if int64(len(body)) > proxy.maxBytes {
		return ExternalResource{}, ErrExternalResponseTooLarge
	}
	return ExternalResource{ContentType: contentType, Body: body}, nil
}

type systemExternalIPResolver struct{}

func (systemExternalIPResolver) LookupIP(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

func (proxy *ExternalProxy) validateDestination(ctx context.Context, hostname string) error {
	if hostname == "" {
		return ErrExternalDestinationUnsafe
	}
	if ip := net.ParseIP(hostname); ip != nil {
		if !isPublicExternalIP(ip) {
			return ErrExternalDestinationUnsafe
		}
		return nil
	}
	ips, err := proxy.resolver.LookupIP(ctx, hostname)
	if err != nil || len(ips) == 0 {
		return ErrExternalDestinationUnsafe
	}
	for _, ip := range ips {
		if !isPublicExternalIP(ip) {
			return ErrExternalDestinationUnsafe
		}
	}
	return nil
}

func isPublicExternalIP(ip net.IP) bool {
	ip = ip.To16()
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || !ip.IsGlobalUnicast() {
		return false
	}
	if ip.Equal(net.ParseIP("169.254.169.254")) {
		return false
	}
	return true
}
