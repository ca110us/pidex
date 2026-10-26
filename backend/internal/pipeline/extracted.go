package pipeline

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/analysis"
	"pidex/internal/identity"
)

// Source says which tier produced a photo's analysis.
const (
	SourceBarcode = "barcode" // tier 0: on-device barcode, no model call
	SourceReused  = "reused"  // near-duplicate of an analyzed photo, no call
	SourceModel   = "model"   // tier 1 (and tier 2 when escalated)
	SourceNone    = "none"    // no model configured; filing still works
)

// Extracted is stored in photos.extracted. It records what was read and
// why the photo was filed where it was.
type Extracted struct {
	Source         string               `json:"source"`
	Model          string               `json:"model,omitempty"`
	ReusedFrom     *uuid.UUID           `json:"reused_from,omitempty"`
	RoleConfidence float64              `json:"role_confidence,omitempty"`
	SubjectType    string               `json:"subject_type,omitempty"`
	IdentityRead   *IdentityRead        `json:"identity_read,omitempty"`
	Resolution     *identity.Resolution `json:"resolution,omitempty"`
	Anchor         *Anchor              `json:"anchor,omitempty"`
	CreatedSubject bool                 `json:"created_subject,omitempty"`
	// Appearance is set for a subject told apart by how it looks: what was
	// seen and how it was matched.
	Appearance *AppearanceMatch `json:"appearance,omitempty"`
	// NamedDescription is the description with the photo's subject as
	// analysis.SubjectPlaceholder: shown with its name once it is filed.
	NamedDescription string                   `json:"named_description,omitempty"`
	DocumentFields   []analysis.DocumentField `json:"document_fields,omitempty"`
	// Tags keeps every tag the model reported, including those below the
	// keep threshold, so thresholds can be tuned without new calls.
	Tags []analysis.TagScore `json:"tags,omitempty"`
}

type IdentityRead struct {
	Fields         map[string]string `json:"fields"`
	Confidence     float64           `json:"confidence"`
	BarcodeField   string            `json:"barcode_field,omitempty"`
	ConfirmedByOCR bool              `json:"confirmed_by_ocr,omitempty"`
	Escalated      bool              `json:"escalated,omitempty"`
}

// Anchor is set when this photo identified a subject, and files its
// neighbours in the session.
type Anchor struct {
	SubjectID  uuid.UUID `json:"subject_id"`
	Confidence float64   `json:"confidence"`
}

func DecodeExtracted(raw []byte) Extracted {
	var ex Extracted
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &ex)
	}
	return ex
}

// DisplayFields flattens identity and document fields for clients.
func (ex Extracted) DisplayFields() map[string]string {
	out := map[string]string{}
	if ex.IdentityRead != nil {
		for k, v := range ex.IdentityRead.Fields {
			out[k] = v
		}
	}
	for _, d := range ex.DocumentFields {
		out[d.Name] = d.Value
	}
	return out
}

func DecodeOnDevice(raw []byte) *pidexv1.OnDeviceAnalysis {
	od := &pidexv1.OnDeviceAnalysis{}
	if len(raw) > 0 {
		_ = protojson.Unmarshal(raw, od)
	}
	return od
}

func ocrText(od *pidexv1.OnDeviceAnalysis) string {
	lines := make([]string, 0, len(od.GetText()))
	for _, t := range od.GetText() {
		lines = append(lines, t.GetText())
	}
	return strings.Join(lines, "\n")
}

// AppearanceMatch records how a photo's subject was matched by appearance.
type AppearanceMatch struct {
	Label    string `json:"label"`
	Features string `json:"features"`
	// Compared are the subjects shown to the model, closest first.
	Compared []uuid.UUID `json:"compared,omitempty"`
	// Verdict is the subject the model picked, or nil for a new one.
	Verdict    *uuid.UUID `json:"verdict,omitempty"`
	Confidence float64    `json:"confidence"`
	// ModelConfidence is the model's own, before the rules held it back.
	ModelConfidence float64 `json:"model_confidence,omitempty"`
	Reason          string  `json:"reason,omitempty"`
	// Unseen: the photo did not show the subject clearly enough to tell.
	Unseen bool `json:"unseen,omitempty"`
	// ByLook: filed by the look service alone, without the model; Look is
	// how similar the closest subject's photos look.
	ByLook bool    `json:"by_look,omitempty"`
	Look   float64 `json:"look,omitempty"`
}
