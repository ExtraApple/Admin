package application_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"admin/internal/messaging/application"
)

type proxyResolverFunc func(context.Context, string) ([]net.IP, error)

func (resolver proxyResolverFunc) LookupIP(ctx context.Context, host string) ([]net.IP, error) {
	return resolver(ctx, host)
}

type proxyRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip proxyRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestExternalProxyFetchesOnlyBoundedHTTPSPublicResources(t *testing.T) {
	proxy := application.NewExternalProxy(application.ExternalProxyConfig{
		Client: &http.Client{Transport: proxyRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader("png")), Request: request}, nil
		})},
		Resolver: proxyResolverFunc(func(_ context.Context, host string) ([]net.IP, error) {
			if host != "cdn.example.test" {
				return nil, errors.New("unexpected host")
			}
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		}),
		Timeout:  time.Second,
		MaxBytes: 8,
	})
	resource, err := proxy.Fetch(context.Background(), application.ExternalResourceRequest{URL: "https://cdn.example.test/image.png", AllowedMIMEs: []string{"image/png"}})
	if err != nil || resource.ContentType != "image/png" || string(resource.Body) != "png" {
		t.Fatalf("Fetch() = %#v, %v", resource, err)
	}
}

func TestExternalProxyRejectsUnsafeDestinationsRedirectsMIMEAndSize(t *testing.T) {
	newProxy := func(roundTrip proxyRoundTripFunc, resolved net.IP) *application.ExternalProxy {
		return application.NewExternalProxy(application.ExternalProxyConfig{
			Client:   &http.Client{Transport: roundTrip},
			Resolver: proxyResolverFunc(func(context.Context, string) ([]net.IP, error) { return []net.IP{resolved}, nil }),
			Timeout:  time.Second,
			MaxBytes: 3,
		})
	}
	for _, test := range []struct {
		name string
		url  string
		ip   net.IP
		rt   proxyRoundTripFunc
	}{
		{name: "http", url: "http://cdn.example.test/a", ip: net.ParseIP("8.8.8.8")},
		{name: "private", url: "https://private.example.test/a", ip: net.ParseIP("127.0.0.1")},
		{name: "redirect", url: "https://cdn.example.test/a", ip: net.ParseIP("8.8.8.8"), rt: func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://other.example.test/a"}}, Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
		}},
		{name: "mime", url: "https://cdn.example.test/a", ip: net.ParseIP("8.8.8.8"), rt: func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader("ok")), Request: request}, nil
		}},
		{name: "size", url: "https://cdn.example.test/a", ip: net.ParseIP("8.8.8.8"), rt: func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader("more than three")), Request: request}, nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rt := test.rt
			if rt == nil {
				rt = func(request *http.Request) (*http.Response, error) {
					return nil, errors.New("transport should not be called")
				}
			}
			proxy := newProxy(rt, test.ip)
			if _, err := proxy.Fetch(context.Background(), application.ExternalResourceRequest{URL: test.url, AllowedMIMEs: []string{"image/png"}}); err == nil {
				t.Fatal("unsafe external resource was accepted")
			}
		})
	}
}
