package totp

import (
	"fmt"
	"strings"

	"rsc.io/qr"
)

// QRCodeSVG renders text as a QR code in a self-contained SVG document.
// The modules are drawn as a single path so the markup stays small, and the
// light background is explicit so the code scans in dark mode too.
func QRCodeSVG(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", fmt.Errorf("totp: encode QR: %w", err)
	}

	const quiet = 4 // quiet zone in modules, as required by the QR spec
	size := code.Size + 2*quiet

	var path strings.Builder
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}

	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" aria-label="QR code for authenticator setup">`+
			`<rect width="%d" height="%d" fill="#ffffff"/><path d="%s" fill="#000000"/></svg>`,
		size, size, size, size, path.String(),
	), nil
}
