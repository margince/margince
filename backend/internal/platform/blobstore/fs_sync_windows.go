// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore

// syncDir is a no-op on Windows, and the reason is the platform's rather than a
// gap left here.
//
// A directory handle that can be flushed needs FILE_FLAG_BACKUP_SEMANTICS, which
// os.Open does not request — and FlushFileBuffers on a volume handle asks the
// whole volume to flush, which is not this package's decision to take on behalf
// of an installation. NTFS journals the metadata change that a rename is, so the
// rename is not the durability gap here that it is on a filesystem which may
// reorder it.
//
// Named as a function rather than skipped at the call site so the unix path
// cannot be read as optional.
func syncDir(_ string) error { return nil }
