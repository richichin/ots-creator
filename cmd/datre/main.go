package main

import (
	"fmt"
	"os"
	"sort"
)

const maxSpriteID = 761264

type P struct {
	b   []byte
	pos int
	err bool
}

func (p *P) u8() byte {
	if p.pos+1 > len(p.b) {
		p.err = true
		return 0
	}
	v := p.b[p.pos]
	p.pos++
	return v
}
func (p *P) u16() uint16 {
	if p.pos+2 > len(p.b) {
		p.err = true
		return 0
	}
	v := uint16(p.b[p.pos]) | uint16(p.b[p.pos+1])<<8
	p.pos += 2
	return v
}
func (p *P) u32() uint32 {
	if p.pos+4 > len(p.b) {
		p.err = true
		return 0
	}
	v := uint32(p.b[p.pos]) | uint32(p.b[p.pos+1])<<8 | uint32(p.b[p.pos+2])<<16 | uint32(p.b[p.pos+3])<<24
	p.pos += 4
	return v
}
func (p *P) skip(n int) {
	p.pos += n
	if p.pos > len(p.b) {
		p.err = true
	}
}
func (p *P) str() {
	n := int(p.u16())
	if n > 300 || p.pos+n > len(p.b) {
		p.err = true
		return
	}
	p.skip(n)
}

var size = map[byte]int{
	0x00: 2, 0x08: 2, 0x09: 2, 0x16: 4, 0x19: 4, 0x1A: 2, 0x1D: 2, 0x1E: 2,
	0x21: 2, 0x23: 2, 0x27: 0, 0xFE: 0, 0x38: 4, 0x39: 4, 0x2F: 2, 0x3A: 0,
	0x33: 0, 0x31: 2, 0x32: 2, 0x36: 0,
}

var vistosGlobal = map[byte]int{}

func (p *P) flags(cat string, id int) bool {
	ground := 0
	var vistos []byte
	dup := false
	for {
		at := p.pos
		fl := p.u8()
		if p.err {
			fmt.Printf("EOF en %s %d\n", cat, id)
			return false
		}
		if fl == 0xFF {
			return true
		}
		if fl == 0x22 { // market
			p.skip(6)
			p.str()
			p.skip(4)
			if p.err {
				fmt.Printf("EOF en market %s %d\n", cat, id)
				return false
			}
			vistos = append(vistos, fl)
			continue
		}
		if fl == 0x30 { // lista npc
			c := int(p.u16())
			if c > 400 {
				fmt.Printf("npc count %d en %s %d\n", c, cat, id)
				return false
			}
			for i := 0; i < c; i++ {
				p.skip(8)
				p.str()
				p.str()
			}
			if p.err {
				fmt.Printf("EOF en lista npc %s %d\n", cat, id)
				return false
			}
			vistos = append(vistos, fl)
			continue
		}
		if fl == 0x37 { // u32 n + n*u32
			c := int(p.u32())
			if c > 64 {
				fmt.Printf("0x37 count %d en %s %d\n", c, cat, id)
				return false
			}
			p.skip(4 * c)
			if p.err {
				fmt.Printf("EOF en 0x37 %s %d\n", cat, id)
				return false
			}
			vistos = append(vistos, fl)
			continue
		}
		n, ok := size[fl]
		if !ok {
			// hipotesis: payload 0
			vistosGlobal[fl]++
			n = 0
		}
		if n == 0 {
			for _, v := range vistos {
				if v == fl {
					dup = true
				}
			}
			if fl == 0x00 {
				ground++
				if ground > 1 {
					fmt.Printf("doble ground en %s %d offset %d\n", cat, id, at)
					return false
				}
			}
			vistos = append(vistos, fl)
			if dup {
				fmt.Printf("flag repetida 0x%02X en %s %d offset %d\n", fl, cat, id, at)
				return false
			}
			continue
		}
		p.skip(n)
		if p.err {
			fmt.Printf("EOF tras 0x%02X en %s %d\n", fl, cat, id)
			return false
		}
		vistos = append(vistos, fl)
	}
}

func (p *P) groups(cat string, id int) bool {
	withGroups := cat == "outfit"
	gc := 1
	if withGroups {
		gc = int(p.u8())
		if gc < 1 || gc > 8 {
			fmt.Printf("numGrupos %d en %s %d\n", gc, cat, id)
			return false
		}
	}
	for g := 0; g < gc; g++ {
		if withGroups {
			p.skip(1)
		}
		w := int(p.u8())
		h := int(p.u8())
		if w > 1 || h > 1 {
			p.skip(1)
		}
		layers := int(p.u8())
		px := int(p.u8())
		py := int(p.u8())
		pz := int(p.u8())
		frames := int(p.u8())
		if w < 1 || h < 1 || w > 4 || h > 4 || layers < 1 || layers > 8 || px < 1 || px > 16 || py < 1 || py > 16 || pz < 1 || pz > 16 || frames < 1 || frames > 64 {
			fmt.Printf("layout raro en %s %d: %dx%d L%d P%d/%d/%d F%d (offset %d)\n", cat, id, w, h, layers, px, py, pz, frames, p.pos)
			return false
		}
		if frames > 1 {
			p.skip(1 + 4 + 1 + 8*frames)
		}
		total := w * h * layers * px * py * pz * frames
		if total > 8192 {
			fmt.Printf("total %d en %s %d\n", total, cat, id)
			return false
		}
		for i := 0; i < total; i++ {
			sid := p.u32()
			if p.err || sid > maxSpriteID {
				fmt.Printf("sprite id invalido %d en %s %d (offset %d)\n", sid, cat, id, p.pos-4)
				return false
			}
		}
	}
	return !p.err
}

func main() {
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	p := &P{b: data}
	p.u16()
	p.u16()
	var counts [4]uint16
	for i := range counts {
		counts[i] = p.u16()
	}
	cats := []struct {
		name     string
		min, max int
	}{
		{"item", 100, int(counts[0])}, {"outfit", 1, int(counts[1])},
		{"effect", 1, int(counts[2])}, {"missile", 1, int(counts[3])},
	}
	for _, c := range cats {
		for id := c.min; id <= c.max; id++ {
			if !p.flags(c.name, id) {
				fmt.Printf("-- parado en %s %d, offset %d de %d\n", c.name, id, p.pos, len(data))
				return
			}
			if !p.groups(c.name, id) {
				fmt.Printf("-- parado en %s %d, offset %d de %d\n", c.name, id, p.pos, len(data))
				return
			}
		}
		fmt.Printf("%-8s OK: %d (offset %d)\n", c.name, c.max-c.min+1, p.pos)
	}
	fmt.Printf("\nfin=%d len=%d\n", p.pos, len(data))
	if p.pos == len(data) {
		fmt.Println("=== EXACTO: hipotesis confirmada ===")
	} else {
		fmt.Printf("sobran/faltan %d bytes\n", len(data)-p.pos)
	}
	ks := []int{}
	for k := range vistosGlobal {
		ks = append(ks, int(k))
	}
	sort.Ints(ks)
	fmt.Println("flags tratadas como 0 bytes:")
	for _, k := range ks {
		fmt.Printf("  0x%02X (visto %d veces)\n", k, vistosGlobal[byte(k)])
	}
}
