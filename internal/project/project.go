// Package project holds an editing session over one client (dat + spr):
// thing and sprite storage, undo/redo and compilation.
package project

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/nekiro/ots-creator/internal/assets"
	"github.com/nekiro/ots-creator/internal/client"
	"github.com/nekiro/ots-creator/internal/dat"
	"github.com/nekiro/ots-creator/internal/spr"
	"github.com/nekiro/ots-creator/internal/thing"
)

// ErrUnknownVersion is returned by Open when signatures match no known
// client and no version was given.
var ErrUnknownVersion = errors.New("unknown client version")

// Info describes the loaded client.
type Info struct {
	Format      Format          `json:"format"`
	Version     client.Version  `json:"version"`
	Features    client.Features `json:"features"`
	DatPath     string          `json:"datPath"`
	SprPath     string          `json:"sprPath"`
	Counts      Counts          `json:"counts"`
	Changed     bool            `json:"changed"`
	CanUndo     bool            `json:"canUndo"`
	CanRedo     bool            `json:"canRedo"`
	FormatLabel string          `json:"formatLabel"`
	// Supported lists property names the current format can store.
	Supported []string `json:"supported"`
	// ItemsOTB is the attached items.otb, kept in sync when compiling.
	ItemsOTB *OTBInfo `json:"itemsOtb"`
}

// Counts holds the highest id of each category and the sprite count.
type Counts struct {
	Items    uint32 `json:"items"`
	Outfits  uint32 `json:"outfits"`
	Effects  uint32 `json:"effects"`
	Missiles uint32 `json:"missiles"`
	Sprites  uint32 `json:"sprites"`
}

// Project is safe for concurrent use.
type Project struct {
	mu       sync.RWMutex
	version  client.Version
	features client.Features
	format   Format
	// detectedFormat is the dat flag table that fitted the file when it was
	// opened, 0 for a project that was never read from disk. It wins over the
	// version default so a client whose version maps to another table keeps
	// being written the way it was read.
	detectedFormat int
	// datPath is the asset folder for FormatAssets.
	datPath string
	sprPath string
	things  *dat.File
	sprites *spriteStore
	history history
	// unsaved is set for a client that was never written to disk; edits
	// are tracked by the history instead.
	unsaved bool
	delta   deltaSet
	assets  *assetState // FormatAssets only
	otb     *itemsOTB   // attached items.otb, nil when none
	// base holds the items as they are on disk and moves maps their ids
	// to the ids they were moved to since (see otb.go).
	base  []*thing.Thing
	moves map[uint32]uint32
}

// OpenOptions control how a client is opened.
type OpenOptions struct {
	DatPath string
	SprPath string
	// Version overrides signature detection when not nil.
	Version *client.Version
	// Features are combined with the defaults of the version.
	Features client.Features
}

// Open loads a dat/spr pair.
func Open(o OpenOptions) (*Project, error) {
	datBytes, err := os.ReadFile(o.DatPath)
	if err != nil {
		return nil, err
	}
	sprBytes, err := os.ReadFile(o.SprPath)
	if err != nil {
		return nil, err
	}
	if len(datBytes) < 4 || len(sprBytes) < 4 {
		return nil, fmt.Errorf("dat or spr file is too small")
	}
	datSig := le32(datBytes)
	sprSig := le32(sprBytes)
	var v client.Version
	if o.Version != nil {
		v = *o.Version
	} else {
		var ok bool
		v, ok = client.FindBySignatures(datSig, sprSig)
		if !ok {
			return nil, fmt.Errorf("%w: dat 0x%X, spr 0x%X", ErrUnknownVersion, datSig, sprSig)
		}
	}
	f := o.Features
	f.ApplyVersionDefaults(v.Value)
	things, format, err := dat.Detect(datBytes, dat.Options{Format: client.MetadataFormat(v.Value), Features: f})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(o.DatPath), err)
	}
	sf, err := spr.Open(sprBytes, spr.Options{Extended: f.Extended, Transparent: f.Transparency, SpriteSize: f.SpriteSize})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(o.SprPath), err)
	}
	// A client exported without an otfi decodes every sprite with the wrong
	// number of channels when the transparency default does not match the file,
	// which shows up as "pixel overflow" on a blank preview. Ask the sprites.
	if t, ok := detectsTransparency(sf, things, f); ok {
		f.Transparency = t
	}
	p := &Project{
		version:        v,
		features:       f,
		format:         FormatDat,
		detectedFormat: format,
		datPath:        o.DatPath,
		sprPath:        o.SprPath,
		things:         things,
		sprites:        newSpriteStore(sprFile(sf), f.SpriteSize, f.Transparency),
	}
	p.resetBase()
	return p, nil
}

// transparentSampleSprites is the number of sprites probed to tell whether a
// spr file keeps an alpha channel.
const transparentSampleSprites = 64

// detectsTransparency reports whether the sprites of a client only decode with
// the other transparency a file could have, and which one that is. Dat and spr
// files do not say it (the otfi does), so a client without one is read with the
// defaults of its version, which are wrong for most exported servers.
func detectsTransparency(sf *spr.File, things *dat.File, f client.Features) (bool, bool) {
	ids := sampleSpriteIDs(things, transparentSampleSprites)
	if len(ids) == 0 || decodesSprites(sf, ids) {
		return f.Transparency, false
	}
	flipped := !f.Transparency
	if !decodesSprites(sf.WithTransparency(flipped), ids) {
		return f.Transparency, false
	}
	return flipped, true
}

// decodesSprites reports whether every sampled sprite decodes with the settings
// of sf: with the wrong number of channels the first ones already overflow.
func decodesSprites(sf *spr.File, ids []uint32) bool {
	for _, id := range ids {
		if _, err := sf.Pixels(id); err != nil {
			return false
		}
	}
	return true
}

// sampleSpriteIDs collects up to n sprite ids of the first things, skipping the
// empty slots (id 0).
func sampleSpriteIDs(file *dat.File, n int) []uint32 {
	out := make([]uint32, 0, n)
	for _, c := range thing.Categories {
		for _, t := range file.Things[c] {
			if t == nil {
				continue
			}
			for _, id := range t.SpriteIDs() {
				if id == 0 {
					continue
				}
				out = append(out, id)
				if len(out) >= n {
					return out
				}
			}
		}
	}
	return out
}

// New creates an empty client with one item, outfit, effect and missile,
// like ObjectBuilder does.
func New(v client.Version, f client.Features) *Project {
	f.ApplyVersionDefaults(v.Value)
	things := dat.NewFile(v.DatSignature)
	for _, c := range thing.Categories {
		things.Things[c] = []*thing.Thing{thing.New(c.MinID(), c)}
	}
	return &Project{
		version:  v,
		features: f,
		format:   FormatDat,
		things:   things,
		sprites:  newSpriteStore(nil, f.SpriteSize, f.Transparency),
		unsaved:  true,
	}
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// Info returns a snapshot of the project state.
func (p *Project) Info() Info {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.infoLocked()
}

// datFormat returns the dat flag table to use for a client version: the one
// the file was opened with when it is the same client, the version default
// otherwise (it is a different client being written).
func (p *Project) datFormat(v client.Version) int {
	if p.detectedFormat != 0 && v.Value == p.version.Value {
		return p.detectedFormat
	}
	return client.MetadataFormat(v.Value)
}

func (p *Project) infoLocked() Info {
	var otb *OTBInfo
	if p.otb != nil {
		in := p.otb.info
		otb = &in
	}
	table := dat.Table(p.datFormat(p.version))
	label, supported := table.Name(), table.Supported()
	if p.format == FormatAssets {
		label, supported = "Protobuf appearances", assets.Supported()
	}
	return Info{
		Format:   p.format,
		Version:  p.version,
		Features: p.features,
		DatPath:  p.datPath,
		SprPath:  p.sprPath,
		Counts: Counts{
			Items:    p.things.MaxID(thing.CategoryItem),
			Outfits:  p.things.MaxID(thing.CategoryOutfit),
			Effects:  p.things.MaxID(thing.CategoryEffect),
			Missiles: p.things.MaxID(thing.CategoryMissile),
			Sprites:  p.sprites.count(),
		},
		Changed:     p.unsaved || !p.history.atClean(),
		CanUndo:     p.history.canUndo(),
		CanRedo:     p.history.canRedo(),
		FormatLabel: label,
		Supported:   supported,
		ItemsOTB:    otb,
	}
}

// SpriteSize returns the sprite edge length in pixels.
func (p *Project) SpriteSize() int { return p.features.SpriteSize }

// CompileOptions select the output of Compile.
type CompileOptions struct {
	// Format is the output format; empty means FormatDat. For FormatAssets
	// DatPath is the output folder and the other fields are ignored.
	Format   Format
	DatPath  string
	SprPath  string
	Version  client.Version
	Features client.Features
	// WriteOTFI also writes an OTClient feature file next to the dat.
	WriteOTFI bool
	// Progress, when set, is called with the work done so far, possibly
	// from several goroutines and very often.
	Progress func(done, total int)
	// Stats, when set, receives what an asset compilation did to the sheets
	// and what happened to the attached items.otb.
	Stats *CompileStats
}

// CompileStats reports the sheets of an asset compilation.
type CompileStats struct {
	// SheetsRemoved counts sheets of the folder that no longer hold a used
	// sprite as they were (dropped, or repacked into new sheets).
	SheetsRemoved int `json:"sheetsRemoved"`
	// Freed is the size of the removed sheets minus the new ones, in bytes.
	Freed int64 `json:"freed"`
	// OTB reports the update of the attached items.otb (nil when none).
	OTB *OTBReport `json:"otb"`
}

// counter reports work done to a CompileOptions.Progress.
type counter struct {
	done   atomic.Int64
	total  int
	report func(done, total int)
}

func newCounter(total int, report func(done, total int)) *counter {
	if report == nil {
		report = func(int, int) {}
	}
	return &counter{total: total, report: report}
}

func (c *counter) tick() { c.report(int(c.done.Add(1)), c.total) }

// countingSource ticks a counter for every sprite read.
type countingSource struct {
	spr.Source
	c *counter
}

func (s countingSource) Compressed(id uint32) ([]byte, error) {
	s.c.tick()
	return s.Source.Compressed(id)
}

// Compile writes the project to disk. Files are written to temporary files
// first and renamed, so a failure never leaves broken files behind.
// Properties unsupported by the target format are dropped; use Warnings to
// list them beforehand.
func (p *Project) Compile(o CompileOptions) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// The items.otb is synced against the items on disk, before they change.
	otbFile, otbData, otbRep := p.prepareOTB()
	if o.Stats != nil {
		o.Stats.OTB = otbRep
	}
	if o.Format == FormatAssets {
		if err := p.compileAssets(o.DatPath, o.Progress, o.Stats); err != nil {
			return err
		}
		p.writeOTB(otbFile, otbData, otbRep)
		p.saved()
		return nil
	}
	f := o.Features
	f.ApplyVersionDefaults(o.Version.Value)
	if f.SpriteSize != p.features.SpriteSize {
		return fmt.Errorf("changing sprite size from %d to %d is not supported", p.features.SpriteSize, f.SpriteSize)
	}
	file := &dat.File{Signature: o.Version.DatSignature, Things: p.things.Things}
	format := p.datFormat(o.Version)
	datBytes, err := dat.Encode(file, dat.Options{Format: format, Features: f})
	if err != nil {
		return err
	}
	var sprBuf bytes.Buffer
	src := p.sprites.source(f.Transparency)
	// Every sprite is read three times: for the size and by both passes of
	// spr.Encode.
	src = countingSource{src, newCounter(3*int(src.Count()), o.Progress)}
	if size, err := encodedSize(src, f.Extended); err == nil {
		sprBuf.Grow(size) // one allocation instead of repeated doubling
	}
	if err := spr.Encode(&sprBuf, o.Version.SprSignature, src, spr.Options{Extended: f.Extended, Transparent: f.Transparency, SpriteSize: f.SpriteSize}); err != nil {
		return err
	}
	if err := writeAtomic(o.DatPath, datBytes); err != nil {
		return err
	}
	if err := writeAtomic(o.SprPath, sprBuf.Bytes()); err != nil {
		return err
	}
	if o.WriteOTFI {
		otfi := client.OTFI{Features: f, MetadataFile: filepath.Base(o.DatPath), SpritesFile: filepath.Base(o.SprPath)}
		// The 13.x clients read the bytes per sprite from the otfi.
		if client.MetadataFormat(o.Version.Value) >= 7 {
			otfi.SpriteDataSize = f.ExpectedSpriteDataSize()
		}
		base := filepath.Join(filepath.Dir(o.DatPath), trimExt(filepath.Base(o.DatPath))+".otfi")
		if err := writeAtomic(base, otfi.Marshal()); err != nil {
			return err
		}
	}
	p.writeOTB(otbFile, otbData, otbRep)
	// Reload sprites from the new file so the in-memory overlay is dropped.
	sf, err := spr.Open(sprBuf.Bytes(), spr.Options{Extended: f.Extended, Transparent: f.Transparency, SpriteSize: f.SpriteSize})
	if err != nil {
		return fmt.Errorf("reopen compiled spr: %w", err)
	}
	p.sprites = newSpriteStore(sprFile(sf), f.SpriteSize, f.Transparency)
	p.format, p.assets = FormatDat, nil
	p.version, p.features = o.Version, f
	p.detectedFormat = format
	p.datPath, p.sprPath = o.DatPath, o.SprPath
	p.things.Signature = o.Version.DatSignature
	p.saved()
	return nil
}

// saved resets change tracking after the files were written.
func (p *Project) saved() {
	p.unsaved = false
	p.history.clear()
	p.resetBase()
	// Version and features (transparency) can change how sprites decode.
	p.delta.everything()
}

// Warnings lists things whose properties would be lost when compiling to
// the given format and version.
func (p *Project) Warnings(format Format, v client.Version) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if format == FormatAssets {
		return p.assetWarnings()
	}
	table := dat.Table(p.datFormat(v))
	var out []string
	extras := 0
	for _, c := range thing.Categories {
		for _, t := range p.things.Things[c] {
			u := table.Unsupported(&t.Props)
			if len(t.NpcSales) > 0 && !table.StoresNpcSales() {
				u = append(u, "npcSales")
			}
			if len(u) > 0 {
				out = append(out, fmt.Sprintf("%s %d: %v", c, t.ID, u))
			}
			if _, ok := t.Extra.(*dat.Thing13); ok {
				extras++
			}
		}
	}
	if extras > 0 && !table.StoresExtraFlags() {
		out = append(out, fmt.Sprintf("%d things store 13.10 flags this format cannot save", extras))
	}
	return out
}

// encodedSize returns the size of the spr file Encode writes for src.
func encodedSize(src spr.Source, extended bool) (int, error) {
	header := 6
	if extended {
		header = 8
	}
	n := src.Count()
	size := header + int(n)*4
	for id := uint32(1); id <= n; id++ {
		c, err := src.Compressed(id)
		if err != nil {
			return 0, err
		}
		if len(c) > 0 {
			size += 5 + len(c)
		}
	}
	return size, nil
}

func trimExt(name string) string { return name[:len(name)-len(filepath.Ext(name))] }

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
