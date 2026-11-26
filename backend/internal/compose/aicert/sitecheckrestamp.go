// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

// restampableSiteChecks maps the digest of an earlier site_checks.golden to the
// one that replaced it. A pair is listed when the replacement changed how a
// check is hashed and not the code that judges an answer. A reviewer who read
// that change adds the pair. The narrowed walk of sitecheck_test.go is the
// first pair. The methods it resolves on field types are the second. The walk
// stopping at code that names the database driver is the third. The sweep's
// outcome check is the fourth: the walked runner gained a saved field's JSON key.
var restampableSiteChecks = map[string]string{
	"6ffebf8729e96d9254947a06603e04fc241ce290b27b46e5975a486c20de174a": "dee6b81ecea0a0559db06bb0f561afcdaff4c88e36b210a20cba50a13cda03d0",
	"dee6b81ecea0a0559db06bb0f561afcdaff4c88e36b210a20cba50a13cda03d0": "0afcd88ef36cddac2602acb21f071c5ec0ab31938eff80015e15de7f15b9b6c7",
	"1e815d7a8b6978f7ca99a4636781150d73a4a3e4a2899e567abf91f5ad9ae72c": "dfa2237db6832819d625048ea804ce487ba3d4f70252bfac009f5e0fc1e04c8e",
	"0e2159738213434dbb92e5c64b2f8af7d48ac8999c97904b61a400fb7e002f08": "dc2b1603b573b9ba85f7a182a70fd4ab332a444e5560b8c8b9933d899f2d6e34",
}

// goldenDigest names one site_checks.golden by its bytes.
func goldenDigest(golden string) string {
	sum := sha256.Sum256([]byte(golden))
	return hex.EncodeToString(sum[:])
}

// NewSiteCheckRestampBasis stamps corpus under this build's site checks and
// under oldGolden, both with this build's grading rule. A row whose stamp equals
// the old one measured the check as it stands, so only its hash moved.
func NewSiteCheckRestampBasis(ctx context.Context, oldGolden string, corpus []Scenario, census *aitasks.Registry) (RestampBasis, error) {
	if restampableSiteChecks[goldenDigest(oldGolden)] != goldenDigest(siteChecksGolden) {
		return RestampBasis{}, fmt.Errorf("aicert: restamp: no review says the site checks %s moved only how a check is hashed into %s, "+
			"so their records need new runs; add the pair to restampableSiteChecks once a reviewer has checked that",
			goldenDigest(oldGolden), goldenDigest(siteChecksGolden))
	}
	return restampBasisBetween(ctx, corpus, census,
		func(scenarios []Scenario) (map[string]string, error) {
			return scenarioStampsWith(ctx, gradingRule, parseSiteChecks(oldGolden), scenarios, census)
		})
}
