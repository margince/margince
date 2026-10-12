// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// ExtractorFloor is the confidence below which both commitment extractors drop
// a reading, for the threshold gate, which must live outside this package
// because the certification records are read through aicert, which imports it.
var ExtractorFloor = min(extractConfidenceFloor, transcriptConfidenceFloor)
