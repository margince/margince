// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"net/url"
	"strings"
)

// hostRoot is the host a provider's adapter dials under. An adapter that adds
// /v1/... itself takes a pasted ".../v1" as its root, or every call would go to
// /v1/v1/... and 404. Any other host, and any other provider's, is returned as
// written, so a stored value that needs nothing keeps its exact spelling.
func hostRoot(provider, host string) string {
	if d, known := providerByName(provider); !known || !d.pathsUnderV1 {
		return host
	}
	parsed, err := url.Parse(host)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return host
	}
	path := strings.TrimRight(parsed.Path, "/")
	cut := strings.LastIndex(path, "/")
	if cut < 0 || !strings.EqualFold(path[cut+1:], "v1") {
		return host
	}
	parsed.Path, parsed.RawPath = path[:cut], ""
	return parsed.String()
}
