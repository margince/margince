// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	argumentText       = "text"
	argumentLabel      = "label"
	argumentCount      = "count"
	argumentDate       = "date"
	argumentComponent  = "component"
	argumentMessage    = "message"
	argumentSourceSpan = "source_span"
	commaSeparatedPair = "%s, %s"
	labeledPair        = "%s: %s"
	recordPreviewTitle = "Margince · %s: %s"
	bracketedText      = "[%s]"
	linkedInText       = "LinkedIn: %s"
	dealCaption        = "Deal"
)

// ErrDisplayLimit prevents unbounded display arguments from reaching readers.
var ErrDisplayLimit = errors.New("system text: message exceeds display bounds")

// LayoutConcat keeps adjoining source spans free of added separators.
const LayoutConcat = "concat"

// Origin distinguishes authored evidence from text the system may translate.
type Origin string

const (
	// Template is deterministic display text from the catalog.
	Template Origin = "template"
	// Generated permits automatic rendering only while its source digest matches.
	Generated Origin = "generated_prose"
	// Quote preserves authored evidence rather than automatically translating it.
	Quote Origin = "source_quote"
)

// Argument keeps substitutions typed so record text is never interpreted as a template.
type Argument struct {
	Component *Component `json:"component,omitempty"`
	Kind      string     `json:"kind"`
	Text      string     `json:"text,omitempty"`
	Count     int64      `json:"count,omitempty"`
	Offset    int        `json:"offset,omitempty"`
	Length    int        `json:"length,omitempty"`
}

// Nested retains the inner template instead of freezing its translated sentence.
func Nested(component *Component) Argument {
	return Argument{Kind: argumentComponent, Component: component}
}

// Text preserves source wording while masking credentials at display time.
func Text(value string) Argument { return Argument{Kind: argumentText, Text: value} }

// Count keeps plural and formatting arguments numeric.
func Count(value int64) Argument { return Argument{Kind: argumentCount, Count: value} }

// MessageArgument defers a catalog noun to the reader’s language.
func MessageArgument(code string) Argument { return Argument{Kind: argumentMessage, Text: code} }

// Excerpt preserves a bounded span of the canonical source text.
func Excerpt(offset, length int) Argument {
	return Argument{Kind: argumentSourceSpan, Offset: offset, Length: length}
}

// Date admits a calendar value without interpreting it as prose.
func Date(value string) Argument { return Argument{Kind: argumentDate, Text: value} }

// Component binds one presentation origin to its canonical text digest.
type Component struct {
	Following []Component   `json:"following,omitempty"`
	Layout    string        `json:"layout,omitempty"`
	Origin    Origin        `json:"origin"`
	Code      string        `json:"code,omitempty"`
	Args      []Argument    `json:"args,omitempty"`
	Language  textlang.Lang `json:"language,omitempty"`
	Digest    string        `json:"digest"`
}

// Message keeps subject and body provenance independent.
type Message struct {
	Version int        `json:"schema_version"`
	Subject *Component `json:"subject,omitempty"`
	Body    *Component `json:"body,omitempty"`
}

// Definition owns a phrase and the argument kinds its translations accept.
type Definition struct {
	Words     langcopy.Phrase
	Arguments []string
}

// Sentence records a catalog template without choosing a reader language.
func Sentence(code string, args ...Argument) *Component {
	return &Component{Origin: Template, Code: code, Args: args}
}

// Prose distinguishes generated language from source evidence.
func Prose(origin Origin, language textlang.Lang) *Component {
	return &Component{Origin: origin, Language: language}
}

// Fingerprint detects changes that retire an earlier presentation descriptor.
func Fingerprint(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// Render refuses invalid or unbounded template arguments.
func (c Component) Render(lang textlang.Lang) (string, error) {
	return c.renderBounded(lang, 0, "", true)
}

func (c Component) renderBounded(lang textlang.Lang, depth int, original string, bounded bool) (string, error) {
	if len(c.Following) > 8 || depth > 4 || len(c.Args) > 16 {
		return "", errors.New("system text: sentence sequence exceeds limit")
	}

	definition, ok := definitionOf(c.Code)
	if !ok || c.Origin != Template || len(c.Args) != len(definition.Arguments) {
		return "", errors.New("system text: invalid definition")
	}
	values := make([]any, len(c.Args))
	for i, arg := range c.Args {
		if !argumentKindMatches(arg.Kind, definition.Arguments[i]) {
			return "", errors.New("system text: invalid argument")
		}
		if arg.Kind == argumentCount {
			values[i] = arg.Count
			continue
		}
		value, err := arg.renderText(lang, depth, original, bounded)
		if err != nil {
			return "", err
		}
		values[i] = value
	}

	sentence := definition.Words.In(lang)
	if len(values) > 0 {
		sentence = fmt.Sprintf(sentence, values...)
	}
	lines := []string{sentence}
	for _, part := range c.Following {
		line, err := part.renderBounded(lang, depth+1, original, bounded)
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	separator, err := layoutSeparator(c.Layout)
	if err != nil {
		return "", err
	}
	return strings.Join(lines, separator), nil
}

func (arg Argument) renderText(lang textlang.Lang, depth int, original string, bounded bool) (string, error) {
	if bounded && (arg.Kind == argumentText || arg.Kind == argumentLabel) && utf8.RuneCountInString(arg.Text) > 512 {
		return "", ErrDisplayLimit
	}
	switch arg.Kind {
	case argumentComponent:
		if arg.Component == nil {
			return "", errors.New("system text: missing nested component")
		}
		return arg.Component.renderBounded(lang, depth+1, original, bounded)
	case argumentSourceSpan:
		return arg.sourceSpan(original)
	case argumentMessage:
		nested, ok := definitionOf(arg.Text)
		if !ok || len(nested.Arguments) != 0 {
			return "", errors.New("system text: invalid nested message")
		}
		return nested.Words.In(lang), nil
	case argumentDate:
		return calendarText(arg.Text)
	case argumentText, argumentLabel:
		return credentialmask.MaskText(arg.Text), nil
	default:
		return "", errors.New("system text: invalid argument kind")
	}
}

func calendarText(value string) (string, error) {
	if _, err := time.Parse(time.DateOnly, value); err != nil {
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			return "", errors.New("system text: invalid calendar value")
		}
	}
	return value, nil
}

func (arg Argument) sourceSpan(original string) (string, error) {
	source := []rune(original)
	if arg.Offset < 0 || arg.Length < 0 || arg.Offset > len(source) || arg.Length > len(source)-arg.Offset {
		return "", errors.New("system text: invalid source range")
	}
	return string(source[arg.Offset : arg.Offset+arg.Length]), nil
}

// Bind records the owner's final normalized text, never a pre-truncation approximation.
func Bind(message *Message, subject, body string) (*Message, error) {
	if message == nil {
		return nil, nil //nolint:nilnil // An absent optional descriptor preserves the legacy content path.
	}
	if message.Version != 0 && message.Version != 1 {
		return nil, errors.New("system text: unsupported version")
	}
	out := Message{Version: 1}
	for _, part := range []struct {
		source *Component
		text   string
		target **Component
	}{{message.Subject, subject, &out.Subject}, {message.Body, body, &out.Body}} {
		if part.source == nil {
			continue
		}
		component, included, err := bindComponent(*part.source, part.text)
		if err != nil {
			return nil, err
		}
		if included {
			*part.target = &component
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if metadataSize(encoded) > 8192 {
		return nil, errors.New("system text: metadata exceeds limit")
	}
	return &out, nil
}

func bindComponent(component Component, text string) (Component, bool, error) {
	if component.Digest != "" && !component.Matches(text) {
		return component, false, nil
	}
	switch component.Origin {
	case Template:
		matches := false
		for _, language := range textlang.Shipped {
			rendered, err := component.renderBounded(language, 0, text, true)
			if err != nil {
				return component, false, err
			}
			matches = matches || rendered == text
		}
		if !matches {
			return component, false, nil
		}
	case Generated, Quote:
		if component.Code != "" || len(component.Args) != 0 || len(component.Following) != 0 || component.Layout != "" {
			return component, false, errors.New("system text: prose has template arguments")
		}
	default:
		return component, false, errors.New("system text: invalid origin")
	}
	component.Digest = Fingerprint(text)
	return component, true, nil
}

// Matches requires the descriptor to belong to these exact source bytes.
func (c *Component) Matches(text string) bool { return c != nil && c.Digest == Fingerprint(text) }

// Display renders only a template still bound to the current source text.
func (c *Component) Display(text string, lang textlang.Lang) *string {
	if !c.Matches(text) || c.Origin != Template {
		return nil
	}
	rendered, err := c.renderBounded(lang, 0, text, true)
	if err != nil {
		return nil
	}
	return &rendered
}

// GeneratedParts identifies positively bound prose eligible for automatic rendering.
func (m *Message) GeneratedParts(subject, body string) []string {
	if m == nil || m.Version != 1 {
		return nil
	}
	var parts []string
	if m.Subject.Matches(subject) && m.Subject.Origin == Generated {
		parts = append(parts, "subject")
	}
	if m.Body.Matches(body) && m.Body.Origin == Generated {
		parts = append(parts, "body")
	}
	return parts
}

// Displays leaves canonical source fields intact while offering reader text.
func (m *Message) Displays(subject, body string, lang textlang.Lang) (*string, *string) {
	if m == nil || m.Version != 1 {
		return nil, nil
	}
	return m.Subject.Display(subject, lang), m.Body.Display(body, lang)
}

// Literal is for fixed, argument-free codes declared by the built-in catalog.
func Literal(code string) string {
	definition, ok := definitionOf(code)
	if !ok || len(definition.Arguments) != 0 {
		panic("system text: invalid literal code")
	}
	return definition.Words.En
}

var catalog = mergeCatalogs(definitions, approvalDefinitions, projectDefinitions, ownershipDefinitions, noticeDefinitions, operatorDefinitions, timelineDefinitions, automationDefinitions, dealDefinitions, formDefinitions, publicDefinitions, agentDefinitions, nounDefinitions, publicNounDefinitions(), actDefinitions, kindDefinitions)

func mergeCatalogs(banks ...map[string]Definition) map[string]Definition {
	out := map[string]Definition{}
	for _, bank := range banks {
		for code, definition := range bank {
			if _, exists := out[code]; exists {
				panic("system text: duplicate code")
			}
			out[code] = definition
		}
	}
	return out
}

func definitionOf(code string) (Definition, bool) {
	definition, ok := catalog[code]
	return definition, ok
}

// Copy resolves an argument-free catalog phrase for a reader.
func Copy(code string, language textlang.Lang) string {
	definition, ok := definitionOf(code)
	if !ok {
		panic("system text: unknown fixed code")
	}
	return definition.Words.In(language)
}

// ForDestination carries a verified logical message across owners with different text limits.
func ForDestination(message *Message) *Message {
	if message == nil {
		return nil
	}
	out := Message{Version: message.Version}
	out.Subject = cloneComponent(message.Subject)
	out.Body = cloneComponent(message.Body)
	return &out
}

// BoundLiteral attaches a known template only to its exact canonical text.
func BoundLiteral(code, text string) *Message {
	component := Sentence(code)
	component.Digest = Fingerprint(text)
	return &Message{Version: 1, Subject: component}
}

// Sequence retains the provenance of adjoining template components.
func Sequence(first *Component, following ...*Component) *Component {
	out := cloneComponent(first)
	for _, part := range following {
		if part != nil {
			out.Following = append(out.Following, *cloneComponent(part))
		}
	}
	return out
}

func cloneComponent(source *Component) *Component {
	if source == nil {
		return nil
	}
	out := *source
	if out.Origin == Template {
		out.Digest = ""
	}
	out.Args = append([]Argument(nil), source.Args...)
	for i := range out.Args {
		if out.Args[i].Component != nil {
			out.Args[i].Component = cloneComponent(out.Args[i].Component)
		}
	}
	out.Following = make([]Component, len(source.Following))
	for i := range source.Following {
		out.Following[i] = *cloneComponent(&source.Following[i])
	}
	return &out
}

// Reference localizes a known catalog literal while retaining other source text.
func Reference(code, original string) Argument {
	definition, ok := definitionOf(code)
	if ok && len(definition.Arguments) == 0 && definition.Words.En == original {
		return MessageArgument(code)
	}
	return Text(original)
}

// LocalizedLiteral leaves unrecognized source wording unchanged.
func LocalizedLiteral(code, original string, language textlang.Lang) string {
	argument := Reference(code, original)
	if argument.Kind == argumentMessage {
		return Copy(code, language)
	}
	return original
}

func metadataSize(encoded []byte) int {
	size := len(encoded)
	quoted, escaped := false, false
	for _, character := range encoded {
		if quoted {
			switch {
			case escaped:
				escaped = false
			case character == '\\':
				escaped = true
			case character == '"':
				quoted = false
			}
			continue
		}
		switch character {
		case '"':
			quoted = true
		case ':', ',':
			size++
		}
	}
	return size
}

// Canonical produces the owner’s source spelling before binding it.
func Canonical(component *Component, language textlang.Lang) string {
	rendered, err := component.renderBounded(language, 0, "", false)
	if err != nil {
		panic(err)
	}
	return rendered
}

func layoutSeparator(layout string) (string, error) {
	separator := " "
	switch layout {
	case "":
	case "commas":
		separator = ", "
	case "concat":
		separator = ""
	case "paragraphs":
		separator = "\n\n"
	case "sentences":
		separator = ". "
	case "lines":
		separator = "\n"
	default:
		return "", errors.New("system text: invalid sentence layout")
	}
	return separator, nil
}

func argumentKindMatches(actual, expected string) bool {
	if actual == expected {
		return true
	}
	if expected != argumentText {
		return false
	}
	switch actual {
	case argumentMessage, argumentSourceSpan, argumentLabel, argumentComponent:
		return true
	default:
		return false
	}
}
