package event

import "strconv"

// CaptionStatus is where a file event's image stands in being described
// (phase 19).
type CaptionStatus string

const (
	// CaptionNone: not an image, or images are off.
	CaptionNone CaptionStatus = ""
	// CaptionDescribed: the event's text holds the description.
	CaptionDescribed CaptionStatus = "described"
	// CaptionPending: waits for a later `ingest` (the per-run limit).
	CaptionPending CaptionStatus = "pending"
	// CaptionUnreadable: the file did not decode or was too large; tried
	// again only when the file changes.
	CaptionUnreadable CaptionStatus = "unreadable"
)

// Image is the image part of a file event's metadata. The description and
// the visible text are kept apart, each with the model and prompt that
// wrote it, so they can later become separate representations (#22);
// SHA256 identifies the image across moves and renames, and stands for it
// as an entity (#20, #23). Pixels are never stored.
type Image struct {
	SHA256        string
	Width         int
	Height        int
	Status        CaptionStatus
	Model         string
	PromptVersion int
	Description   string
	VisibleText   string
}

// Image metadata keys, as stored; ImageSHA256Key is indexed (schema
// version 10) to find a description by the image's content.
const (
	ImageSHA256Key          = "image_sha256"
	keyImageWidth           = "image_width"
	keyImageHeight          = "image_height"
	keyCaptionStatus        = "caption_status"
	keyCaptionModel         = "caption_model"
	keyCaptionPromptVersion = "caption_prompt_version"
	keyCaptionDescription   = "caption_description"
	keyCaptionVisibleText   = "caption_visible_text"
)

// Metadata is the stored form of i, merged into a file's metadata.
func (i Image) Metadata() Metadata {
	return Metadata{ImageSHA256Key: i.SHA256, keyImageWidth: strconv.Itoa(i.Width), keyImageHeight: strconv.Itoa(i.Height),
		keyCaptionStatus: string(i.Status), keyCaptionModel: i.Model, keyCaptionPromptVersion: strconv.Itoa(i.PromptVersion),
		keyCaptionDescription: i.Description, keyCaptionVisibleText: i.VisibleText}
}

// Image reads e's metadata as an image; Status is CaptionNone for a file
// that is not one.
func (e Event) Image() Image {
	m := e.Metadata
	width, _ := strconv.Atoi(m[keyImageWidth])
	height, _ := strconv.Atoi(m[keyImageHeight])
	promptVersion, _ := strconv.Atoi(m[keyCaptionPromptVersion])
	return Image{SHA256: m[ImageSHA256Key], Width: width, Height: height, Status: CaptionStatus(m[keyCaptionStatus]),
		Model: m[keyCaptionModel], PromptVersion: promptVersion, Description: m[keyCaptionDescription], VisibleText: m[keyCaptionVisibleText]}
}
