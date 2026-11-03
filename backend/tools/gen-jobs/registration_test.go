// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestGeneratedRegistrationSupportsLargeCatalogsAndRejectsUndeclaredTypes(t *testing.T) {
	catalog := contract{Kinds: map[string]kindDef{}}
	var args strings.Builder
	for index := range 130 {
		name := fmt.Sprintf("Job%dArgs", index)
		catalog.Kinds[name] = kindDef{GoType: name}
		fmt.Fprintf(&args, "type %s struct{}\nfunc (%s) Kind() string { return %q }\n", name, name, name)
	}
	var declaration strings.Builder
	writeDeclaredSet(&declaration, catalog)
	var source strings.Builder
	fmt.Fprintf(&source, "package registration\nimport \"github.com/riverqueue/river\"\n%s%s\nfunc register[T declaredJobArgs[T]](T) {}\n", args.String(), declaration.String())
	for name := range catalog.Kinds {
		fmt.Fprintf(&source, "func use%s(){ register(%s{}) }\n", name, name)
	}
	compileRegistration(t, source.String(), true)
	compileRegistration(t, source.String()+"type Unknown struct{}\nfunc (Unknown) Kind() string{return \"unknown\"}\nfunc useUnknown(){register(Unknown{})}\n", false)
	compileRegistration(t, source.String()+"type Wrapped struct{Job0Args}\nfunc useWrapped(){register(Wrapped{})}\n", false)
}

func compileRegistration(t *testing.T, source string, accepted bool) {
	t.Helper()
	if err := gatekit.DeclareInputs(os.Getenv, "../../../backend/go.mod", "../../../backend/go.sum", "../go.mod", "../go.sum"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "registration_test.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", "-run", "^$", path)
	command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOWORK=off")
	output, err := command.CombinedOutput()
	if accepted && err != nil {
		t.Fatalf("generated registration did not compile: %v\n%s", err, output)
	}
	if !accepted && (err == nil || !strings.Contains(string(output), declaredJobMarker)) {
		t.Fatalf("undeclared type was not refused by its marker: %v\n%s", err, output)
	}
}
