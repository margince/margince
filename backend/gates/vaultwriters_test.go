// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every writer of the installation's ciphertext store records its act somewhere.
//
// `internal/platform/keyvault` is waived in moduleaudits_test.go from writing an
// audit row itself, and the reason is that keyvault is a SEAM: callers receive a
// Vault from compose and perform the domain act in their own module, which is
// where that act's history belongs. Filing the vault row as well would record one
// change twice, the second time under an entity type that is a secret's
// reference.
//
// THAT WAIVER IS MODULE-WIDE, WHICH IS WHY THIS CENSUS EXISTS. A waiver on the
// package covers every write that reaches it — including one added tomorrow that
// records nothing at all. An enumeration in that comment could not bound it: no
// gate holds a comment true, so it goes stale the first time a caller is added.
// The enumeration lives here instead, where a NEW writer fails until somebody
// gives it a verdict.
//
// It does not judge WHERE a caller records — audit_log, system_log, or a settings
// write that carries the audit row are all legitimate for different shapes. It
// judges that somebody has looked.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// vaultPut matches a call that seals bytes into the key vault. The receiver name
// varies (`r.vault`, `s.vault`, `b.vault`, a bare `vault` parameter), so the
// selector is what identifies it.
var vaultPut = regexp.MustCompile(`\bvault\.Put\(`)

// vaultWriters records, for each non-test caller that seals a secret, the ledger
// its act lands in. TestEveryKeyVaultWriterHasAVerdict below is what holds it
// complete — a path here with no call, or a call with no path here, both fail,
// because a stale entry is how a census stops describing the tree.
//
// gatekit:fixture the ledger each caller records in — expected data about the
// tree, not a cost anybody is choosing to pay. The single exception is marked
// NOTHING and carries its issue on the line above it.
var vaultWriters = map[string]string{
	// The domain act is connecting or reconnecting a channel, audited under
	// those verbs by the store that performs it.
	"internal/modules/capture/registry_grant.go": "audit_log, under the connect/disconnect verbs",
	"internal/modules/capture/channelconn.go":    "audit_log, under the channel-connection verbs",
	"internal/modules/integrations/connect.go":   "audit_log, under the integration connect verbs",
	"internal/modules/overlay/connection.go":     "audit_log, under the overlay connection verbs",

	// Sealed alongside a SETTINGS write, which is what carries the audit row
	// (settings.SetTx -> storekit.Audit, under the setting's own verb). The two
	// boot seals each stamp their own actor so they stay distinguishable there.
	"internal/compose/deploymentsecretseal.go":   "audit_log via the settings write, actor deployment-secret-seal",
	"internal/compose/providerkeyseal.go":        "audit_log via the settings write, actor routing-seed",
	"internal/modules/ai/providerkeystore.go":    "audit_log via the settings write, under the ProviderKeys verb",
	"internal/modules/capture/googleappstore.go": "audit_log via the settings write, under capture settings' update verb",

	// The declared no-domain-fact posture: bytes move, nothing changes meaning,
	// so it records operationally rather than as an entity change.
	"internal/platform/extsecrets/store.go": "system_log — the explicit no-domain-fact posture",
	// A provider replaced the durable credential during a background pull.
	// Nobody decided it, so there is no domain change to audit and no actor to
	// attribute one to — the operational fact is what makes an aged-out mailbox
	// diagnosable. Written in the same transaction as the ref it describes.
	"internal/modules/capture/registry_rotation.go": "system_log, action capture.credential_rotated — the no-domain-fact posture",

	// RECORDS NOTHING, and named rather than covered. The boot-time relocation of
	// a legacy credential into the vault writes no audit row, no system log and no
	// event. Whether a raw relocation should record at all is capture's posture to
	// decide, so it is filed rather than assumed:
	// https://github.com/margince/margince/issues/2552
	"internal/modules/capture/credentialbackfill.go": "NOTHING — #2552",
}

func TestEveryKeyVaultWriterHasAVerdict(t *testing.T) {
	t.Parallel()
	found := map[string]bool{}
	root := "internal"
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if vaultPut.Match(body) {
			found[filepath.ToSlash(path)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	// NOT a tolerated zero: the tree carries vault writers today, so an empty
	// result means the pattern stopped matching rather than that nobody seals a
	// secret — and an empty census reports PASS.
	if len(found) == 0 {
		t.Fatal("no vault.Put call sites found, but the tree has several — the pattern has gone " +
			"blind, and a census that matches nothing passes while checking nothing")
	}

	for path := range found {
		if _, declared := vaultWriters[path]; !declared {
			t.Errorf("%s seals a secret into the key vault and has no entry in vaultWriters.\n"+
				"internal/platform/keyvault is waived from writing history of its own because each "+
				"CALLER records its act; a new caller has to say which ledger that is. Add the entry "+
				"with the ledger it writes — or, if it writes none, say so and file the gap rather "+
				"than letting the module-wide waiver absorb it.", path)
		}
	}
	for path := range vaultWriters {
		if !found[path] {
			t.Errorf("vaultWriters declares %s, which no longer calls vault.Put.\n"+
				"Drop the entry: a census carrying paths that have moved stops describing the tree, "+
				"and the next reader trusts it anyway.", path)
		}
	}
}
