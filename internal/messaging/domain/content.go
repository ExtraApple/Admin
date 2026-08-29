package domain

import (
	"bytes"
	"errors"
	"regexp"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	htmlrenderer "github.com/yuin/goldmark/renderer/html"
)

const (
	MaxMessageTitleRunes = 100
	MaxMessageBodyRunes  = 20_000
	MaxMessageHTMLBytes  = 128 * 1024
)

var (
	ErrMessageTitleTooLong  = errors.New("message title exceeds the maximum length")
	ErrMessageBodyTooLong   = errors.New("message body exceeds the maximum length")
	ErrMessageHTMLTooLarge  = errors.New("message HTML exceeds the maximum length")
	ErrMessageTextInvalid   = errors.New("message text is not valid UTF-8")
	ErrMessageContentUnsafe = errors.New("message content contains an unsafe construct")
)

type CompiledMessageContent struct {
	Title string
	HTML  string
}

var (
	unsafeMarkdownDestination = regexp.MustCompile(`(?is)(?:!?\[[^\]]*\]\s*\(\s*|<\s*)(?:javascript|data|vbscript)\s*:`)
	unsafeHTMLTag             = regexp.MustCompile(`(?is)<\s*/?\s*(?:script|iframe|object|embed|style|svg|math|link|meta|base|form|input|video|audio)\b`)
	unsafeHTMLEventAttribute  = regexp.MustCompile(`(?is)<[^>]*\bon[a-z0-9_-]+\s*=`)
	unsafeHTMLProtocol        = regexp.MustCompile(`(?is)<[^>]*\b(?:href|src)\s*=\s*["']?\s*(?:javascript|data|vbscript)\s*:`)
)

var safeExternalURL = regexp.MustCompile(`(?i)^https://`)

func CompileMessageContent(title, markdown string) (CompiledMessageContent, error) {
	if !utf8.ValidString(title) || !utf8.ValidString(markdown) {
		return CompiledMessageContent{}, ErrMessageTextInvalid
	}
	if utf8.RuneCountInString(title) > MaxMessageTitleRunes {
		return CompiledMessageContent{}, ErrMessageTitleTooLong
	}
	if utf8.RuneCountInString(markdown) > MaxMessageBodyRunes {
		return CompiledMessageContent{}, ErrMessageBodyTooLong
	}
	if containsUnsafeMarkdown(markdown) {
		return CompiledMessageContent{}, ErrMessageContentUnsafe
	}

	var rendered bytes.Buffer
	markdownParser := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(htmlrenderer.WithXHTML()),
	)
	if err := markdownParser.Convert([]byte(markdown), &rendered); err != nil {
		return CompiledMessageContent{}, err
	}

	html := messageHTMLPolicy().Sanitize(rendered.String())
	if len(html) > MaxMessageHTMLBytes {
		return CompiledMessageContent{}, ErrMessageHTMLTooLarge
	}
	return CompiledMessageContent{Title: title, HTML: html}, nil
}

func containsUnsafeMarkdown(markdown string) bool {
	return unsafeMarkdownDestination.MatchString(markdown) || unsafeHTMLTag.MatchString(markdown) || unsafeHTMLEventAttribute.MatchString(markdown) || unsafeHTMLProtocol.MatchString(markdown)
}

func messageHTMLPolicy() *bluemonday.Policy {
	policy := bluemonday.NewPolicy()
	policy.AllowElements(
		"a", "blockquote", "br", "code", "del", "em", "h1", "h2", "h3", "h4", "h5", "h6",
		"hr", "img", "li", "ol", "p", "pre", "strong", "ul",
	)
	policy.AllowAttrs("href", "title").Matching(safeExternalURL).OnElements("a")
	policy.AllowAttrs("alt", "height", "src", "title", "width").Matching(safeExternalURL).OnElements("img")
	return policy
}
