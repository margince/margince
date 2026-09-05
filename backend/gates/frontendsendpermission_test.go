// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every surface that posts to a send door asks the engine first, through the
// one component that says what it answered.
//
// The engine decides whether a message may go, and the preview doors have
// answered that since it landed. A surface that sends without asking teaches a
// rep the refusal by pressing Send and reading an error — and a surface that
// asks through its OWN reading of the preview is a second spelling of a consent
// surface, which is the worst place for one: two spellings that drift mean one
// screen tells a rep they may send and another that they may not, about the
// same message.
//
// So every frontend file that names a send door either renders SendPermission
// or is ratified here with what its silence costs.
//
// The doors are DERIVED from the contract rather than listed: a send door is an
// operation whose request body carries `operator_reason`, the field through
// which a sender's own word about a send reaches the engine. A list here would
// be a second copy of the contract, and the copy that stopped matching would
// let a fourth door ship with no question asked.
//
// NOT asserted: a surface that stages a send without posting to a door — the
// scheduled-send queue lists messages already staged, and the approval card
// releases them through a generic decide route. Both adopt the component today,
// and nothing derivable marks their routes as sends, so the census cannot see a
// third such surface arriving without one.

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	sendPermissionContract  = "api/crm.yaml"
	sendPermissionFrontend  = "../frontend/src"
	sendPermissionComponent = "../frontend/src/screens/sendpermission.tsx"
)

// silentSendSurfaces ratifies each surface that posts to a send door without
// asking the engine first, with what the omission costs.
var silentSendSurfaces = gatekit.Waive(map[string]string{
	"screens/persondrawers.tsx": "the person page's composer leads with the person's purpose-keyed " +
		"consent guard, read off the 360; drawing the engine's answer beside it would put two " +
		"verdicts about one message on one drawer, so adopting the component there means " +
		"retiring that guard first, which is its own change",
})

// contractPathLine matches a path entry under `paths:`.
var contractPathLine = regexp.MustCompile(`^  (/[^\s:]+):\s*$`)

// contractSchemaLine matches a schema entry under `components: schemas:`.
var contractSchemaLine = regexp.MustCompile(`^    ([A-Za-z][A-Za-z0-9]*):\s*$`)

// contractSchemaRef matches a request-body reference to a named schema.
var contractSchemaRef = regexp.MustCompile(`\$ref: '#/components/schemas/([A-Za-z][A-Za-z0-9]*)'`)

// overrideProperty is the field that marks a schema as a send request body.
var overrideProperty = regexp.MustCompile(`^        operator_reason:\s*$`)

// sendDoors reads the contract for every path whose request body schema carries
// operator_reason: the doors through which a sender's own word about a send
// reaches the engine.
func sendDoors(t *testing.T) []string {
	t.Helper()
	file, err := os.Open(sendPermissionContract)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	defer file.Close()

	refsByPath := map[string][]string{}
	carriesOverride := map[string]bool{}
	var path, schema string
	inSchemas := false
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "components:":
			inSchemas = true
			path = ""
		case !inSchemas && contractPathLine.MatchString(line):
			path = contractPathLine.FindStringSubmatch(line)[1]
		case !inSchemas && path != "":
			if m := contractSchemaRef.FindStringSubmatch(line); m != nil {
				refsByPath[path] = append(refsByPath[path], m[1])
			}
		case inSchemas && contractSchemaLine.MatchString(line):
			schema = contractSchemaLine.FindStringSubmatch(line)[1]
		case inSchemas && schema != "" && overrideProperty.MatchString(line):
			carriesOverride[schema] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning the contract: %v", err)
	}

	var doors []string
	for p, refs := range refsByPath {
		for _, ref := range refs {
			if carriesOverride[ref] {
				doors = append(doors, p)
				break
			}
		}
	}
	slices.Sort(doors)
	return doors
}

// namesADoor reports whether source posts to any of the doors.
func namesADoor(source string, doors []string) bool {
	for _, door := range doors {
		if strings.Contains(source, `"`+door+`"`) {
			return true
		}
	}
	return false
}

// rendersSendPermission matches the component's JSX tag and nothing that merely
// starts with its name: a `<SendPermissionBanner` of a surface's own would be
// the second spelling this gate exists to refuse.
var rendersSendPermission = regexp.MustCompile(`<SendPermission[\s/>]`)

// asksTheEngine reports whether source renders the shared component.
func asksTheEngine(source string) bool {
	return strings.Contains(source, `from "./sendpermission"`) &&
		rendersSendPermission.MatchString(source)
}

// isSendSurfaceSource is the frontend production source the census sweeps:
// TypeScript, minus tests, stories and the generated contract.
func isSendSurfaceSource(path string) bool {
	if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
		return false
	}
	base := filepath.Base(path)
	return !strings.Contains(base, ".test.") &&
		!strings.Contains(base, ".stories.") &&
		!strings.Contains(base, ".testkit.") &&
		!strings.HasSuffix(base, ".d.ts")
}

func TestEverySurfaceThatSendsAsksTheEngineFirst(t *testing.T) {
	t.Parallel()

	doors := sendDoors(t)
	// Under-recognition is the failure that reports PASS. The mail reply, the
	// channel reply and the account-started mail all carry the field; fewer
	// means the scanner has stopped seeing its subject.
	if len(doors) < 3 {
		t.Fatalf("derived %d send door(s) from the contract, want at least the three that carry "+
			"operator_reason: the census has stopped seeing its subject (%v)", len(doors), doors)
	}

	component, err := os.ReadFile(sendPermissionComponent)
	if err != nil {
		t.Fatalf("reading the shared component: %v", err)
	}
	if !strings.Contains(string(component), "export function SendPermission(") {
		t.Fatal("sendpermission.tsx no longer exports SendPermission: every surface that satisfies " +
			"this gate by naming it now renders something else, or nothing")
	}

	surfaces := 0
	walkErr := filepath.WalkDir(sendPermissionFrontend, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isSendSurfaceSource(path) {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		source := string(raw)
		if !namesADoor(source, doors) {
			return nil
		}
		surfaces++
		rel, relErr := filepath.Rel(sendPermissionFrontend, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if asksTheEngine(source) || silentSendSurfaces.Waived(t, rel) {
			return nil
		}
		t.Errorf("%s posts to a send door and never asks the engine: a rep learns a refusal by "+
			"pressing Send. Render SendPermission fed by useSendPermission, or ratify the surface in "+
			"silentSendSurfaces with what its silence costs", rel)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking the frontend: %v", walkErr)
	}
	// The composer and the person page's composer both post to a door today.
	// Fewer means the walk has stopped seeing the surfaces it exists to hold.
	if surfaces < 2 {
		t.Fatalf("found %d frontend file(s) posting to a send door, want at least the two composers: "+
			"the census has stopped seeing its subject", surfaces)
	}
	silentSendSurfaces.AssertAllMatched(t)
}
