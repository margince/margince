// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// Only platform/events writes to the event stream. An event goes out through
// the outbox, which commits with the change it describes, and the relay ships
// it. A send of its own, anywhere else, can reach consumers for a change that
// rolls back. It also carries no audit row a consumer could trace it to.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// streamWriterRoot is the one package that may send XADD.
const streamWriterRoot = "internal/platform/events"

// The scope fails when the root holds no writer, so a relay that stopped
// matching this predicate cannot read as a clean tree.
func TestOnlyPlatformEventsWritesToTheEventStream(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots: []string{streamWriterRoot},
		Subject: func(_ string, file *ast.File) bool {
			return len(streamWrites(file)) > 0
		},
	}
	scope.Files(t)
}

// Extensions compose into the same binary from outside this module, where the
// scope above does not reach.
func TestNoExtensionWritesToTheEventStream(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "extensions")
	if !walkedAnyGo(t, root) {
		t.Fatalf("%s holds no Go, so this check reads nothing: the extension tree has moved", root)
	}
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		for _, line := range streamWrites(file) {
			t.Errorf("%s:%d writes to a Redis stream from an extension; emit through the outbox instead", path, line)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}

// streamWrites answers the lines that send XADD. That is a call of a method
// named XAdd on any receiver, or the command named as a string, as Do and
// NewCmd take it.
func streamWrites(file *ast.File) []int {
	var lines []int
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			if node.Sel.Name == "XAdd" {
				lines = append(lines, gatekit.SourceFileSet().Position(node.Pos()).Line)
			}
		case *ast.BasicLit:
			if word, err := strconv.Unquote(node.Value); node.Kind == token.STRING && err == nil &&
				strings.EqualFold(word, "xadd") {
				lines = append(lines, gatekit.SourceFileSet().Position(node.Pos()).Line)
			}
		}
		return true
	})
	return lines
}

// Each spelling the predicate must see, and the mentions it must not count.
func TestTheStreamWritePredicateSeesEverySpellingOfXADD(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"client method":    `rdb.XAdd(ctx, &redis.XAddArgs{Stream: "s"})`,
		"pipeline method":  `pipe := rdb.Pipeline(); pipe.XAdd(ctx, args)`,
		"generic command":  `rdb.Do(ctx, "XADD", "s", "*", "v", "{}")`,
		"lowercase string": `rdb.Process(ctx, redis.NewCmd(ctx, "xadd", "s", "*"))`,
	} {
		if streamWrites(parseGoSource(t, plantedStreamSource(body)).File) == nil {
			t.Errorf("the predicate missed a %s: %s", name, body)
		}
	}
	for name, body := range map[string]string{
		"an error message": `_ = fmt.Errorf("bus: XADD %s: %w", stream, err)`,
		"a stream read":    `rdb.XRead(ctx, &redis.XReadArgs{Streams: []string{"s", "0"}})`,
	} {
		if lines := streamWrites(parseGoSource(t, plantedStreamSource(body)).File); lines != nil {
			t.Errorf("the predicate counted %s as a write at lines %v: %s", name, lines, body)
		}
	}
}

func plantedStreamSource(body string) string {
	return "package planted\n\nfunc send() {\n\t" + body + "\n}\n"
}
