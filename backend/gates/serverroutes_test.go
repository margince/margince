// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The server bundle's front nginx sends every path the api mounts to the api.
//
// On a server, scripts/deploy/server/nginx.conf alone splits one host between
// the api and the web server. A route it misses reaches the SPA, which answers
// 200 with the app shell, so a webhook or /readyz looks served and does nothing.
// The routes come from compose/routes.go, so a new mount fails here first.

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	apiMount       = regexp.MustCompile(`mux\.Handle(?:Func)?\("(?:[A-Z]+ )?(/[^"]*)"`)
	securityTxt    = regexp.MustCompile(`securityTxtPath = "(/[^"]+)"`)
	nginxLocation  = regexp.MustCompile(`(?m)^    location (\^~|~|=)? ?(\S+) \{\n((?:        .*\n)*?)    \}`)
	proxiedToAPI   = "proxy_pass http://margince_api;"
	pathsOfTheWebs = []string{"/", "/mcp-apps/view.html", "/assets/app.js", "/.well-known/acme-challenge/token", "/r/deals/1"}
)

type serverLocation struct {
	modifier, pattern string
}

func (l serverLocation) matches(path string) bool {
	switch l.modifier {
	case "^~":
		return strings.HasPrefix(path, l.pattern)
	case "=":
		return path == l.pattern
	default:
		return regexp.MustCompile(l.pattern).MatchString(path)
	}
}

func TestTheServerFrontRoutesEveryApiMountToTheApi(t *testing.T) {
	t.Parallel()
	mounts := apiMounts(t)
	api := apiLocations(t)

	for _, mount := range mounts {
		if !slices.ContainsFunc(api, func(l serverLocation) bool { return l.matches(mount) }) {
			t.Errorf("the api mounts %s, and scripts/deploy/server/nginx.conf sends it to the web server", mount)
		}
	}
	for _, l := range api {
		if !slices.ContainsFunc(mounts, l.matches) {
			t.Errorf("nginx.conf sends %s %s to the api, which mounts nothing there", l.modifier, l.pattern)
		}
	}
	for _, path := range pathsOfTheWebs {
		for _, l := range api {
			if l.matches(path) {
				t.Errorf("nginx.conf sends %s to the api through %s %s; it belongs to the web server", path, l.modifier, l.pattern)
			}
		}
	}
}

// apiMounts lists the top-level paths compose/routes.go mounts. The bare "/" is
// the extension edge's inner mux, which sits behind /v1/ and mounts nothing at
// the top level.
func apiMounts(t *testing.T) []string {
	t.Helper()
	routes := readRepoFile(t, filepath.Join("internal", "compose", "routes.go"))
	var mounts []string
	for _, m := range apiMount.FindAllStringSubmatch(routes, -1) {
		if m[1] != "/" {
			mounts = append(mounts, m[1])
		}
	}
	m := securityTxt.FindStringSubmatch(readRepoFile(t, filepath.Join("internal", "compose", "securitytxt.go")))
	if m == nil {
		t.Fatal("securitytxt.go no longer declares securityTxtPath; read the path from where it is now")
	}
	mounts = append(mounts, m[1])
	if !slices.Contains(mounts, "/v1/") || len(mounts) < 10 {
		t.Fatalf("read %d mounts from routes.go (%v); the pattern no longer sees how routes.go mounts", len(mounts), mounts)
	}
	return mounts
}

func apiLocations(t *testing.T) []serverLocation {
	t.Helper()
	conf := readRepoFile(t, filepath.Join("..", "scripts", "deploy", "server", "nginx.conf"))
	var api []serverLocation
	all := nginxLocation.FindAllStringSubmatch(conf, -1)
	for _, m := range all {
		if strings.Contains(m[3], proxiedToAPI) {
			api = append(api, serverLocation{modifier: m[1], pattern: m[2]})
		}
	}
	if len(all) != strings.Count(conf, "\n    location ") || len(api) == 0 {
		t.Fatalf("read %d of the %d locations in nginx.conf, %d to the api; the pattern no longer reads the file",
			len(all), strings.Count(conf, "\n    location "), len(api))
	}
	return api
}
