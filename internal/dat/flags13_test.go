package dat

import (
	"reflect"
	"testing"

	"github.com/nekiro/ots-creator/internal/binio"
	"github.com/nekiro/ots-creator/internal/client"
	"github.com/nekiro/ots-creator/internal/thing"
)

// kingsValeThing builds the flags of an item of a 13.10 client, taken from the
// real dat of one: a ground with speed 140, a light, a market name, an NPC
// sale list and the private flags of the format.
func kingsValeThing() []byte {
	w := binio.NewWriter(128)
	w.U8(0x00) // ground
	w.U16(140)
	w.U8(0x0D) // unmoveable
	w.U8(0x16) // light
	w.U16(3)   // level
	w.U16(156) // color
	w.U8(0x22) // market
	w.U16(9)   // category
	w.U16(111) // trade as
	w.U16(111) // show as
	w.U16(13)  // name length
	w.Write([]byte("special flask"))
	w.U16(0)   // restriction profession
	w.U16(0)   // restriction level
	w.U8(0x27) // bones: no payload in this format
	w.U8(0x2B) // unknown flag: no payload, like the client default branch
	w.U8(0x2F) // private u16
	w.U16(5)
	w.U8(0x30) // NPC sale list
	w.U16(1)
	w.U32(2200) // the NPC pays the player
	w.U32(0)    // the player pays the NPC
	w.U16(10)
	w.Write([]byte("Black Bert"))
	w.U16(10)
	w.Write([]byte("Thais City"))
	w.U8(0x33) // private boolean
	w.U8(0x37) // private u32 list
	w.U32(2)
	w.U32(1)
	w.U32(2)
	w.U8(0x38) // private u32
	w.U32(42)
	w.U8(0x3A) // private boolean
	w.U8(0xFF) // last flag
	return w.Bytes()
}

func TestKingsValeFlags(t *testing.T) {
	if got := client.MetadataFormat(1310); got != 7 {
		t.Fatalf("MetadataFormat(1310) = %d, want 7", got)
	}
	th := &thing.Thing{ID: 110, Category: thing.CategoryItem}
	if err := table7.ReadThing(binio.NewReader(kingsValeThing()), th); err != nil {
		t.Fatal(err)
	}
	p := th.Props
	if !p.Ground || p.GroundSpeed != 140 || !p.Unmoveable {
		t.Fatalf("props %+v", p)
	}
	if !p.HasLight || p.LightLevel != 3 || p.LightColor != 156 {
		t.Fatalf("light %d %d", p.LightLevel, p.LightColor)
	}
	if !p.IsMarket || p.Market.Category != 9 || p.Market.TradeAs != 111 || p.Market.Name != "special flask" {
		t.Fatalf("market %+v", p.Market)
	}
	if len(th.NpcSales) != 1 {
		t.Fatalf("npc sales %+v", th.NpcSales)
	}
	if s := th.NpcSales[0]; s.Name != "Black Bert" || s.Location != "Thais City" || s.BuyPrice != 2200 || s.SalePrice != 0 {
		t.Fatalf("npc sale %+v", s)
	}
	ex, ok := th.Extra.(*Thing13)
	if !ok {
		t.Fatalf("extra %T", th.Extra)
	}
	if ex.U16[0x2F] != 5 || ex.U32[0x38] != 42 || !ex.Bool[0x33] || !ex.Bool[0x3A] {
		t.Fatalf("private flags %+v", ex)
	}
	if !reflect.DeepEqual(ex.List[0x37], []uint32{1, 2}) {
		t.Fatalf("list %v", ex.List[0x37])
	}
	if len(ex.Bool) != 2 || len(ex.U16) != 1 || len(ex.U32) != 1 || len(ex.List) != 1 {
		t.Fatalf("flags without payload must not be kept: %+v", ex)
	}

	// Writing the thing back must describe the same thing.
	w := binio.NewWriter(128)
	if err := table7.WriteThing(w, th); err != nil {
		t.Fatal(err)
	}
	got := &thing.Thing{ID: 110, Category: thing.CategoryItem}
	if err := table7.ReadThing(binio.NewReader(w.Bytes()), got); err != nil {
		t.Fatal(err)
	}
	if got.Props != th.Props {
		t.Fatalf("props differ:\n%+v\n%+v", got.Props, th.Props)
	}
	if !reflect.DeepEqual(got.NpcSales, th.NpcSales) {
		t.Fatalf("npc sales differ: %+v %+v", got.NpcSales, th.NpcSales)
	}
	if !reflect.DeepEqual(got.Extra, th.Extra) {
		t.Fatalf("private flags differ: %+v %+v", got.Extra, th.Extra)
	}
}

// TestKingsValeNoBones checks that 0x27 carries no payload: an item with only
// that flag is followed directly by the next flag.
func TestKingsValeNoBones(t *testing.T) {
	th := &thing.Thing{ID: 100, Category: thing.CategoryItem}
	if err := table7.ReadThing(binio.NewReader([]byte{0x27, 0x0D, 0xFF}), th); err != nil {
		t.Fatal(err)
	}
	if !th.Props.Unmoveable || th.Props.HasBones {
		t.Fatalf("props %+v", th.Props)
	}
}

// TestKingsValeTruncatedNpcSales checks that a truncated NPC sale list is an
// error instead of a silently empty list.
func TestKingsValeTruncatedNpcSales(t *testing.T) {
	th := &thing.Thing{ID: 100, Category: thing.CategoryItem}
	in := []byte{0x30, 0x02, 0x00, 0x01, 0x00, 0x00, 0x00}
	if err := table7.ReadThing(binio.NewReader(in), th); err == nil {
		t.Fatal("expected an error")
	}
}
