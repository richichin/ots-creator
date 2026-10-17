// Command kvcheck opens a client with the editor codecs and checks that the
// dat survives a load/save cycle unchanged. It is a development tool: point it
// at a folder holding Tibia.dat, Tibia.spr and Tibia.otfi.
//
//	go run ./cmd/kvcheck "C:\path\to\client"
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/nekiro/ots-creator/internal/client"
	"github.com/nekiro/ots-creator/internal/dat"
	"github.com/nekiro/ots-creator/internal/project"
	"github.com/nekiro/ots-creator/internal/thing"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: kvcheck <client folder with dat, spr and otfi>")
		os.Exit(2)
	}
	dir := os.Args[1]
	datPath := filepath.Join(dir, "Tibia.dat")
	sprPath := filepath.Join(dir, "Tibia.spr")

	otfiBytes, err := os.ReadFile(filepath.Join(dir, "Tibia.otfi"))
	if err != nil {
		fail("the otfi next to the dat is required: %v", err)
	}
	o, err := client.ParseOTFI(otfiBytes)
	if err != nil {
		fail("otfi: %v", err)
	}
	f := o.Features

	p, err := project.Open(project.OpenOptions{DatPath: datPath, SprPath: sprPath, Features: f})
	if err != nil {
		fail("project.Open: %v", err)
	}
	info := p.Info()
	fmt.Printf("version    : %s (%d), dat 0x%X spr 0x%X\n", info.Version.Name, info.Version.Value, info.Version.DatSignature, info.Version.SprSignature)
	fmt.Printf("format     : %s (%d properties supported)\n", info.FormatLabel, len(info.Supported))
	fmt.Printf("counts     : items=%d outfits=%d effects=%d missiles=%d\n", info.Counts.Items, info.Counts.Outfits, info.Counts.Effects, info.Counts.Missiles)
	fmt.Printf("features   : extended=%v transparency=%v animations=%v groups=%v size=%d\n",
		info.Features.Extended, info.Features.Transparency, info.Features.ImprovedAnimations, info.Features.FrameGroups, info.Features.SpriteSize)

	// One item with every kind of payload the format has.
	it, err := p.Thing(thing.CategoryItem, 110)
	if err != nil {
		fail("item 110: %v", err)
	}
	fmt.Printf("item 110   : groundSpeed=%d flags=%v\n", it.Props.GroundSpeed, describe(it.Props))
	for _, s := range it.NpcSales {
		fmt.Printf("  npc      : %q in %q, npc pays %d, player pays %d\n", s.Name, s.Location, s.BuyPrice, s.SalePrice)
	}
	if ex, ok := it.Extra.(*dat.Thing13); ok {
		fmt.Printf("  private  : u16=%v u32=%v bool=%v list=%v\n", ex.U16, ex.U32, ex.Bool, ex.List)
	}
	g := it.FrameGroups[0]
	fmt.Printf("  layout   : %dx%d frames=%d sprites=%v\n", g.Width, g.Height, g.Frames, g.Sprites)
	for _, sid := range g.Sprites {
		if sid == 0 {
			continue
		}
		px, err := p.SpritePixels(sid)
		if err != nil {
			fail("sprite %d: %v", sid, err)
		}
		lit := 0
		for _, v := range px {
			if v != 0 {
				lit++
			}
		}
		fmt.Printf("  sprite %d : %d bytes, %d non-zero\n", sid, len(px), lit)
		break
	}

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
	npcs, entries, extra, diffs := 0, 0, 0, 0
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
				entries += len(a.NpcSales)
			}
			if a.Props != b.Props || !reflect.DeepEqual(a.NpcSales, b.NpcSales) ||
				!reflect.DeepEqual(a.Extra, b.Extra) || !reflect.DeepEqual(a.FrameGroups, b.FrameGroups) {
				diffs++
				if diffs <= 5 {
					fmt.Printf("  DIFF at %s %d\n", c, a.ID)
				}
			}
		}
	}
	fmt.Printf("\nrewrite    : %d bytes (original %d, same size=%v, identical=%v)\n",
		len(out), len(raw), len(out) == len(raw), reflect.DeepEqual(append([]byte(nil), out...), raw))
	fmt.Printf("npc lists  : %d things (%d entries)\n", npcs, entries)
	fmt.Printf("private    : %d things\n", extra)
	fmt.Printf("differences: %d\n", diffs)
	if diffs != 0 {
		os.Exit(1)
	}
	fmt.Println("\nOK: the client loads and is written back without losing data")
}

func describe(p thing.Properties) []string {
	var on []string
	add := func(name string, v bool) {
		if v {
			on = append(on, name)
		}
	}
	add("ground", p.Ground)
	add("multiUse", p.MultiUse)
	add("pickupable", p.Pickupable)
	add("light", p.HasLight)
	add("market", p.IsMarket)
	add("cloth", p.Cloth)
	add("usable", p.Usable)
	add("unpassable", p.Unpassable)
	add("unmoveable", p.Unmoveable)
	return on
}

func fail(format string, a ...any) {
	fmt.Printf("ERROR: "+format+"\n", a...)
	os.Exit(1)
}
