// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

import (
	"encoding/binary"
	"fmt"
)

// maxJPEGBlockScans bounds decode work, because image/jpeg walks every 8x8 block once per
// scan. It admits 64 scans of the largest canvas Decode takes, so a small picture may carry
// far more scans than the about 10 a progressive encoder writes.
const maxJPEGBlockScans = maxSourcePixels

// jpegScanCap is how many scans a width x height JPEG may carry within maxJPEGBlockScans.
func jpegScanCap(width, height int) int {
	blocks := ((width + 7) / 8) * ((height + 7) / 8)
	return max(1, maxJPEGBlockScans/blocks)
}

// Marker codes from ITU-T T.81 Table B.1.
const (
	jpegSOI  = 0xd8
	jpegEOI  = 0xd9
	jpegSOS  = 0xda
	jpegRST0 = 0xd0
	jpegRST7 = 0xd7
)

// jpegMagic is the prefix image.Decode routes to the JPEG decoder.
var jpegMagic = []byte{0xff, jpegSOI}

// checkJPEGScans refuses a JPEG with more scans than jpegScanCap allows its canvas, or one
// whose marker stream breaks off before its end-of-image marker. Between segments it skips
// bytes the way image/jpeg resynchronizes, so it counts every scan the decoder would run.
func checkJPEGScans(src []byte, width, height int) error {
	scanCap := jpegScanCap(width, height)
	scans := 0
	for pos := len(jpegMagic); ; {
		marker, at, found := nextJPEGMarker(src, pos)
		if !found {
			return fmt.Errorf("%w: the JPEG ends before its end-of-image marker", ErrUnsupported)
		}
		if marker == jpegEOI {
			return nil
		}
		length := 0
		if at+2 <= len(src) {
			length = int(binary.BigEndian.Uint16(src[at:]))
		}
		if length < 2 || at+length > len(src) {
			return fmt.Errorf("%w: a JPEG segment length does not fit the file", ErrUnsupported)
		}
		if marker == jpegSOS {
			scans++
			if scans > scanCap {
				return fmt.Errorf("%w: the JPEG carries more than the %d scans its canvas allows", ErrUnsupported, scanCap)
			}
		}
		pos = at + length
	}
}

// nextJPEGMarker returns the first marker at or after pos and the offset just
// past it. Stuffed zeros, fill bytes and restart markers belong to the data
// around them, so they never end the search.
func nextJPEGMarker(src []byte, pos int) (marker byte, after int, found bool) {
	for i := pos; i+1 < len(src); i++ {
		if src[i] != 0xff {
			continue
		}
		code := src[i+1]
		if code == 0x00 || code == 0xff || (jpegRST0 <= code && code <= jpegRST7) {
			continue
		}
		return code, i + 2, true
	}
	return 0, 0, false
}
