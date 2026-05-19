package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wowsims/tbc/sim/core"
	proto "github.com/wowsims/tbc/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
)

const defaultProfileAPIVersion = 13

type defaultProfileConfig struct {
	class           proto.Class
	race            proto.Race
	profession1     proto.Profession
	profession2     proto.Profession
	gearPath        string
	talents         string
	rotationPath    string
	rotation        *proto.APLRotation
	specOptions     any
	consumes        *proto.ConsumesSpec
	individualBuffs *proto.IndividualBuffs
	raidBuffs       *proto.RaidBuffs
	partyBuffs      *proto.PartyBuffs
	debuffs         *proto.Debuffs
	distance        float64
	reactionTimeMs  int32
	inFrontOfTarget bool
	healingModel    *proto.HealingModel
	tanks           []*proto.UnitReference
	targetDummies   int32
}

func handleDefaultProfileJSONAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	settings, err := defaultIndividualSimSettings(r.URL.Query().Get("class"), r.URL.Query().Get("spec"))
	if err != nil {
		writeJSONAPIError(w, http.StatusNotFound, err.Error())
		return
	}

	out, err := (protojson.MarshalOptions{}).Marshal(settings)
	if err != nil {
		writeJSONAPIError(w, http.StatusInternalServerError, "failed to marshal default profile: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(out)
}

func defaultIndividualSimSettings(className, specName string) (*proto.IndividualSimSettings, error) {
	className = normalizeDefaultProfileName(className)
	specName = normalizeDefaultProfileName(specName)

	cfg, err := defaultProfileConfigFor(className, specName)
	if err != nil {
		return nil, err
	}

	equipment, err := readDefaultEquipment(cfg.gearPath)
	if err != nil {
		return nil, err
	}
	var rotation *proto.APLRotation
	if cfg.rotation != nil {
		rotation = cloneDefault(cfg.rotation).(*proto.APLRotation)
	} else {
		rotation, err = readDefaultRotation(cfg.rotationPath)
		if err != nil {
			return nil, err
		}
	}

	reactionTime := cfg.reactionTimeMs
	if reactionTime == 0 {
		reactionTime = 100
	}
	individualBuffs := cfg.individualBuffs
	if individualBuffs == nil {
		individualBuffs = core.FullIndividualBuffs
	}
	raidBuffs := cfg.raidBuffs
	if raidBuffs == nil {
		raidBuffs = core.FullRaidBuffs
	}
	partyBuffs := cfg.partyBuffs
	if partyBuffs == nil {
		partyBuffs = core.FullPartyBuffs
	}
	debuffs := cfg.debuffs
	if debuffs == nil {
		debuffs = core.FullDebuffs
	}

	player := &proto.Player{
		Class:              cfg.class,
		Race:               cfg.race,
		Equipment:          equipment,
		Consumables:        cfg.consumes,
		Buffs:              cloneDefault(individualBuffs).(*proto.IndividualBuffs),
		TalentsString:      cfg.talents,
		Profession1:        defaultProfession(cfg.profession1, proto.Profession_Engineering),
		Profession2:        cfg.profession2,
		Rotation:           rotation,
		DistanceFromTarget: cfg.distance,
		ReactionTimeMs:     reactionTime,
		ChannelClipDelayMs: 50,
		InFrontOfTarget:    cfg.inFrontOfTarget,
	}
	if cfg.healingModel != nil {
		player.HealingModel = cloneDefault(cfg.healingModel).(*proto.HealingModel)
	}
	if err := setDefaultPlayerSpec(player, cfg.specOptions); err != nil {
		return nil, err
	}

	return &proto.IndividualSimSettings{
		ApiVersion:    defaultProfileAPIVersion,
		RaidBuffs:     cloneDefault(raidBuffs).(*proto.RaidBuffs),
		PartyBuffs:    cloneDefault(partyBuffs).(*proto.PartyBuffs),
		Debuffs:       cloneDefault(debuffs).(*proto.Debuffs),
		Encounter:     core.MakeSingleTargetEncounter(5),
		Player:        player,
		Tanks:         cfg.tanks,
		TargetDummies: cfg.targetDummies,
	}, nil
}

func defaultProfileConfigFor(className, specName string) (defaultProfileConfig, error) {
	switch className + "/" + specName {
	case "hunter/bm", "hunter/beastmastery", "hunter/beastmaster":
		return hunterDefaultProfile("bm", "522002005150122431051-0505201205"), nil
	case "hunter/survival", "hunter/sv":
		return hunterDefaultProfile("sv", "502-0550201205-333200022003223005103"), nil
	case "hunter/marksmanship", "hunter/marksman", "hunter/mm":
		return hunterDefaultProfile("bm", "522002005150122431051-0505201205"), nil
	case "druid/balance":
		return defaultProfileConfig{
			class:        proto.Class_ClassDruid,
			race:         proto.Race_RaceNightElf,
			gearPath:     "druid/balance/gear_sets/p1_a.gear.json",
			talents:      "510022312503135231351--520033",
			rotationPath: "druid/balance/apls/default.apl.json",
			specOptions: &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{
				Options: &proto.BalanceDruid_Options{ClassOptions: &proto.DruidOptions{}},
			}},
		}, nil
	case "druid/feral", "druid/feralcat":
		return defaultProfileConfig{
			class:       proto.Class_ClassDruid,
			race:        proto.Race_RaceNightElf,
			profession1: proto.Profession_Engineering,
			profession2: proto.Profession_Enchanting,
			gearPath:    "druid/feralcat/gear_sets/p1_realistic_6p.gear.json",
			talents:     "-503032132322105301251-05503301",
			rotation:    feralDefaultRotation(),
			specOptions: &proto.Player_FeralCatDruid{FeralCatDruid: &proto.FeralCatDruid{
				Rotation: &proto.FeralCatDruid_Rotation{
					FinishingMove:      proto.FeralCatDruid_Rotation_Rip,
					Biteweave:          true,
					RipMinComboPoints:  5,
					BiteMinComboPoints: 5,
					MangleTrick:        true,
					MaintainFaerieFire: true,
				},
				Options: &proto.FeralCatDruid_Options{},
			}},
			consumes: &proto.ConsumesSpec{
				PotId:            22838,
				BattleElixirId:   22831,
				GuardianElixirId: 32067,
				FoodId:           27664,
				MhImbueId:        34340,
				ConjuredId:       12662,
				SuperSapper:      true,
				GoblinSapper:     true,
				ScrollAgi:        true,
				ScrollStr:        true,
			},
			individualBuffs: feralDefaultIndividualBuffs(),
			raidBuffs:       feralDefaultRaidBuffs(),
			partyBuffs:      feralDefaultPartyBuffs(),
			debuffs:         feralDefaultDebuffs(),
		}, nil
	case "druid/bear", "druid/feralbear", "druid/tank":
		return feralBearDefaultProfile(), nil
	case "mage/arcane", "mage/fire", "mage/frost":
		return defaultProfileConfig{
			class:        proto.Class_ClassMage,
			race:         proto.Race_RaceTroll,
			profession1:  proto.Profession_Engineering,
			profession2:  proto.Profession_Tailoring,
			gearPath:     "mage/dps/gear_sets/p1Arcane.gear.json",
			talents:      "2500052300030150330125--053500031003001",
			rotationPath: "mage/dps/apls/arcane.apl.json",
			specOptions: &proto.Player_Mage{Mage: &proto.Mage{
				Options: &proto.Mage_Options{ClassOptions: &proto.MageOptions{
					DefaultMageArmor: proto.MageArmor_MageArmorMageArmor,
				}},
			}},
			consumes: &proto.ConsumesSpec{
				GuardianElixirId: 32067,
				BattleElixirId:   28103,
				FoodId:           27657,
				MhImbueId:        25122,
				PotId:            22839,
			},
			distance: 20,
		}, nil
	case "paladin/ret", "paladin/retribution":
		return defaultProfileConfig{
			class:        proto.Class_ClassPaladin,
			race:         proto.Race_RaceBloodElf,
			gearPath:     "paladin/retribution/gear_sets/p1.gear.json",
			talents:      "5-053201-0523005120033125331051",
			rotationPath: "paladin/retribution/apls/default.apl.json",
			specOptions: &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{
				Options: &proto.RetributionPaladin_Options{ClassOptions: &proto.PaladinOptions{}},
			}},
		}, nil
	case "paladin/prot", "paladin/protection":
		return protectionPaladinDefaultProfile(), nil
	case "priest/shadow":
		return defaultProfileConfig{
			class:        proto.Class_ClassPriest,
			race:         proto.Race_RaceTroll,
			gearPath:     "priest/dps/gear_sets/pre_raid.gear.json",
			talents:      "500230013--503250510240103051451",
			rotationPath: "priest/dps/apls/default.apl.json",
			specOptions: &proto.Player_Priest{Priest: &proto.Priest{
				Options: &proto.Priest_Options{ClassOptions: &proto.PriestOptions{
					PreShadowform: true,
				}},
			}},
		}, nil
	case "rogue/combat", "rogue/assassination", "rogue/subtlety":
		return defaultProfileConfig{
			class:        proto.Class_ClassRogue,
			race:         proto.Race_RaceHuman,
			gearPath:     "rogue/dps/gear_sets/preraid.gear.json",
			talents:      "00532012502-023305200005015002321151",
			rotationPath: "rogue/dps/apls/swords.apl.json",
			specOptions: &proto.Player_Rogue{Rogue: &proto.Rogue{
				Options: &proto.Rogue_Options{ClassOptions: &proto.RogueOptions{}},
			}},
			consumes: &proto.ConsumesSpec{
				FlaskId:    22854,
				FoodId:     33872,
				PotId:      22838,
				ConjuredId: 7676,
				OhImbueId:  27186,
			},
		}, nil
	case "shaman/elemental":
		return defaultProfileConfig{
			class:        proto.Class_ClassShaman,
			race:         proto.Race_RaceTroll,
			gearPath:     "shaman/elemental/gear_sets/p1_a.gear.json",
			talents:      "55003105100213351051--05105301005",
			rotationPath: "shaman/elemental/apls/default.apl.json",
			specOptions: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{
				Options: &proto.ElementalShaman_Options{ClassOptions: &proto.ShamanOptions{
					ShieldProcrate: 0,
				}},
			}},
		}, nil
	case "shaman/enhancement":
		return defaultProfileConfig{
			class:        proto.Class_ClassShaman,
			race:         proto.Race_RaceTroll,
			gearPath:     "shaman/enhancement/gear_sets/p1.gear.json",
			talents:      "03-500502210501133531151-50005301",
			rotationPath: "shaman/enhancement/apls/default.apl.json",
			specOptions: &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{
				Options: &proto.EnhancementShaman_Options{
					SyncType:    proto.ShamanSyncType_Auto,
					ImbueOh:     proto.ShamanImbue_WindfuryWeapon,
					ImbueOhSwap: proto.ShamanImbue_WindfuryWeapon,
					ClassOptions: &proto.ShamanOptions{
						ImbueMh:        proto.ShamanImbue_WindfuryWeapon,
						ImbueMhSwap:    proto.ShamanImbue_WindfuryWeapon,
						ShieldProcrate: 0,
					},
				},
			}},
		}, nil
	case "warlock/affliction":
		return warlockDefaultProfile("affliction", "05022221112351055003--50500051220001", false), nil
	case "warlock/demonology":
		return warlockDefaultProfile("demonology", "01-2050030133250101501351-5050005112", false), nil
	case "warlock/destruction":
		return warlockDefaultProfile("destruction", "-20500301332101-50500051220051053105", true), nil
	case "warrior/arms":
		return warriorDefaultProfile("arms", "p1_arms", "32005020352010500221-0550000500521203"), nil
	case "warrior/fury":
		return warriorDefaultProfile("fury", "p1_fury", "3500501130201-05050005505012050115"), nil
	case "warrior/prot", "warrior/protection":
		return protectionWarriorDefaultProfile(), nil
	default:
		return defaultProfileConfig{}, fmt.Errorf("no built-in default profile for %s/%s", className, specName)
	}
}

func hunterDefaultProfile(specName, talents string) defaultProfileConfig {
	switch specName {
	case "survival", "sv":
		specName = "sv"
	case "bm", "beastmastery", "beastmaster":
		specName = "bm"
	case "marksmanship", "marksman", "mm":
		specName = "bm"
	default:
		specName = "bm"
	}

	return defaultProfileConfig{
		class:        proto.Class_ClassHunter,
		race:         proto.Race_RaceOrc,
		profession1:  proto.Profession_Engineering,
		profession2:  proto.Profession_Blacksmithing,
		gearPath:     "hunter/dps/gear_sets/phase_1/" + specName + "/2h_6p.gear.json",
		talents:      talents,
		rotationPath: "hunter/dps/apls/default.apl.json",
		specOptions: &proto.Player_Hunter{Hunter: &proto.Hunter{
			Options: &proto.Hunter_Options{ClassOptions: &proto.HunterOptions{
				Ammo:             proto.HunterOptions_WardensArrow,
				QuiverBonus:      proto.HunterOptions_Speed15,
				PetType:          proto.HunterOptions_Ravager,
				PetUptime:        1,
				PetSingleAbility: false,
			}},
		}},
		consumes: &proto.ConsumesSpec{
			BattleElixirId:   22831,
			GuardianElixirId: 22840,
			FoodId:           27659,
			PotId:            22838,
			ConjuredId:       12662,
			ExplosiveId:      30217,
			PetFoodId:        33874,
			PetScrollAgi:     true,
			PetScrollStr:     true,
			SuperSapper:      true,
			GoblinSapper:     true,
			ScrollAgi:        true,
			ScrollStr:        true,
		},
		individualBuffs: hunterDefaultIndividualBuffs(),
		raidBuffs:       hunterDefaultRaidBuffs(),
		partyBuffs:      hunterDefaultPartyBuffs(),
		debuffs:         hunterDefaultDebuffs(),
		distance:        7,
	}
}

func hunterDefaultIndividualBuffs() *proto.IndividualBuffs {
	return &proto.IndividualBuffs{
		BlessingOfKings:  true,
		BlessingOfMight:  proto.TristateEffect_TristateEffectImproved,
		BlessingOfWisdom: proto.TristateEffect_TristateEffectImproved,
		UnleashedRage:    true,
	}
}

func hunterDefaultRaidBuffs() *proto.RaidBuffs {
	return &proto.RaidBuffs{
		Bloodlust:          true,
		ArcaneBrilliance:   true,
		DivineSpirit:       proto.TristateEffect_TristateEffectImproved,
		GiftOfTheWild:      proto.TristateEffect_TristateEffectImproved,
		PowerWordFortitude: proto.TristateEffect_TristateEffectImproved,
		ShadowProtection:   true,
	}
}

func hunterDefaultPartyBuffs() *proto.PartyBuffs {
	return &proto.PartyBuffs{
		BattleShout:          proto.TristateEffect_TristateEffectImproved,
		BraidedEterniumChain: true,
		FerociousInspiration: 1,
		GraceOfAirTotem:      proto.TristateEffect_TristateEffectImproved,
		LeaderOfThePack:      proto.TristateEffect_TristateEffectImproved,
		StrengthOfEarthTotem: proto.TristateEffect_TristateEffectImproved,
		TotemTwisting:        true,
		WindfuryTotem:        proto.TristateEffect_TristateEffectImproved,
		Drums:                proto.Drums_LesserDrumsOfBattle,
	}
}

func hunterDefaultDebuffs() *proto.Debuffs {
	return &proto.Debuffs{
		BloodFrenzy:                 true,
		CurseOfRecklessness:         true,
		ExposeArmor:                 proto.TristateEffect_TristateEffectImproved,
		ExposeWeaknessUptime:        0.9,
		ExposeWeaknessHunterAgility: 1080,
		FaerieFire:                  proto.TristateEffect_TristateEffectImproved,
		GiftOfArthas:                true,
		HuntersMark:                 proto.TristateEffect_TristateEffectImproved,
		ImprovedSealOfTheCrusader:   proto.TristateEffect_TristateEffectImproved,
		InsectSwarm:                 true,
		JudgementOfLight:            true,
		JudgementOfWisdom:           true,
		Mangle:                      true,
		Misery:                      true,
		SunderArmor:                 true,
	}
}

func feralDefaultRotation() *proto.APLRotation {
	return &proto.APLRotation{
		PriorityList: []*proto.APLListItem{
			{
				Action: &proto.APLAction{
					Action: &proto.APLAction_CatOptimalRotationAction{
						CatOptimalRotationAction: &proto.APLActionCatOptimalRotationAction{
							FinishingMove:      proto.FeralCatDruid_Rotation_Rip,
							Biteweave:          true,
							RipMinComboPoints:  5,
							BiteMinComboPoints: 5,
							MangleTrick:        true,
							MaintainFaerieFire: true,
						},
					},
				},
			},
		},
	}
}

func feralDefaultIndividualBuffs() *proto.IndividualBuffs {
	return &proto.IndividualBuffs{
		BlessingOfKings: true,
		BlessingOfMight: proto.TristateEffect_TristateEffectImproved,
		UnleashedRage:   true,
	}
}

func feralDefaultRaidBuffs() *proto.RaidBuffs {
	return &proto.RaidBuffs{
		Bloodlust:          true,
		ArcaneBrilliance:   true,
		DivineSpirit:       proto.TristateEffect_TristateEffectImproved,
		GiftOfTheWild:      proto.TristateEffect_TristateEffectImproved,
		PowerWordFortitude: proto.TristateEffect_TristateEffectImproved,
		ShadowProtection:   true,
	}
}

func feralDefaultPartyBuffs() *proto.PartyBuffs {
	return &proto.PartyBuffs{
		Drums:                proto.Drums_LesserDrumsOfBattle,
		FerociousInspiration: 2,
		BattleShout:          proto.TristateEffect_TristateEffectImproved,
		GraceOfAirTotem:      proto.TristateEffect_TristateEffectImproved,
		WindfuryTotem:        proto.TristateEffect_TristateEffectImproved,
		ManaSpringTotem:      proto.TristateEffect_TristateEffectRegular,
		StrengthOfEarthTotem: proto.TristateEffect_TristateEffectImproved,
		TotemTwisting:        true,
	}
}

func feralDefaultDebuffs() *proto.Debuffs {
	return &proto.Debuffs{
		ExposeWeaknessUptime:        0.9,
		ExposeWeaknessHunterAgility: 1080,
		BloodFrenzy:                 true,
		ExposeArmor:                 proto.TristateEffect_TristateEffectImproved,
		HuntersMark:                 proto.TristateEffect_TristateEffectImproved,
		ImprovedSealOfTheCrusader:   proto.TristateEffect_TristateEffectImproved,
		JudgementOfWisdom:           true,
		Misery:                      true,
		CurseOfRecklessness:         true,
		FaerieFire:                  proto.TristateEffect_TristateEffectImproved,
		SunderArmor:                 true,
		GiftOfArthas:                true,
	}
}

func feralBearDefaultProfile() defaultProfileConfig {
	return defaultProfileConfig{
		class:        proto.Class_ClassDruid,
		race:         proto.Race_RaceNightElf,
		profession1:  proto.Profession_Engineering,
		profession2:  proto.Profession_Enchanting,
		gearPath:     "druid/feralbear/gear_sets/preraid.gear.json",
		talents:      "-503032132322105301251-05503301",
		rotationPath: "druid/feralbear/apls/default.apl.json",
		specOptions: &proto.Player_FeralBearDruid{FeralBearDruid: &proto.FeralBearDruid{
			Options: &proto.FeralBearDruid_Options{StartingRage: 0},
		}},
		consumes: &proto.ConsumesSpec{
			BattleElixirId:   22831,
			GuardianElixirId: 9088,
			FoodId:           27667,
			PotId:            22849,
			ConjuredId:       22105,
			MhImbueId:        34340,
			GoblinSapper:     true,
			SuperSapper:      true,
			ScrollAgi:        true,
			ScrollStr:        true,
			ScrollArm:        true,
			NightmareSeed:    true,
		},
		individualBuffs: feralBearDefaultIndividualBuffs(),
		raidBuffs:       feralBearDefaultRaidBuffs(),
		partyBuffs:      feralBearDefaultPartyBuffs(),
		debuffs:         feralBearDefaultDebuffs(),
		reactionTimeMs:  250,
		inFrontOfTarget: true,
		healingModel:    tankHealingModel(),
		tanks:           singlePlayerTankReference(),
	}
}

func feralBearDefaultIndividualBuffs() *proto.IndividualBuffs {
	return &proto.IndividualBuffs{
		BlessingOfKings:     true,
		BlessingOfMight:     proto.TristateEffect_TristateEffectImproved,
		BlessingOfSanctuary: true,
		UnleashedRage:       true,
	}
}

func feralBearDefaultRaidBuffs() *proto.RaidBuffs {
	return &proto.RaidBuffs{
		ArcaneBrilliance:   true,
		GiftOfTheWild:      proto.TristateEffect_TristateEffectImproved,
		PowerWordFortitude: proto.TristateEffect_TristateEffectImproved,
		Bloodlust:          true,
		ShadowProtection:   true,
		Thorns:             proto.TristateEffect_TristateEffectRegular,
		DivineSpirit:       proto.TristateEffect_TristateEffectImproved,
	}
}

func feralBearDefaultPartyBuffs() *proto.PartyBuffs {
	return &proto.PartyBuffs{
		Drums:                proto.Drums_LesserDrumsOfBattle,
		FerociousInspiration: 2,
		BattleShout:          proto.TristateEffect_TristateEffectImproved,
		GraceOfAirTotem:      proto.TristateEffect_TristateEffectImproved,
		WindfuryTotem:        proto.TristateEffect_TristateEffectImproved,
		ManaSpringTotem:      proto.TristateEffect_TristateEffectRegular,
		StrengthOfEarthTotem: proto.TristateEffect_TristateEffectImproved,
		TotemTwisting:        true,
	}
}

func feralBearDefaultDebuffs() *proto.Debuffs {
	return &proto.Debuffs{
		ExposeWeaknessUptime:        0.9,
		ExposeWeaknessHunterAgility: 1080,
		BloodFrenzy:                 true,
		ExposeArmor:                 proto.TristateEffect_TristateEffectImproved,
		FaerieFire:                  proto.TristateEffect_TristateEffectImproved,
		HuntersMark:                 proto.TristateEffect_TristateEffectImproved,
		ImprovedSealOfTheCrusader:   proto.TristateEffect_TristateEffectImproved,
		CurseOfRecklessness:         true,
		InsectSwarm:                 true,
		JudgementOfWisdom:           true,
		Misery:                      true,
		Screech:                     true,
		ShadowEmbrace:               true,
		SunderArmor:                 true,
	}
}

func warriorDefaultProfile(specName, gearName, talents string) defaultProfileConfig {
	return defaultProfileConfig{
		class:        proto.Class_ClassWarrior,
		race:         proto.Race_RaceOrc,
		profession1:  proto.Profession_Engineering,
		profession2:  proto.Profession_Blacksmithing,
		gearPath:     "warrior/dps/gear_sets/" + gearName + ".gear.json",
		talents:      talents,
		rotationPath: "warrior/dps/apls/" + specName + ".apl.json",
		specOptions: &proto.Player_DpsWarrior{DpsWarrior: &proto.DpsWarrior{
			Options: &proto.DpsWarrior_Options{ClassOptions: &proto.WarriorOptions{
				DefaultShout:  proto.WarriorShout_WarriorShoutBattle,
				DefaultStance: proto.WarriorStance_WarriorStanceBerserker,
			}},
		}},
		consumes: &proto.ConsumesSpec{
			PotId:       22838,
			FlaskId:     22854,
			FoodId:      27658,
			ConjuredId:  22788,
			ExplosiveId: 30217,
			SuperSapper: true,
			OhImbueId:   29453,
			ScrollAgi:   true,
			ScrollStr:   true,
		},
		distance: 25,
	}
}

func protectionWarriorDefaultProfile() defaultProfileConfig {
	return defaultProfileConfig{
		class:        proto.Class_ClassWarrior,
		race:         proto.Race_RaceOrc,
		profession1:  proto.Profession_Engineering,
		profession2:  proto.Profession_Blacksmithing,
		gearPath:     "warrior/protection/gear_sets/p1_bis.gear.json",
		talents:      "35000301302-03-0055511033001101501351",
		rotationPath: "warrior/protection/apls/default.apl.json",
		specOptions: &proto.Player_ProtectionWarrior{ProtectionWarrior: &proto.ProtectionWarrior{
			Options: &proto.ProtectionWarrior_Options{ClassOptions: &proto.WarriorOptions{
				QueueDelay:     250,
				StartingRage:   100,
				DefaultShout:   proto.WarriorShout_WarriorShoutCommanding,
				DefaultStance:  proto.WarriorStance_WarriorStanceDefensive,
				HasBsT2:        true,
				StanceSnapshot: true,
			}},
		}},
		consumes: &proto.ConsumesSpec{
			PotId:            22849,
			FoodId:           27667,
			ConjuredId:       22105,
			ExplosiveId:      30217,
			SuperSapper:      true,
			GoblinSapper:     true,
			OhImbueId:        29453,
			ScrollAgi:        true,
			ScrollStr:        true,
			ScrollArm:        true,
			BattleElixirId:   22831,
			GuardianElixirId: 9088,
			NightmareSeed:    true,
		},
		individualBuffs: protectionWarriorDefaultIndividualBuffs(),
		raidBuffs:       protectionWarriorDefaultRaidBuffs(),
		partyBuffs:      protectionWarriorDefaultPartyBuffs(),
		debuffs:         protectionWarriorDefaultDebuffs(),
		inFrontOfTarget: true,
		healingModel:    tankHealingModel(),
		tanks:           singlePlayerTankReference(),
	}
}

func protectionWarriorDefaultIndividualBuffs() *proto.IndividualBuffs {
	return &proto.IndividualBuffs{
		BlessingOfKings:     true,
		BlessingOfMight:     proto.TristateEffect_TristateEffectImproved,
		BlessingOfSanctuary: true,
		UnleashedRage:       true,
	}
}

func protectionWarriorDefaultRaidBuffs() *proto.RaidBuffs {
	return &proto.RaidBuffs{
		Bloodlust:          true,
		PowerWordFortitude: proto.TristateEffect_TristateEffectImproved,
		GiftOfTheWild:      proto.TristateEffect_TristateEffectImproved,
		Thorns:             proto.TristateEffect_TristateEffectRegular,
		ShadowProtection:   true,
	}
}

func protectionWarriorDefaultPartyBuffs() *proto.PartyBuffs {
	return &proto.PartyBuffs{
		SanctityAura:         proto.TristateEffect_TristateEffectImproved,
		BraidedEterniumChain: true,
		GraceOfAirTotem:      proto.TristateEffect_TristateEffectImproved,
		StrengthOfEarthTotem: proto.TristateEffect_TristateEffectImproved,
		WindfuryTotem:        proto.TristateEffect_TristateEffectImproved,
		TotemTwisting:        true,
		BattleShout:          proto.TristateEffect_TristateEffectImproved,
	}
}

func protectionWarriorDefaultDebuffs() *proto.Debuffs {
	return &proto.Debuffs{
		ExposeWeaknessUptime:        0.9,
		ExposeWeaknessHunterAgility: 1080,
		ImprovedSealOfTheCrusader:   proto.TristateEffect_TristateEffectImproved,
		Misery:                      true,
		BloodFrenzy:                 true,
		Mangle:                      true,
		ExposeArmor:                 proto.TristateEffect_TristateEffectImproved,
		FaerieFire:                  proto.TristateEffect_TristateEffectImproved,
		SunderArmor:                 true,
		CurseOfRecklessness:         true,
		HuntersMark:                 proto.TristateEffect_TristateEffectImproved,
		InsectSwarm:                 true,
		ShadowEmbrace:               true,
		Screech:                     true,
	}
}

func protectionPaladinDefaultProfile() defaultProfileConfig {
	return defaultProfileConfig{
		class:        proto.Class_ClassPaladin,
		race:         proto.Race_RaceBloodElf,
		profession1:  proto.Profession_Blacksmithing,
		profession2:  proto.Profession_Engineering,
		gearPath:     "paladin/protection/gear_sets/p1-balanced.gear.json",
		rotationPath: "paladin/protection/apls/default.apl.json",
		specOptions: &proto.Player_ProtectionPaladin{ProtectionPaladin: &proto.ProtectionPaladin{
			Options: &proto.ProtectionPaladin_Options{ClassOptions: &proto.PaladinOptions{}},
		}},
		consumes: &proto.ConsumesSpec{},
		raidBuffs: &proto.RaidBuffs{
			Bloodlust: true,
		},
		partyBuffs:      &proto.PartyBuffs{},
		individualBuffs: &proto.IndividualBuffs{},
		debuffs:         &proto.Debuffs{},
		distance:        5,
		inFrontOfTarget: true,
		healingModel:    tankHealingModel(),
		tanks:           singlePlayerTankReference(),
	}
}

func tankHealingModel() *proto.HealingModel {
	return &proto.HealingModel{
		Hps:               2200,
		CadenceSeconds:    0.4,
		CadenceVariation:  1.2,
		AbsorbFrac:        0.02,
		BurstWindow:       6,
		InspirationUptime: 0.25,
	}
}

func singlePlayerTankReference() []*proto.UnitReference {
	return []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
}

func warlockDefaultProfile(rotationName, talents string, sacrificeSummon bool) defaultProfileConfig {
	return defaultProfileConfig{
		class:        proto.Class_ClassWarlock,
		race:         proto.Race_RaceOrc,
		profession1:  proto.Profession_Engineering,
		profession2:  proto.Profession_Tailoring,
		gearPath:     "warlock/dps/gear_sets/preraid.gear.json",
		talents:      talents,
		rotationPath: "warlock/dps/apls/" + rotationName + ".apl.json",
		specOptions: &proto.Player_Warlock{Warlock: &proto.Warlock{
			Options: &proto.Warlock_Options{ClassOptions: &proto.WarlockOptions{
				Summon:          proto.WarlockOptions_Succubus,
				SacrificeSummon: sacrificeSummon,
				Armor:           proto.WarlockOptions_FelArmor,
				CurseOptions:    proto.WarlockOptions_Recklessness,
			}},
		}},
		consumes: &proto.ConsumesSpec{
			FlaskId:      22866,
			FoodId:       27657,
			ConjuredId:   12662,
			MhImbueId:    25122,
			PotId:        22839,
			ExplosiveId:  30217,
			PetScrollAgi: true,
			PetScrollStr: true,
		},
		distance: 20,
	}
}

func readDefaultEquipment(relPath string) (*proto.EquipmentSpec, error) {
	equipment := &proto.EquipmentSpec{}
	if err := readDefaultProto(relPath, equipment); err != nil {
		return nil, err
	}
	return equipment, nil
}

func readDefaultRotation(relPath string) (*proto.APLRotation, error) {
	if relPath == "" {
		return &proto.APLRotation{Type: proto.APLRotation_TypeSimple}, nil
	}
	rotation := &proto.APLRotation{}
	if err := readDefaultProto(relPath, rotation); err != nil {
		return nil, err
	}
	return rotation, nil
}

func readDefaultProto(relPath string, msg googleProto.Message) error {
	data, err := readDefaultProfileFile(relPath)
	if err != nil {
		return err
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, msg); err != nil {
		return fmt.Errorf("failed to parse default profile file %s: %w", relPath, err)
	}
	return nil
}

func readDefaultProfileFile(relPath string) ([]byte, error) {
	relPath = filepath.FromSlash(relPath)
	candidates := []string{
		filepath.Join("ui", relPath),
		filepath.Join("..", "..", "ui", relPath),
		filepath.Join("/app", "ui", relPath),
	}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return data, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("default profile file not found: %s", filepath.ToSlash(relPath))
}

func normalizeDefaultProfileName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func defaultProfession(value proto.Profession, fallback proto.Profession) proto.Profession {
	if value != proto.Profession_ProfessionUnknown {
		return value
	}
	return fallback
}

func setDefaultPlayerSpec(player *proto.Player, spec any) error {
	switch typed := spec.(type) {
	case *proto.Player_BalanceDruid:
		player.Spec = typed
	case *proto.Player_FeralCatDruid:
		player.Spec = typed
	case *proto.Player_FeralBearDruid:
		player.Spec = typed
	case *proto.Player_Hunter:
		player.Spec = typed
	case *proto.Player_Mage:
		player.Spec = typed
	case *proto.Player_RetributionPaladin:
		player.Spec = typed
	case *proto.Player_ProtectionPaladin:
		player.Spec = typed
	case *proto.Player_Priest:
		player.Spec = typed
	case *proto.Player_Rogue:
		player.Spec = typed
	case *proto.Player_ElementalShaman:
		player.Spec = typed
	case *proto.Player_EnhancementShaman:
		player.Spec = typed
	case *proto.Player_Warlock:
		player.Spec = typed
	case *proto.Player_DpsWarrior:
		player.Spec = typed
	case *proto.Player_ProtectionWarrior:
		player.Spec = typed
	default:
		return fmt.Errorf("unsupported default player spec %T", spec)
	}
	return nil
}

func cloneDefault(msg googleProto.Message) googleProto.Message {
	return googleProto.Clone(msg)
}
