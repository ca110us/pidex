// Package configgen drafts team configs from plain-language descriptions,
// so a team can set up its own vocabulary without writing YAML. The model
// fills a simplified, strictly typed form; the result goes through the same
// validation as a hand-written config, and validation errors are sent back
// for a repair round.
package configgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"pidex/internal/ai"
	"pidex/internal/identity"
	"pidex/internal/teamconfig"
)

// MaxDescription bounds the user's text.
const MaxDescription = 4000

const repairRounds = 2

type Model interface {
	Structured(ctx context.Context, meta ai.CallMeta, req ai.StructuredRequest) (json.RawMessage, ai.Usage, error)
}

type Request struct {
	// Description of the work, or of the change when Base is set.
	Description string
	// Base is the config to change; nil drafts one from scratch.
	Base *teamconfig.Config
	// TeamID is only used to meter the call.
	TeamID uuid.UUID
	// IdentifyBy, when set, is how the user said subjects are told apart;
	// every subject type is drafted that way.
	IdentifyBy teamconfig.IdentifyBy
}

type Draft struct {
	Config   *teamconfig.Config
	Summary  string
	TestTips []string
}

// ErrInvalid is returned when the model could not produce a valid config.
var ErrInvalid = errors.New("the model could not produce a valid config")

func Generate(ctx context.Context, m Model, req Request) (*Draft, error) {
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		return nil, errors.New("description is required")
	}
	if len(desc) > MaxDescription {
		return nil, fmt.Errorf("description is longer than %d characters", MaxDescription)
	}

	var prompt strings.Builder
	if req.Base != nil {
		base, err := json.MarshalIndent(fromConfig(req.Base), "", "  ")
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&prompt, "Current config:\n%s\n\nRequested change:\n%s\n", base, desc)
		prompt.WriteString("\nKeep the language of the current config for its names and for anything you add. Write summary and test_tips in the language of the requested change.\n")
	} else {
		fmt.Fprintf(&prompt, "Description of the team's work:\n%s\n", desc)
		prompt.WriteString("\nWrite every human-readable text in the language of the description above.\n")
	}

	switch req.IdentifyBy {
	case "":
	case teamconfig.IdentifyByAppearance:
		prompt.WriteString("\nThe user chose how subjects are told apart: by how they look. They carry no label or number and none will be added. Set identify_by to appearance for every subject type.\n")
	case teamconfig.IdentifyByCode:
		prompt.WriteString("\nThe user chose how subjects are told apart: by a label, tag, barcode or number they carry. Set identify_by to code for every subject type.\n")
	default:
		return nil, fmt.Errorf("identify_by %q must be code or appearance", req.IdentifyBy)
	}

	var lastErr error
	for round := 0; round <= repairRounds; round++ {
		raw, _, err := m.Structured(ctx, ai.CallMeta{TeamID: req.TeamID, Purpose: "config_draft"}, ai.StructuredRequest{
			Instructions:    instructions,
			Prompt:          prompt.String(),
			Schema:          schema(),
			MaxOutputTokens: 16000,
		})
		if err != nil {
			return nil, err
		}
		draft, err := parse(raw)
		if err == nil && req.IdentifyBy != "" {
			for _, st := range draft.Config.SubjectTypes {
				if st.IdentifyBy != req.IdentifyBy {
					err = fmt.Errorf("subject type %q has identify_by %s, but the user chose %s for every subject type", st.Key, st.IdentifyBy, req.IdentifyBy)
					break
				}
			}
		}
		if err == nil {
			if req.Base != nil {
				keepUnformed(req.Base, draft.Config)
			}
			return draft, nil
		}
		lastErr = err
		fmt.Fprintf(&prompt, "\nYour previous answer:\n%s\n\nIt was rejected:\n%v\n\nReturn a corrected, complete answer.\n", raw, err)
	}
	return nil, fmt.Errorf("%w: %v", ErrInvalid, lastErr)
}

// checkExamples runs each field's example through the same barcode rule,
// normalization and pattern as a real photo would go through. A pattern
// written for the printed form ("IT-004512") rather than the normalized one
// ("IT004512") would otherwise silently drop every identity read.
func checkExamples(f formConfig, cfg *teamconfig.Config) error {
	var errs []error
	for _, ft := range f.SubjectTypes {
		st, _ := cfg.SubjectType(ft.Key)
		for _, ff := range ft.IdentityFields {
			ex := strings.TrimSpace(ff.Example)
			if ex == "" {
				continue
			}
			value := ex
			if b := st.Barcode; b != nil && b.Field == ff.Key {
				v, ok := b.Extract(ex)
				if !ok {
					errs = append(errs, fmt.Errorf("subject type %q: example %q of barcode field %q does not match the barcode pattern %q", ft.Key, ex, ff.Key, b.Pattern))
					continue
				}
				value = v
			}
			if _, norm := identity.Clean(st, map[string]string{ff.Key: value}, cfg.DateOrder); norm[ff.Key] == "" {
				fl, _ := st.Field(ff.Key)
				errs = append(errs, fmt.Errorf("subject type %q: example %q of field %q normalizes to %q, which the pattern %q rejects; patterns apply to the normalized value",
					ft.Key, ex, ff.Key, identity.Normalize(fl.Kind, value, cfg.DateOrder), fl.Pattern))
			}
		}
	}
	return errors.Join(errs...)
}

func parse(raw json.RawMessage) (*Draft, error) {
	var f form
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("answer is not the requested JSON: %w", err)
	}
	// Round-trip through the config format so defaults and validation are
	// exactly those of a hand-written config.
	data, err := json.Marshal(f.Config.toConfig())
	if err != nil {
		return nil, err
	}
	cfg, err := teamconfig.Parse(data)
	if err != nil {
		return nil, err
	}
	if err := checkExamples(f.Config, cfg); err != nil {
		return nil, err
	}
	var tips []string
	for _, t := range f.TestTips {
		if t = strings.TrimSpace(t); t != "" {
			tips = append(tips, t)
		}
	}
	return &Draft{Config: cfg, Summary: strings.TrimSpace(f.Summary), TestTips: tips}, nil
}

const instructions = `You design the configuration of a photo filing app for one team. Team members only take photos with their phone. A vision model reads each photo using your configuration, so everything you write must be visible in photos. Subjects are recognized in one of two ways, chosen per subject type with identify_by:
- code: the subject carries an identifier (wristband, serial number, door number, license plate, asset label). Before photographing a subject the team photographs that identifier (an "identity photo"), and every following photo is filed under that subject until the next identity photo.
- appearance: the subjects carry no identifier and nobody would label them: pets, mugs, plants in a home, personal belongings, artworks. Each photo is matched to the subject it shows by how it looks, compared with photos already filed; new ones are added by themselves, and unsure matches are confirmed with one tap. Never ask a team to stick numbers or labels on such things.

Write the configuration for the team described by the user, or apply the requested change to the current configuration.

Subject types (0 to 6) are the things photos are filed under: a patient, a house, a unit in a building, a vehicle, a plant, a sample. Use parent for real hierarchies (a unit belongs to a project) and "" otherwise. Use none (an empty list) when photos are kept by where and when they were taken rather than by who or what they show: a trip, a holiday, an event, a diary of places. Those photos are grouped by place and day by themselves, from the phone's location and clock; give them tags for what they show (scenery, food, people, buildings…).

Choose identify_by: code whenever subjects already carry an identifier, and always for people's records in healthcare, law or finance, where a mix-up causes harm; appearance when they carry none. Telling people apart by their face is not offered: for people, use code.

carriers, for code, says in plain words where a subject's identity is visible in photos: a wristband, a door number plate, a license plate, a label. For appearance, it says which visible features tell one subject from another of its kind: colours, markings, patterns, prints or logos, shape, wear, accessories.

Use one subject type for things identified by the same kind of carrier, even if they differ (computers and monitors with the same asset label are one "device" type; tell them apart with tags). A barcode or label cannot tell two such types apart.

Identity fields (1 to 5 per type) are what the vision model reads from a carrier. Appearance types read nothing: give them one text field "name" (what the owner calls it, filled in later by hand), display_fields ["name"], min_fields 0 and no barcode. Use 1 to 3 fields. kind is one of: person_name, date, id (codes and numbers that must match exactly), address, text. example is one realistic value exactly as printed on the carrier. Set pattern only when the format is certain, otherwise "". A pattern is an RE2 regex checked against the normalized value: id values keep only uppercase letters and digits, so printed "IT-004512" is checked as "IT004512" and its pattern is ^IT[0-9]{6}$. display_fields lists the field keys that name a subject in lists.

min_fields is how many identity fields must agree before a photo is filed under an existing subject. Use 1, except where mixing up two subjects causes harm (healthcare: 2 of name, date of birth and record number).

barcode: enable it only if carriers usually carry a barcode or QR code encoding one identity field, and name that field. Its pattern is checked against the raw code before normalization; capture group 1, if any, becomes the value. strong is true only when the code alone identifies the subject with certainty (hospital wristband).

strictness: strict when filing a photo under the wrong subject causes real harm (health, legal evidence, money); lenient when it is easily fixed and low stakes; standard otherwise. privacy: high when subjects are people or health information is involved, else standard. live_photos: true only when the user asks to keep Live Photos or the motion around each shot; otherwise false, and kept as it was when editing. date_order: dmy only when the description says the team works in a country that writes the day first; otherwise mdy.

name: a short name for what the collection holds in general ("Travels", "Pets", "Site photos", "Wound care"). Never name it after one place, date, person or animal the user mentions as an example, unless they say the whole collection is about only that.

Tags (10 to 40) are things a team member would search for later, each recognizable in a single photo: body site or part, condition or defect, stage of work, action, kind of photo (overview, close-up, document). Group related tags with group, a short heading in the language of the user's text (for example "Side", "Body site", "Stage"). Do not add tags that need a diagnosis or a measurement the photo cannot show. Tags are kinds of content, never one particular place, landmark, city, date or name: where and when a photo was taken is known from the phone and searched by itself.

session: gap_minutes is how long a pause ends the current subject (5 to 60); anchor_valid_minutes is how long an identity photo keeps filing photos (0 means until the gap); max_jump_meters is a distance between photos that ends the current subject (0 to disable).

language is the BCP 47 tag of the language you write in ("en", "zh-Hans", "es"); photo descriptions will be written in it too. Keys are lowercase English snake_case. Write names, groups, descriptions, carriers, summary and test_tips in the language the user's text is written in: English text gets English names, Chinese text Chinese names. Never switch to another language because of where the work might take place.

When changing a configuration, keep every existing key and everything the user did not ask to change exactly as it is. Removing or renaming a subject type, identity field or tag affects photos already filed, so do it only when asked.

summary: 2 to 4 sentences on what the configuration does, or what changed. test_tips: 2 to 4 short steps to try it with a phone today: for code, starting with which identity photo to take; for appearance, photograph each one from a couple of angles, then later ones are recognized, and confirm or name them in the app; with no subject types, just shoot or add photos from the library, and find them later by place, day or what they show.`
