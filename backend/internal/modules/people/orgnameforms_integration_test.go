// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

import "testing"

// The Vietnamese market against a live database.
//
// The unit tests ask the scorer directly. This asks the LADDER — the trigram
// candidate query, then the gate, then the score — which is the only place two
// of these claims can be decided at all: whether the candidate query RETURNS
// the incumbent, and whether Go and Postgres fold a name the same way. A name
// that never comes back from the query scores nothing and reads exactly like a
// name that scored badly.
func TestVietnameseCompaniesResolveThroughTheLadder(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// Seeded WITHOUT domains on purpose. exactOrgByDomain is tier 1 and returns
	// before any name is looked at, so a shared domain would decide every case
	// below and the name tiers would go untested.
	incumbents := []string{
		"CÔNG TY CỔ PHẦN TẬP ĐOÀN HÒA PHÁT",
		"CÔNG TY CỔ PHẦN ĐẦU TƯ XÂY DỰNG RICONS",
		"CÔNG TY TRÁCH NHIỆM HỮU HẠN THƯƠNG MẠI VĨNH PHÁT",
		"CÔNG TY CỔ PHẦN CÔNG NGHỆ CMC",
		"CTCP Sữa Việt Nam",
		"CÔNG TY CỔ PHẦN ĐẦU TƯ NAM LONG",
		"CÔNG TY TNHH MỘT THÀNH VIÊN HÒA BÌNH",
	}
	for _, name := range incumbents {
		if _, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
			DisplayName: name, Source: "manual",
		}); err != nil {
			t.Fatalf("seeding %q: %v", name, err)
		}
	}

	// None of these is any of the above, and each shares a legal form with all
	// of them — which is what used to be enough to collide.
	for _, name := range []string{
		"CÔNG TY CỔ PHẦN ĐẦU TƯ XÂY DỰNG TRUNG NAM",
		"CÔNG TY TNHH THƯƠNG MẠI DỊCH VỤ TÂN HIỆP PHÁT",
		"CÔNG TY CỔ PHẦN CÔNG NGHỆ FPT",
		"TẬP ĐOÀN VINGROUP",
	} {
		m := e.dedupeOrgInTx(ctx, t, OrganizationCandidate{DisplayName: name})
		if m.Decision != DecisionNoMatch {
			t.Errorf("%q collided (decision %s, confidence %.4f) — no company of "+
				"that name is in the estate", name, m.Decision, m.Confidence)
		}
	}

	// THE RECALL CLAIM, and the reason this tier uses word similarity. Each of
	// these is a bare brand whose own registered name is seeded above, and each
	// scores below the 0.3 trigram limit against it as a whole string: the
	// candidate query never returned the incumbent, so the duplicate could not
	// be found however well the scorer worked.
	for _, name := range []string{"Hòa Bình", "Sữa Việt Nam", "Nam Long"} {
		m := e.dedupeOrgInTx(ctx, t, OrganizationCandidate{DisplayName: name})
		if m.Decision != DecisionFuzzyReview {
			t.Errorf("%q got %s (confidence %.4f), want fuzzy_review — its own "+
				"company is in the estate under its registered name",
				name, m.Decision, m.Confidence)
		}
	}

	// GO AND POSTGRES MUST FOLD ONE NAME THE SAME WAY. "Đ" has no canonical
	// decomposition, so Go's unaccenting pass leaves it where f_unaccent maps it
	// to "d" — a name typed on a keyboard without Vietnamese diacritics has to
	// reach the same company as the name typed with them.
	ascii := e.dedupeOrgInTx(ctx, t, OrganizationCandidate{
		DisplayName: "Cong ty co phan dau tu Nam Long",
	})
	if ascii.Decision != DecisionFuzzyReview {
		t.Errorf("the ASCII spelling got %s (confidence %.4f), want fuzzy_review "+
			"— it is the seeded company written without diacritics",
			ascii.Decision, ascii.Confidence)
	}

	// A LONG LEGAL FORM MUST NOT HIDE A SHORT BRAND. The candidate query
	// searches whole strings, and "Perseroan Terbatas IBM" against a stored
	// "IBM" scores 0.174 that way — under the trigram limit, so the incumbent
	// was never returned and the duplicate could not be found however well the
	// scorer worked. The query searches the stripped brand as well for exactly
	// this.
	if _, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "IBM", Source: "manual",
	}); err != nil {
		t.Fatalf("seeding the short-brand incumbent: %v", err)
	}
	longForm := e.dedupeOrgInTx(ctx, t, OrganizationCandidate{
		DisplayName: "Perseroan Terbatas IBM",
	})
	if longForm.Decision != DecisionFuzzyReview {
		t.Errorf("a name whose legal form dwarfs its brand got %s (confidence "+
			"%.4f), want fuzzy_review — its own company is seeded",
			longForm.Decision, longForm.Confidence)
	}

	// And tier 1 still outranks every name judgement: a shared domain is an
	// exact key, whatever the two names look like.
	withDomain, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "TẬP ĐOÀN ĐIỆN LỰC VIỆT NAM", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "evn.com.vn", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the domain-carrying company: %v", err)
	}
	sharedDomain := e.dedupeOrgInTx(ctx, t, OrganizationCandidate{
		DisplayName: "HỢP TÁC XÃ NÔNG NGHIỆP ĐẠI THÀNH",
		Domains:     []string{"evn.com.vn"},
	})
	if sharedDomain.Decision != DecisionExactCollision {
		t.Fatalf("a shared domain got %s, want exact_collision — the domain tier "+
			"decides before any name is read", sharedDomain.Decision)
	}
	if sharedDomain.OrganizationID.String() != withDomain.Id.String() {
		t.Fatalf("the domain tier named %s, want the company holding the domain (%s)",
			sharedDomain.OrganizationID, withDomain.Id)
	}
}
