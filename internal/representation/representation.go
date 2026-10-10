// Package representation names what a model derived from an entity's
// content (#22): a description, the text in an image, a vector, later the
// objects or faces in it. Each one records the model and revision that
// wrote it, so a change of model or prompt finds what to write again, and
// replacing one never touches its entity.
package representation

// Modality is what a representation is made of.
type Modality string

const (
	ModalityText   Modality = "text"
	ModalityVector Modality = "vector"
	ModalityObject Modality = "object"
	ModalityFace   Modality = "face"
	ModalityAudio  Modality = "audio"
)

// Kind is what a representation says about its entity; an entity has at
// most one of each kind per modality.
type Kind string

const (
	// KindDescription is what an image shows, in words.
	KindDescription Kind = "description"
	// KindVisibleText is the text written in an image.
	KindVisibleText Kind = "visible_text"
	KindEmbedding   Kind = "embedding"
	KindDetection   Kind = "detection"
	KindTranscript  Kind = "transcript"
)

// Representation is one derived view of an entity. Revision is what the
// model was run with (the caption prompt version), 0 when nothing varies.
type Representation struct {
	Modality Modality
	Kind     Kind
	Model    string
	Revision int
	Content  string
}
