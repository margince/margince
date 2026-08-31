// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// Two token exchanges for the SAME client, racing. issueGrant already
// serializes any two exchanges for one human — requireLiveConsentingUser takes
// app_user FOR UPDATE before either grant is inserted, and whichever commits
// second runs supersedePriorGrants and revokes what the first just wrote
// (oauth_grant.go's file header, and supersedePriorGrants's own doc comment).
// That argument is sound by construction; this suite is the proof it holds
// under a real Postgres, not merely on paper.

import (
	"net/url"
	"sync"
	"testing"
)

// concurrentTokenExchange fires two already-minted authorization codes at
// /oauth/token at once. It cannot use exchange/postToken: only the goroutine
// running the test may call t, so each presentation reports its outcome as a
// value the way presentInParallel's present does for refresh rotation.
func (o *oauthEnv) concurrentTokenExchange(codeA, codeB string) (a, b renewal) {
	formFor := func(code string) string {
		return url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {o.clientID},
			"redirect_uri":  {oauthRedirect},
			"code_verifier": {o.verifier},
			"code":          {code},
		}.Encode()
	}
	forms := [2]string{formFor(codeA), formFor(codeB)}
	results := [2]renewal{}

	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := range forms {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-release
			results[i] = o.present(forms[i])
		}(i)
	}
	close(release)
	wg.Wait()
	return results[0], results[1]
}

// Two authorization codes for one client, exchanged at once, must not leave
// two live grants behind: requireLiveConsentingUser's app_user lock serializes
// the pair, and the one that commits second supersedes the one that committed
// first. Both codes are single-use and independently valid, so each exchange
// succeeds on its own terms — what this test guards is that "succeeds" never
// means "two connections for one client," which the Settings list, and every
// scope check downstream of it, both assume can never happen.
func TestConcurrentCodeExchangesForOneClientSerialize(t *testing.T) {
	o := setupOAuth(t)

	codeA := o.authorize(t, url.Values{"scope": {"read write"}})
	codeB := o.authorize(t, url.Values{"scope": {"read write"}})

	a, b := o.concurrentTokenExchange(codeA, codeB)

	for name, r := range map[string]renewal{"first exchange": a, "second exchange": b} {
		if r.err != nil {
			t.Fatalf("%s failed on the interleaving rather than the rule: %v", name, r.err)
		}
	}

	assertOwnerCount(t, o, 1,
		`SELECT count(*) FROM oauth_grant WHERE client_id = $1 AND revoked_at IS NULL`, o.clientID)
}
