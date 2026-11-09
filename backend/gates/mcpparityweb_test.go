// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Which contract operations the web app calls, read from its source. A typed
// `api.GET("/path")` names its operation outright. The other two forms are inferred.
// One is a path in a constant passed to the client. The other is a `/v1/…` URL used
// as a link, an image or a multipart upload.

import (
	"io/fs"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	webSourceRoot = "../frontend/src"
	webUnitRoot   = "../extensions"
	// How the scan knows the web app calls an operation.
	webCallDirect   = "direct"
	webCallInferred = "inferred"
	webCallURL      = "url"
)

// webTestSupport names the files that run only under a test or a story, by the
// suffixes and the folder this tree gives them. A call in one proves no screen
// makes it.
var webTestSupport = regexp.MustCompile(`\.(test|spec|stories|testkit|testing|judge)\.[a-z]+$|\.d\.ts$|(^|/)(testing|__mocks__)/`)

var webModuleFile = regexp.MustCompile(`\.(ts|tsx|mts|cts|js|jsx|mjs|cjs)$`)

var (
	// webClientCall matches any receiver, because the sign-in screens chain the
	// client over a line break. A literal is held to the contract afterwards.
	webClientCall = regexp.MustCompile(`\.\s*(GET|POST|PUT|PATCH|DELETE)\s*(?:<[^>()]*>)?\(\s*`)
	webVersioned  = regexp.MustCompile(`["'` + "`" + `](?:\$\{[^}]*\})?/v1(/[^"'` + "`" + `\s]*)["'` + "`" + `]`)
	webMultipart  = regexp.MustCompile(`postMultipart\(\s*$`)
	webMethodKey  = regexp.MustCompile(`method:\s*["'](GET|POST|PUT|PATCH|DELETE)["']`)
	webTemplate   = regexp.MustCompile(`\$\{[^}]*\}`)
	// webPathConstant is a module-level const, the one kind a call in any function
	// can name. webDeclaration finds a second binding that would make the name unsure.
	webPathConstant = regexp.MustCompile(`(?m)^(?:export\s+)?const\s+([A-Za-z_$][\w$]*)\s*(?::[^=\n]+)?=\s*`)
	webDeclaration  = regexp.MustCompile(`\b(?:const|let|var)\s+([A-Za-z_$][\w$]*)`)
	webValueEnds    = regexp.MustCompile(`^(?:\s+as\s+const)?[ \t]*(?:;|\r?\n|$)`)
	webPlainName    = regexp.MustCompile(`^([A-Za-z_$][\w$]*)\s*[,)]`)
	webNameHole     = regexp.MustCompile(`\$\{\s*([A-Za-z_$][\w$]*)\s*\}`)
)

// webScan is what one read of the web app's source found. Only Unresolved is
// published. The other counts move with every new screen, so a page that printed
// them would fail each frontend change for nothing a reader needs.
type webScan struct {
	Unresolved []string `json:"computed_calls_no_path_was_found_for"`
	// calls is METHOD plus contract path, to how the scan knows it.
	calls       map[string]string
	files       int
	directCalls int
	unitCalls   int
}

// contractRoutes holds the contract's paths and which methods each one serves.
type contractRoutes map[string][]string

func (r contractRoutes) has(method, path string) bool { return slices.Contains(r[path], method) }

// webFiles lists the source files of the web app and of each unit's frontend.
func webFiles() ([]string, error) {
	var kept []string
	walk := func(root string, keep func(string) bool) error {
		return gatekit.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			name := filepath.ToSlash(path)
			if !webModuleFile.MatchString(name) || !keep(name) {
				return nil
			}
			if !webTestSupport.MatchString(name) {
				kept = append(kept, path)
			}
			return nil
		})
	}
	if err := walk(webSourceRoot, func(string) bool { return true }); err != nil {
		return nil, err
	}
	err := walk(webUnitRoot, func(name string) bool { return strings.Contains(name, "/frontend/") })
	return kept, err
}

// scanWebApp reads every source file. The path-template index is built from the
// contract, so a path that names no contract route cannot count.
func scanWebApp(routes contractRoutes) (webScan, []string, error) {
	files, err := webFiles()
	if err != nil {
		return webScan{}, nil, err
	}
	sources := map[string]string{}
	for _, file := range files {
		body, readErr := gatekit.ReadFile(file)
		if readErr != nil {
			return webScan{}, nil, readErr
		}
		sources[filepath.ToSlash(file)] = string(body)
	}
	scan, strays := scanWebSources(sources, routes)
	return scan, strays, nil
}

// scanWebSources does the reading on sources held in memory, so a test can plant
// each form. The second result lists literal paths the contract does not serve:
// the typed client cannot name one, so each is a defect in this scan.
func scanWebSources(sources map[string]string, routes contractRoutes) (webScan, []string) {
	scan := webScan{files: len(sources), calls: map[string]string{}, Unresolved: []string{}}
	matchers := routeMatchers(routes)
	var strays []string
	var waiting []computedCall
	namedInFile := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		code, literals := stripTSComments(sources[name])
		constants := webPathConstants(code)
		unused := unusedAsWholePaths(code, constants)
		literals = slices.DeleteFunc(literals, func(literal string) bool { return unused[literal] })
		for i, literal := range literals {
			literals[i] = expandPathConstants(literal, constants)
		}
		direct := map[string]bool{}
		for _, at := range webClientCall.FindAllStringSubmatchIndex(code, -1) {
			method := code[at[2]:at[3]]
			path, isLiteral := stringLiteralAt(code, at[1])
			switch {
			case !isLiteral:
				built, isBuilt := builtPath(code, at[1], constants)
				if !isBuilt || !scan.creditBuilt(name, method, built, routes, matchers, &strays) {
					waiting = append(waiting, computedCall{file: name, method: method})
				}
			case strings.HasPrefix(path, unitRoutePrefix):
				scan.unitCalls++
			case !routes.has(method, path):
				strays = append(strays, name+" "+method+" "+path)
			default:
				scan.directCalls++
				direct[path] = true
				scan.calls[method+" "+path] = webCallDirect
			}
		}
		namedInFile[name] = routesNamedBy(literals, direct, routes, matchers)
		scan.collectURLs(code, matchers, routes)
	}
	scan.resolveComputed(waiting, namedInFile, routes)
	return scan, strays
}

// webPathConstants reads the file's module-level consts that hold one whole path.
// A value may start from an earlier constant. A name bound twice is left out.
func webPathConstants(code string) map[string]string {
	bindings := map[string]int{}
	for _, match := range webDeclaration.FindAllStringSubmatch(code, -1) {
		bindings[match[1]]++
	}
	constants := map[string]string{}
	for _, at := range webPathConstant.FindAllStringSubmatchIndex(code, -1) {
		name := code[at[2]:at[3]]
		if bindings[name] != 1 || at[1] >= len(code) || !strings.ContainsRune(`"'`+"`", rune(code[at[1]])) {
			continue
		}
		end := literalEnd(code, at[1])
		if end+1 > len(code) || !webValueEnds.MatchString(code[end+1:]) {
			continue
		}
		if value := expandPathConstants(code[at[1]+1:end], constants); strings.HasPrefix(value, "/") {
			constants[name] = value
		}
	}
	return constants
}

// unusedAsWholePaths lists the values of constants the file never passes whole. The
// file declares one and uses it only to start a template, or not at all. Such a
// value is no call's path, so it names no route.
func unusedAsWholePaths(code string, constants map[string]string) map[string]bool {
	holes := map[string]int{}
	for _, match := range webNameHole.FindAllStringSubmatch(code, -1) {
		holes[match[1]]++
	}
	values := map[string]bool{}
	for name, value := range constants {
		uses := len(regexp.MustCompile(`\b`+regexp.QuoteMeta(name)+`\b`).FindAllString(code, -1))
		if uses == holes[name]+1 {
			values[value] = true
		}
	}
	return values
}

// expandPathConstants writes each `${NAME}` hole a path constant fills, and keeps
// any other hole.
func expandPathConstants(text string, constants map[string]string) string {
	return webNameHole.ReplaceAllStringFunc(text, func(hole string) string {
		if value, known := constants[webNameHole.FindStringSubmatch(hole)[1]]; known {
			return value
		}
		return hole
	})
}

// builtPath reads the path a computed call states itself. That is a constant's name,
// or a template that starts at the root once its constants are written out.
func builtPath(code string, offset int, constants map[string]string) (string, bool) {
	if offset >= len(code) {
		return "", false
	}
	if code[offset] == '`' {
		path := expandPathConstants(code[offset+1:literalEnd(code, offset)], constants)
		return path, strings.HasPrefix(path, "/")
	}
	if name := webPlainName.FindStringSubmatch(code[offset:]); name != nil {
		path, known := constants[name[1]]
		return path, known
	}
	return "", false
}

// creditBuilt counts the routes a built path names. It answers false for a shape the
// contract cannot settle, so the file's named routes get to answer. A whole path the
// contract does not serve is a stray, as a literal one is.
func (s *webScan) creditBuilt(file, method, path string, routes contractRoutes, matchers []routeMatcher, strays *[]string) bool {
	if !strings.Contains(path, "${") {
		switch {
		case strings.HasPrefix(path, unitRoutePrefix):
			s.unitCalls++
		case routes.has(method, path):
			s.creditInferred(method, path)
		default:
			*strays = append(*strays, file+" "+method+" "+path)
		}
		return true
	}
	shape := typedTemplateShape(path)
	if shape == nil {
		return false
	}
	found := false
	for _, matcher := range matchers {
		if shape.MatchString(matcher.path) && routes.has(method, matcher.path) {
			found = true
			s.creditInferred(method, matcher.path)
		}
	}
	return found
}

// creditInferred never overwrites how the scan already knows a call.
func (s *webScan) creditInferred(method, path string) {
	if _, known := s.calls[method+" "+path]; !known {
		s.calls[method+" "+path] = webCallInferred
	}
}

// routesNamedBy lists the contract routes a file's literals spell without a direct
// call: a const, an object field, or a template. A template with a hole names every
// route its shape fits.
func routesNamedBy(literals []string, direct map[string]bool, routes contractRoutes, matchers []routeMatcher) []string {
	var named []string
	for _, literal := range literals {
		if _, isRoute := routes[literal]; isRoute && !direct[literal] {
			named = append(named, literal)
		}
		shape := typedTemplateShape(literal)
		if shape == nil {
			continue
		}
		for _, matcher := range matchers {
			if shape.MatchString(matcher.path) {
				named = append(named, matcher.path)
			}
		}
	}
	return named
}

// typedTemplateShape matches the routes a template such as `/rooms/{id}/${verb}`
// can name. It answers nil for any other string: a typed path keeps a `{param}`,
// which an app link such as `/contacts/${id}` does not.
func typedTemplateShape(literal string) *regexp.Regexp {
	parts := webTemplate.Split(literal, -1)
	if !strings.HasPrefix(literal, "/") || len(parts) < 2 || !strings.Contains(strings.Join(parts, ""), "{") {
		return nil
	}
	for i, part := range parts {
		parts[i] = regexp.QuoteMeta(part)
	}
	return regexp.MustCompile("^" + strings.Join(parts, "[^/]+") + "$")
}

// computedCall is a typed client call whose path is an expression.
type computedCall struct{ file, method string }

// resolveComputed pairs each call builtPath could not read with the routes its own
// file names and that serve its method. A call with none takes its path from another
// file, and the page lists it as unresolved.
func (s *webScan) resolveComputed(waiting []computedCall, namedInFile map[string][]string, routes contractRoutes) {
	for _, call := range waiting {
		found := false
		for _, path := range namedInFile[call.file] {
			if !routes.has(call.method, path) {
				continue
			}
			found = true
			s.creditInferred(call.method, path)
		}
		if !found {
			s.Unresolved = append(s.Unresolved, call.file+" "+call.method)
		}
	}
	slices.Sort(s.Unresolved)
	s.Unresolved = slices.Compact(s.Unresolved)
}

// collectURLs reads `/v1/…` strings. A link, an image and a redirect are GETs, an
// upload is whatever method its code states. A URL built from a variable prefix
// is out of reach, and the page says so.
func (s *webScan) collectURLs(code string, matchers []routeMatcher, routes contractRoutes) {
	for _, at := range webVersioned.FindAllStringSubmatchIndex(code, -1) {
		raw, _, _ := strings.Cut(code[at[2]:at[3]], "?")
		shaped := webTemplate.ReplaceAllString(raw, "{}")
		for _, matcher := range matchers {
			if !matcher.pattern.MatchString(shaped) {
				continue
			}
			method := urlMethod(code, at[0], at[1], matcher.path, routes)
			if _, known := s.calls[method+" "+matcher.path]; !known && routes.has(method, matcher.path) {
				s.calls[method+" "+matcher.path] = webCallURL
			}
			break
		}
	}
}

// urlMethod reads the method near one URL: a multipart helper in front of it, or
// a `method:` key in the call's options. With neither, the URL is opened as a GET,
// or is an upload when its route serves no GET.
func urlMethod(code string, start, end int, path string, routes contractRoutes) string {
	if webMultipart.MatchString(code[max(0, start-40):start]) {
		return "POST"
	}
	if near := webMethodKey.FindStringSubmatch(code[end:ownCallEnd(code, end)]); near != nil {
		return near[1]
	}
	if routes.has("GET", path) {
		return "GET"
	}
	return "POST"
}

// ownCallEnd is where the call around a URL closes, at most 200 bytes on.
func ownCallEnd(code string, start int) int {
	depth := 0
	limit := min(len(code), start+200)
	for i := start; i < limit; i++ {
		switch code[i] {
		case '"', '\'', '`':
			// A delimiter inside a string is text, not the call's structure.
			quote := code[i]
			for i++; i < limit && code[i] != quote; i++ {
				if code[i] == '\\' {
					i++
				}
			}
		case '(', '{', '[':
			depth++
		case ')', '}', ']':
			depth--
		case ';':
			if depth <= 0 {
				return i
			}
		}
		if depth < 0 {
			return i
		}
	}
	return limit
}

type routeMatcher struct {
	path    string
	pattern *regexp.Regexp
}

// routeMatchers turns each contract path into a pattern for a URL in source, where
// `{}` stands for a `${…}` hole. Longer paths come first, so the most specific wins.
func routeMatchers(routes contractRoutes) []routeMatcher {
	out := make([]routeMatcher, 0, len(routes))
	for _, path := range slices.Sorted(maps.Keys(routes)) {
		quoted := regexp.QuoteMeta(path)
		shaped := regexp.MustCompile(`\\\{[^/]*\\\}`).ReplaceAllString(quoted, `[^/]+`)
		out = append(out, routeMatcher{path: path, pattern: regexp.MustCompile("^" + shaped + "$")})
	}
	slices.SortStableFunc(out, func(a, b routeMatcher) int { return len(b.path) - len(a.path) })
	return out
}

// stringLiteralAt reads a quoted path that opens at offset, and declines a template
// with a hole, which has no one path.
func stringLiteralAt(code string, offset int) (string, bool) {
	if offset >= len(code) || !strings.ContainsRune(`"'`+"`", rune(code[offset])) {
		return "", false
	}
	quote := code[offset]
	end := strings.IndexByte(code[offset+1:], quote)
	if end < 0 {
		return "", false
	}
	text := code[offset+1 : offset+1+end]
	return text, !strings.Contains(text, "${")
}

// stripTSComments blanks every comment and returns the string literals it passed.
// A plain regexp cannot do this: `//` inside a URL string starts no comment, and a
// call quoted in a comment is no call. A string ends at its line's end, so a quote
// in a regular expression literal costs at most the rest of that line.
func stripTSComments(source string) (string, []string) {
	code := []byte(source)
	var literals []string
	for i := 0; i < len(code); {
		switch {
		case code[i] == '/' && i+1 < len(code) && code[i+1] == '/':
			for i < len(code) && code[i] != '\n' {
				code[i] = ' '
				i++
			}
		case code[i] == '/' && i+1 < len(code) && code[i+1] == '*':
			end := strings.Index(source[i+2:], "*/")
			stop := len(code)
			if end >= 0 {
				stop = i + 2 + end + 2
			}
			for ; i < stop; i++ {
				if code[i] != '\n' {
					code[i] = ' '
				}
			}
		case code[i] == '"' || code[i] == '\'' || code[i] == '`':
			end := literalEnd(source, i)
			literals = append(literals, source[i+1:max(i+1, end)])
			i = end + 1
		default:
			i++
		}
	}
	return string(code), literals
}

// literalEnd is the offset of the quote closing the literal opened at start, or of
// the line's end for a quote that never closes.
func literalEnd(source string, start int) int {
	quote := source[start]
	for i := start + 1; i < len(source); i++ {
		switch {
		case source[i] == '\\':
			i++
		case source[i] == quote:
			return i
		case source[i] == '\n' && quote != '`':
			return i
		}
	}
	return len(source) - 1
}
