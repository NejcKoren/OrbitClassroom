// genicon renders the Orbit logo as PNG files for the Windows .exe icons.
//
//	go run ./tools/genicon <outdir>
package main

import (
	"fmt"
	"image/png"
	"orbit/internal/logo"
	"os"
	"path/filepath"
)

func main() {
	out := "winres"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	os.MkdirAll(out, 0o755)
	for _, s := range []int{16, 20, 24, 32, 40, 48, 64, 128, 256} {
		f, err := os.Create(filepath.Join(out, fmt.Sprintf("icon%d.png", s)))
		if err != nil {
			panic(err)
		}
		png.Encode(f, logo.Render(s, logo.BadgeNone))
		f.Close()
	}
}
