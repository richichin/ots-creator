// Command kvcheck opens a client with the editor codecs and checks that the
// dat survives a load/save cycle unchanged. It is a development tool: point it
// at a client folder (or at its dat file).
//
//	go run ./cmd/kvcheck "C:\path\to\client"
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/nekiro/ots-creator/internal/client"
	"github.com/nekiro/ots-creator/internal/dat"
	"github.com/nekiro/ots-creator/internal/project"
	"github.com/nekiro/ots-creator/internal/thing"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: kvcheck <client folder or dat file>")
		os.Exit(2)
	}
	datPath, err := findDat(os.Args[1])
	if err != nil {
		fail("%v", err)
	}
	dir := filepath.Dir(datPath)
	base := strings.TrimSuffix(filepath.Base(datPath), filepath.Ext(datPath))
	sprPath := filepath.Join(dir, base+".spr")

	// The otfi is optional: without it the defaults of the detected version are
	// used, which is what the editor does.
	f := client.Features{}
	if b, err := os.ReadFile(filepath.Join(dir, base+".otfi")); err == nil {
		o, err := client.ParseOTFI(b)
		if err != nil {
			fail("otfi: %v", err)
		}
		f = o.Features
		if o.SpriteDataSize != 0 && o.SpriteDataSize != f.ExpectedSpriteDataSize() {
			fail("otfi declares %d bytes per sprite, sprite-size %d with transparency %t means %d",
				o.SpriteDataSize, f.SpriteSize, f.Transparency, f.ExpectedSpriteDataSize())
		}
	} else {
		fmt.Printf("files      : %s, %s (no otfi, version defaults)\n", filepath.Base(datPath), filepath.Base(sprPath))
	}

	p, err := project.Open(project.OpenOptions{DatPath: datPath, SprPath: sprPath, Features: f})
	if err != nil {
		fail("project.Open: %v", err)
	}
	info := p.Info()
	fmt.Printf("version    : %s (%d), dat 0x%X spr 0x%X\n", info.Version.Name, info.Version.Value, info.Version.DatSignature, info.Version.SprSignature)
	fmt.Printf("format     : %s (%d properties supported)\n", info.FormatLabel, len(info.Supported))
	fmt.Printf("counts     : items=%d outfits=%d effects=%d missiles=%d sprites=%d\n",
		info.Counts.Items, info.Counts.Outfits, info.Counts.Effects, info.Counts.Missiles, info.Counts.Sprites)
	fmt.Printf("features   : extended=%v transparency=%v animations=%v groups=%v size=%d\n",
		info.Features.Extended, info.Features.Transparency, info.Features.ImprovedAnimations, info.Features.FrameGroups, info.Features.SpriteSize)

	// A few things that should have sprites, and whether they decode.
	blank, empty, shown := 0, 0, 0
	for _, c := range thing.Categories {
		for _, t := range p.ThingRange(c, c.MinID(), c.MinID()+3000) {
			if t == nil || len(t.FrameGroups) == 0 {
				continue
			}
			ids := t.SpriteIDs()
			nonZero := 0
			for _, id := range ids {
				if id != 0 {
					nonZero++
				}
			}
			if nonZero == 0 {
				blank++
				continue
			}
			if shown < 3 {
				shown++
				sid := 0
				for _, id := range ids {
					if id != 0 {
						sid = int(id)
						break
					}
				}
				px, err := p.SpritePixels(uint32(sid))
				lit := 0
				if err == nil {
					for _, v := range px {
						if v != 0 {
							lit++
						}
					}
				} else {
					fmt.Printf("  %s %d: sprite %d ERROR %v\n", c, t.ID, sid, err)
				}
				if err == nil && lit == 0 {
					empty++
				}
				g := t.FrameGroups[0]
				fmt.Printf("  %s %d: %dx%d frames=%d sprites=%d, sprite %d -> %d bytes, %d lit\n",
					c, t.ID, g.Width, g.Height, g.Frames, len(ids), sid, len(px), lit)
			}
		}
	}
	fmt.Printf("first 3000 of each category: %d things with no sprite id, %d sampled sprites fully transparent\n", blank, empty)

	// Full dat round trip: decode, encode, decode again and compare the model.
	raw, err := os.ReadFile(datPath)
	if err != nil {
		fail("%v", err)
	}
	opts := dat.OptionsFor(info.Version.Value, f)
	first, err := dat.Decode(raw, opts)
	if err != nil {
		fail("decode: %v", err)
	}
	out, err := dat.Encode(first, opts)
	if err != nil {
		fail("encode: %v", err)
	}
	second, err := dat.Decode(out, opts)
	if err != nil {
		fail("re-decode: %v", err)
	}
	npcs, extra, diffs := 0, 0, 0
	for _, c := range thing.Categories {
		if len(first.Things[c]) != len(second.Things[c]) {
			fail("%s: %d things before, %d after", c, len(first.Things[c]), len(second.Things[c]))
		}
		for i, a := range first.Things[c] {
			b := second.Things[c][i]
			if _, ok := a.Extra.(*dat.Thing13); ok {
				extra++
			}
			if len(a.NpcSales) > 0 {
				npcs++
			}
			if a.Props != b.Props || !reflect.DeepEqual(a.NpcSales, b.NpcSales) ||
				!reflect.DeepEqual(a.Extra, b.Extra) || !reflect.DeepEqual(a.FrameGroups, b.FrameGroups) {
				diffs++
				if diffs <= 3 {
					fmt.Printf("  DIFF at %s %d\n    before %s\n    after  %s\n", c, a.ID, groupSummary(a), groupSummary(b))
				}
			}
		}
	}
	fmt.Printf("\nrewrite    : %d bytes (original %d, same size=%v, identical=%v)\n",
		len(out), len(raw), len(out) == len(raw), reflect.DeepEqual(append([]byte(nil), out...), raw))
	fmt.Printf("npc lists  : %d things\nprivate    : %d things\ndifferences: %d\n", npcs, extra, diffs)
	if diffs != 0 {
		os.Exit(1)
	}
	fmt.Println("\nOK: the client loads and is written back without losing data")
}

// findDat resolves the dat of a client: the path itself, or the first *.dat in
// a folder, like the editor does.
func findDat(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return path, nil
	}
	matches, err := filepath.Glob(filepath.Join(path, "*.dat"))
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("no *.dat in %s", path)
	}
	return matches[0], nil
}

// groupSummary describes the frame groups of a thing for diff messages.
func groupSummary(t *thing.Thing) string {
	var b strings.Builder
	for _, g := range t.FrameGroups {
		fmt.Fprintf(&b, "[type=%d %dx%d size=%d layers=%d px=%d py=%d pz=%d frames=%d sprites=%d]",
			g.Type, g.Width, g.Height, g.ExactSize, g.Layers, g.PatternX, g.PatternY, g.PatternZ, g.Frames, len(g.Sprites))
	}
	return b.String()
}
func fail(format string, a ...any) {
	fmt.Printf("ERROR: "+format+"\n", a...)
	os.Exit(1)
}
