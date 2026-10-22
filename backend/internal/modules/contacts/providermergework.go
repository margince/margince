// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// WithMergeSignalReader retains the owning module’s policy on tool previews.
func (p *Provider) WithMergeSignalReader(read MergeSignalReader) *Provider {
	p.store.WithMergeSignalReader(read)
	return p
}

// WithMergeCommitmentReader retains the owning module’s policy on tool previews.
func (p *Provider) WithMergeCommitmentReader(read MergeCommitmentReader) *Provider {
	p.store.WithMergeCommitmentReader(read)
	return p
}

// WithMergeNoticeReader retains the owning module’s policy on tool previews.
func (p *Provider) WithMergeNoticeReader(read MergeNoticeReader) *Provider {
	p.store.WithMergeNoticeReader(read)
	return p
}
