// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// The profile picture row refuses a file before it is sent, so its list and its cap must be
// the server's. A type only the server takes is a picture nobody can pick. A client cap over
// the route's ceiling is an upload refused with a 413 the row did not foresee.

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendPictureLimits = "../frontend/src/features/settings/picturelimits.ts"

var (
	tsPictureTypes    = regexp.MustCompile(`(?s)PICTURE_TYPES[^=]*=\s*\[(.*?)\]`)
	tsPictureMaxBytes = regexp.MustCompile(`PICTURE_MAX_BYTES\s*=\s*([0-9_]+)\s*;`)
	tsPictureType     = regexp.MustCompile(`["']([^"']+)["']`)
)

func TestTheFrontendPictureLimitsMatchTheServer(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(frontendPictureLimits)
	if err != nil {
		t.Fatalf("reading the frontend picture limits: %v", err)
	}
	source := tsComment.ReplaceAllString(string(raw), " ")

	list := tsPictureTypes.FindStringSubmatch(source)
	if list == nil {
		t.Fatalf("%s declares no PICTURE_TYPES array, so this gate reads nothing", frontendPictureLimits)
	}
	var inTS []string
	for _, m := range tsPictureType.FindAllStringSubmatch(list[1], -1) {
		inTS = append(inTS, m[1])
	}
	if len(inTS) == 0 {
		t.Fatal("PICTURE_TYPES parsed empty, and a gate that reads nothing agrees with everything")
	}
	for mediaType := range compose.AvatarMediaTypes {
		if !slices.Contains(inTS, mediaType) {
			t.Errorf("the server takes %s as a picture and the frontend refuses it before sending", mediaType)
		}
	}
	for _, mediaType := range inTS {
		if _, taken := compose.AvatarMediaTypes[mediaType]; !taken {
			t.Errorf("the frontend offers %s as a picture and the server answers it 415", mediaType)
		}
	}

	limit := tsPictureMaxBytes.FindStringSubmatch(source)
	if limit == nil {
		t.Fatalf("%s declares no numeric PICTURE_MAX_BYTES", frontendPictureLimits)
	}
	got, err := strconv.Atoi(strings.ReplaceAll(limit[1], "_", ""))
	if err != nil {
		t.Fatalf("PICTURE_MAX_BYTES %q is not a byte count: %v", limit[1], err)
	}
	if got != compose.AvatarFileMaxBytes {
		t.Errorf("the frontend caps a picture at %d bytes and the server at %d", got, compose.AvatarFileMaxBytes)
	}
}
