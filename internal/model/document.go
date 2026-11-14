package model

// Document is a long-form reference text that defines part of the product,
// such as a protocol, a data schema or an interface contract (FR-044). It is
// part of the product's definition, not a backlog item.
type Document struct {
	ID        string
	ProductID string
	// SequenceNumber numbers the product's documents (FR-050); with the
	// product's document prefix it forms the identifier, e.g. DOC-3.
	SequenceNumber int
	// Identifier is the human-readable identifier, filled in when read from
	// the store with the product's current prefix.
	Identifier string
	Title      string
	// Body is stored byte for byte (FR-046). For the markdown format it is
	// rendered as Markdown; every other format is shown and exported verbatim.
	Body   string
	Format string // one of the DocumentFormat* constants
	State  string // one of the DocumentState* constants
	// SupersededByID names the document that replaces this one; only set
	// while State is superseded (FR-049).
	SupersededByID string
	ComponentID    string // empty unless filed under a component
	CapabilityID   string // empty unless filed under a capability
	MilestoneID    string // empty if not linked
	// Revision is the number of the latest revision (FR-051), starting at 1.
	Revision  int
	CreatedAt string // RFC 3339
	UpdatedAt string // RFC 3339
}

// DocumentRevision is a snapshot of a document's content (FR-051).
type DocumentRevision struct {
	ID         string
	DocumentID string
	Revision   int
	Title      string
	Body       string
	Format     string
	UserID     string
	CreatedAt  string // RFC 3339
}

// Document formats (FR-046).
const (
	DocumentFormatMarkdown = "markdown"
	DocumentFormatText     = "text"
	DocumentFormatJSON     = "json"
	DocumentFormatYAML     = "yaml"
	DocumentFormatXML      = "xml"
	DocumentFormatProto    = "proto"
	DocumentFormatSQL      = "sql"
	DocumentFormatGraphQL  = "graphql"
)

// documentFormatExt maps each format to the file extension it is exported
// with (FR-907).
var documentFormatExt = map[string]string{
	DocumentFormatMarkdown: ".md",
	DocumentFormatText:     ".txt",
	DocumentFormatJSON:     ".json",
	DocumentFormatYAML:     ".yaml",
	DocumentFormatXML:      ".xml",
	DocumentFormatProto:    ".proto",
	DocumentFormatSQL:      ".sql",
	DocumentFormatGraphQL:  ".graphql",
}

// DocumentFormats lists the formats in display order.
var DocumentFormats = []string{
	DocumentFormatMarkdown, DocumentFormatText, DocumentFormatJSON, DocumentFormatYAML,
	DocumentFormatXML, DocumentFormatProto, DocumentFormatSQL, DocumentFormatGraphQL,
}

// ValidDocumentFormat reports whether format is a known document format.
func ValidDocumentFormat(format string) bool {
	_, ok := documentFormatExt[format]
	return ok
}

// DocumentFileExtension returns the export file extension of format, with
// the leading dot; unknown formats export as plain text.
func DocumentFileExtension(format string) string {
	if ext, ok := documentFormatExt[format]; ok {
		return ext
	}
	return ".txt"
}

// DocumentFileName is the name a document is exported under (FR-907), e.g.
// DOC-3.md. It is built from the identifier only, never from user text.
func DocumentFileName(identifier, format string) string {
	return identifier + DocumentFileExtension(format)
}

// Document states (FR-049). Only current documents are binding.
const (
	DocumentStateDraft      = "draft"
	DocumentStateCurrent    = "current"
	DocumentStateSuperseded = "superseded"
)

// DocumentStates lists the states in display order.
var DocumentStates = []string{DocumentStateDraft, DocumentStateCurrent, DocumentStateSuperseded}

// ValidDocumentState reports whether state is a known document state.
func ValidDocumentState(state string) bool {
	switch state {
	case DocumentStateDraft, DocumentStateCurrent, DocumentStateSuperseded:
		return true
	}
	return false
}

// Document link targets (FR-048). Tasks are deliberately absent.
const (
	DocumentLinkRequirement = "requirement"
	DocumentLinkConstraint  = "constraint"
	DocumentLinkTestCase    = "test_case"
)

// ValidDocumentLinkTarget reports whether a document can be linked to the
// target type.
func ValidDocumentLinkTarget(targetType string) bool {
	switch targetType {
	case DocumentLinkRequirement, DocumentLinkConstraint, DocumentLinkTestCase:
		return true
	}
	return false
}

// DocumentLink is a link between a document and a requirement, constraint
// or test case.
type DocumentLink struct {
	ID         string
	DocumentID string
	TargetType string // one of the DocumentLink* constants
	TargetID   string
	CreatedAt  string // RFC 3339
}

// DocumentRef names a document without its content (FR-909).
type DocumentRef struct {
	ID         string
	Identifier string
	Title      string
	Format     string
	State      string
}

// DefaultDocumentPrefix is the document identifier prefix of new products.
const DefaultDocumentPrefix = "DOC"
