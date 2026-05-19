package main

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net/http"
	"sync"
	"testing"
	"time"

	_ "github.com/wowsims/tbc/sim/common"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

var basicSpec = &proto.Player_ElementalShaman{
	ElementalShaman: &proto.ElementalShaman{
		Options: &proto.ElementalShaman_Options{
			ClassOptions: &proto.ShamanOptions{},
		},
	},
}

var p1Equip = &proto.EquipmentSpec{
	Items: []*proto.ItemSpec{
		{Id: 40516, Enchant: 3820, Gems: []int32{41285, 40027}},
		{Id: 44661, Gems: []int32{39998}},
		{Id: 40286, Enchant: 3810},
		{Id: 44005, Enchant: 3722, Gems: []int32{40027}},
		{Id: 40514, Enchant: 3832, Gems: []int32{42144, 42144}},
		{Id: 40324, Enchant: 2332, Gems: []int32{42144, 0}},
		{Id: 40302, Enchant: 3246, Gems: []int32{0}},
		{Id: 40301, Gems: []int32{40014}},
		{Id: 40560, Enchant: 3721},
		{Id: 40519, Enchant: 3826},
		{Id: 37694},
		{Id: 40399},
		{Id: 40432},
		{Id: 40255},
		{Id: 40395, Enchant: 3834},
		{Id: 40401, Enchant: 1128},
		{Id: 40267},
	},
}

func init() {
	s := &server{
		progMut:         sync.RWMutex{},
		asyncProgresses: map[string]*asyncProgress{},
	}
	go func() {
		s.runServer(true, "localhost:3339", false, "", false, bufio.NewReader(bytes.NewBuffer([]byte{})))
	}()

	time.Sleep(time.Second) // hack so we have time for server to startup. Probably could repeatedly curl the endpoint until it responds.
}

// TestIndividualSim is just a smoke test to make sure the http server works as expected.
//
//	Don't modify this test unless the proto defintions change and this no longer compiles.
func TestIndividualSim(t *testing.T) {
	req := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:      proto.Race_RaceTroll,
				Class:     proto.Class_ClassShaman,
				Equipment: p1Equip,
				Spec:      basicSpec,
			},
			&proto.PartyBuffs{},
			&proto.RaidBuffs{},
			&proto.Debuffs{}),
		Encounter: &proto.Encounter{
			Duration: 120,
			Targets: []*proto.Target{
				{},
			},
		},
		SimOptions: &proto.SimOptions{
			Iterations: 5000,
			RandomSeed: 1,
			Debug:      false,
		},
	}

	msgBytes, err := googleProto.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to encode request: %s", err.Error())
	}

	r, err := http.Post("http://localhost:3339/raidSim", "application/x-protobuf", bytes.NewReader(msgBytes))
	if err != nil {
		t.Fatalf("Failed to POST request: %s", err.Error())
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed to read result body: %s", err.Error())
		return
	}

	rsr := &proto.RaidSimResult{}
	if err := googleProto.Unmarshal(body, rsr); err != nil {
		t.Fatalf("Failed to parse request: %s", err.Error())
		return
	}

	log.Printf("RESULT: %#v", rsr)
}

func TestRaidSimRequestFromIndividualSettings(t *testing.T) {
	settings := &proto.IndividualSimSettings{
		Settings: &proto.SimSettings{
			Iterations:   123,
			FixedRngSeed: 456,
		},
		RaidBuffs:  &proto.RaidBuffs{Bloodlust: true},
		PartyBuffs: &proto.PartyBuffs{TrueshotAura: true},
		Debuffs:    &proto.Debuffs{JudgementOfWisdom: true},
		Player: &proto.Player{
			Name:      "Example",
			Race:      proto.Race_RaceTroll,
			Class:     proto.Class_ClassShaman,
			Equipment: p1Equip,
			Spec:      basicSpec,
		},
		Encounter: &proto.Encounter{
			Duration: 60,
			Targets:  []*proto.Target{{Level: 73}},
		},
	}

	req, err := raidSimRequestFromIndividualSettings(settings, 321, 654)
	if err != nil {
		t.Fatalf("raidSimRequestFromIndividualSettings failed: %v", err)
	}
	if req.Type != proto.SimType_SimTypeIndividual {
		t.Fatalf("expected individual sim type, got %s", req.Type)
	}
	if req.SimOptions.Iterations != 321 {
		t.Fatalf("expected iterations override, got %d", req.SimOptions.Iterations)
	}
	if req.SimOptions.RandomSeed != 654 {
		t.Fatalf("expected random seed override, got %d", req.SimOptions.RandomSeed)
	}
	if req.Raid.Buffs != settings.RaidBuffs || req.Raid.Parties[0].Buffs != settings.PartyBuffs || req.Raid.Debuffs != settings.Debuffs {
		t.Fatalf("expected buffs/debuffs from individual settings")
	}
	if req.Raid.Parties[0].Players[0] != settings.Player {
		t.Fatalf("expected player from individual settings")
	}
}
