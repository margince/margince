// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// bash32Breakers are tests a bash 3.2 parser rejects outright. macOS ships that
// bash, and a parse error there fails the whole script, not one line of it.
var bash32Breakers = regexp.MustCompile(`\[\[\s+(?:!\s+)?-v\s`)

// No shell script in the tree uses a test macOS bash cannot parse. Then make
// check runs on a contributor's Mac as it does on CI's Linux.
func TestNoShellScriptUsesATestBash32CannotParse(t *testing.T) {
	t.Parallel()
	read := 0
	err := gatekit.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git" || strings.HasPrefix(d.Name(), ".tmp")) {
			return fs.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sh") {
			return nil
		}
		body, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		read++
		for i, line := range strings.Split(string(body), "\n") {
			if bash32Breakers.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "#") {
				t.Errorf("%s:%d uses [[ -v ]], which macOS bash 3.2 cannot parse; test \"${VAR+set}\" instead", path, i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read < 50 {
		t.Fatalf("read %d shell scripts; the walk is short", read)
	}
}
