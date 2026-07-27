package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	"nexmap.net/web/templates/patterns"
)

// Comment and attachment tabs.
//
// Unlike the link tabs, the payload here is identical for every owner type, so
// the logic lives once and each entity contributes only a thin wrapper that
// supplies its owner type and reads its own path value. ownerType is always a
// server-side constant and ownerID always comes from the route, never from the
// request body — the store rows carry no product scope of their own beyond the
// product_id the service stamps on them.

// --- Comments ---

func (c *PageProductController) commentsTab(w http.ResponseWriter, r *http.Request, ownerType, ownerID string) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	views, canComment, err := c.buildCommentViews(r, user, productID, ownerType, ownerID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = patterns.CommentList(
		views,
		commentsBaseURL(productID, ownerType, ownerID),
		middleware.CSRFTokenFromContext(r.Context()),
		canComment,
	).Render(r.Context(), w)
}

func (c *PageProductController) commentCreate(w http.ResponseWriter, r *http.Request, ownerType, ownerID string) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	body := r.FormValue("body")
	if body == "" {
		c.writeDiscussionError(w, r, patterns.CommentsErrorRegion,
			http.StatusBadRequest, "Write something before posting.")
		return
	}

	created, err := c.discussion.CreateComment(r.Context(), user, productID, ownerType, ownerID, body)
	if err != nil {
		c.writeDiscussionErrorFor(w, r, patterns.CommentsErrorRegion, err)
		return
	}

	// The form swaps with hx-swap="beforeend", so return exactly one item —
	// plus an out-of-band clear of the empty state.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = patterns.CommentCreated(
		c.commentView(created, user, c.displayNames(r, user), productID, ownerType, ownerID, true),
		middleware.CSRFTokenFromContext(r.Context()),
	).Render(r.Context(), w)
}

func (c *PageProductController) commentDelete(w http.ResponseWriter, r *http.Request, ownerType, ownerID string) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	commentID := r.PathValue("commentId")

	if err := c.discussion.DeleteComment(r.Context(), user, productID, commentID); err != nil {
		writePageServiceError(w, err)
		return
	}

	// hx-swap="outerHTML" against an empty body removes the row. If that was
	// the last one, bring the empty state back out of band.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if remaining, err := c.discussion.ListComments(r.Context(), user, productID, ownerType, ownerID); err == nil && len(remaining) == 0 {
		_ = patterns.CommentsEmptied().Render(r.Context(), w)
	}
}

// buildCommentViews resolves author display names and per-comment permissions.
// Mirrors buildAuditViews: display names are encrypted at rest and decrypted in
// the store layer, so they cannot be joined in SQL.
func (c *PageProductController) buildCommentViews(r *http.Request, user *model.User, productID, ownerType, ownerID string) ([]templates.CommentView, bool, error) {
	comments, err := c.discussion.ListComments(r.Context(), user, productID, ownerType, ownerID)
	if err != nil {
		return nil, false, err
	}

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return nil, false, err
	}
	canComment := user.IsAdmin || roleAtLeast(role, minRoleForOwner(ownerType))
	isOwner := user.IsAdmin || role == model.RoleOwner

	names := c.displayNames(r, user)
	views := make([]templates.CommentView, 0, len(comments))
	for _, cm := range comments {
		views = append(views, c.commentView(cm, user, names, productID, ownerType, ownerID, isOwner))
	}
	return views, canComment, nil
}

func (c *PageProductController) commentView(cm *model.Comment, user *model.User, names map[string]string, productID, ownerType, ownerID string, productOwner bool) templates.CommentView {
	author := cm.UserID
	if dn, ok := names[cm.UserID]; ok {
		author = dn
	}
	mine := cm.UserID == user.ID
	canDelete := mine || productOwner

	v := templates.CommentView{
		ID:         cm.ID,
		AuthorName: author,
		Body:       cm.Body,
		CreatedAt:  formatTimestamp(cm.CreatedAt),
		IsOwner:    mine,
		CanDelete:  canDelete,
	}
	if canDelete {
		v.DeleteURL = commentsBaseURL(productID, ownerType, ownerID) + "/" + cm.ID
	}
	return v
}

// --- Attachments ---

func (c *PageProductController) attachmentsTab(w http.ResponseWriter, r *http.Request, ownerType, ownerID string) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	views, canUpload, err := c.buildAttachmentViews(r, user, productID, ownerType, ownerID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = patterns.AttachmentList(
		views,
		attachmentsBaseURL(productID, ownerType, ownerID),
		middleware.CSRFTokenFromContext(r.Context()),
		canUpload,
		c.maxUploadBytes,
	).Render(r.Context(), w)
}

func (c *PageProductController) attachmentCreate(w http.ResponseWriter, r *http.Request, ownerType, ownerID string) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	file, header, err := r.FormFile("file")
	if err != nil {
		// A body over the router's cap fails here rather than in storage, so
		// it needs its own message — "a file is required" would be misleading.
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			c.writeDiscussionError(w, r, patterns.AttachmentsErrorRegion,
				http.StatusRequestEntityTooLarge, oversizeMessage(c.maxUploadBytes))
			return
		}
		c.writeDiscussionError(w, r, patterns.AttachmentsErrorRegion,
			http.StatusBadRequest, "Choose a file to upload.")
		return
	}
	defer func() { _ = file.Close() }()

	created, err := c.discussion.CreateAttachment(r.Context(), user, productID, service.CreateAttachmentInput{
		OwnerType:   ownerType,
		OwnerID:     ownerID,
		Filename:    filepath.Base(header.Filename),
		ContentType: header.Header.Get("Content-Type"),
		Content:     file,
	})
	if err != nil {
		c.writeDiscussionErrorFor(w, r, patterns.AttachmentsErrorRegion, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = patterns.AttachmentCreated(
		attachmentView(created, productID, ownerType, ownerID, true),
		middleware.CSRFTokenFromContext(r.Context()),
	).Render(r.Context(), w)
}

func (c *PageProductController) attachmentDelete(w http.ResponseWriter, r *http.Request, ownerType, ownerID string) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	attachmentID := r.PathValue("attachmentId")

	if err := c.discussion.DeleteAttachment(r.Context(), user, productID, attachmentID); err != nil {
		writePageServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if remaining, err := c.discussion.ListAttachments(r.Context(), user, productID, ownerType, ownerID); err == nil && len(remaining) == 0 {
		_ = patterns.AttachmentsEmptied().Render(r.Context(), w)
	}
}

// AttachmentDownload streams an attachment. It needs no owner type: the
// attachment ID plus product scope fully identify the file.
func (c *PageProductController) AttachmentDownload(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	attachmentID := r.PathValue("attachmentId")

	a, rc, err := c.discussion.OpenAttachment(r.Context(), user, productID, attachmentID)
	if err != nil {
		writePageServiceError(w, err)
		return
	}
	defer func() { _ = rc.Close() }()

	writeAttachmentDownload(w, a, rc)
}

func (c *PageProductController) buildAttachmentViews(r *http.Request, user *model.User, productID, ownerType, ownerID string) ([]templates.AttachmentView, bool, error) {
	items, err := c.discussion.ListAttachments(r.Context(), user, productID, ownerType, ownerID)
	if err != nil {
		return nil, false, err
	}

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return nil, false, err
	}
	canUpload := user.IsAdmin || roleAtLeast(role, minRoleForOwner(ownerType))
	productOwner := user.IsAdmin || role == model.RoleOwner

	views := make([]templates.AttachmentView, 0, len(items))
	for _, a := range items {
		views = append(views, attachmentView(a, productID, ownerType, ownerID, a.UserID == user.ID || productOwner))
	}
	return views, canUpload, nil
}

func attachmentView(a *model.Attachment, productID, ownerType, ownerID string, canDelete bool) templates.AttachmentView {
	base := attachmentsBaseURL(productID, ownerType, ownerID)
	v := templates.AttachmentView{
		ID:          a.ID,
		Filename:    a.Filename,
		ContentType: a.ContentType,
		Size:        patterns.HumanizeBytes(a.Size),
		CreatedAt:   formatTimestamp(a.CreatedAt),
		DownloadURL: base + "/" + a.ID + "/download",
	}
	if canDelete {
		v.DeleteURL = base + "/" + a.ID
	}
	return v
}

// --- Shared helpers ---

// ownerSegments maps an owner type to its URL path segment.
var ownerSegments = map[string]string{
	model.OwnerTypeRequirement: "requirements",
	model.OwnerTypeConstraint:  "constraints",
	model.OwnerTypeTestCase:    "test-cases",
	model.OwnerTypeComponent:   "components",
	model.OwnerTypeCapability:  "capabilities",
	model.OwnerTypeMilestone:   "milestones",
	model.OwnerTypeDefect:      "defects",
}

func commentsBaseURL(productID, ownerType, ownerID string) string {
	return fmt.Sprintf("/products/%s/%s/%s/comments", productID, ownerSegments[ownerType], ownerID)
}

func attachmentsBaseURL(productID, ownerType, ownerID string) string {
	return fmt.Sprintf("/products/%s/%s/%s/attachments", productID, ownerSegments[ownerType], ownerID)
}

// minRoleForOwner mirrors the service's role mapping so the UI hides controls
// the server would reject anyway.
func minRoleForOwner(ownerType string) string {
	switch ownerType {
	case model.OwnerTypeDefect, model.OwnerTypeTestCase:
		return model.RoleTester
	default:
		return model.RoleDeveloper
	}
}

func roleAtLeast(role, minRole string) bool {
	return rolePriority(role) >= rolePriority(minRole)
}

func rolePriority(role string) int {
	switch role {
	case model.RoleOwner:
		return 3
	case model.RoleDeveloper:
		return 2
	case model.RoleTester:
		return 1
	case model.RoleViewer:
		return 0
	default:
		return -1
	}
}

// displayNames returns a userID → display name map. Display names are
// encrypted at rest, so this cannot be a SQL join.
func (c *PageProductController) displayNames(r *http.Request, user *model.User) map[string]string {
	allUsers, _ := c.user.List(r.Context(), store.UserFilter{})
	names := make(map[string]string, len(allUsers))
	for _, u := range allUsers {
		names[u.ID] = u.DisplayName
	}
	return names
}

// formatTimestamp renders an RFC 3339 timestamp for display, falling back to
// the raw value if it cannot be parsed.
func formatTimestamp(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("2 Jan 2006, 15:04")
}

// writeDiscussionErrorFor renders a service error into a form's error region.
func (c *PageProductController) writeDiscussionErrorFor(w http.ResponseWriter, r *http.Request, region string, err error) {
	status, msg := discussionError(err, c.maxUploadBytes)
	c.writeDiscussionError(w, r, region, status, msg)
}

// uploadMemoryLimit is how much of a multipart upload is buffered in RAM
// before spilling to temp files. The total is bounded by LimitUploadBody.
const uploadMemoryLimit = 1 << 20

// LimitUploadBody caps the request body and parses the multipart form before
// anything else reads it.
//
// This has to happen ahead of CSRF validation: that middleware calls
// r.FormValue, which parses the body itself. If the body is over the cap the
// parse fails there, CSRF cannot find the token, and the user gets an opaque
// 403 instead of a size error — with htmx discarding it, no message at all.
// Parsing here means the size problem is reported properly, and CSRF then
// reads the already-parsed form.
func (c *PageProductController) LimitUploadBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Headroom for the multipart envelope around the file itself.
		const envelope = 1 << 20
		body := r.Body
		r.Body = http.MaxBytesReader(w, body, c.maxUploadBytes+envelope)

		if err := r.ParseMultipartForm(uploadMemoryLimit); err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				drainRemainder(body)
				c.writeDiscussionError(w, r, patterns.AttachmentsErrorRegion,
					http.StatusRequestEntityTooLarge, oversizeMessage(c.maxUploadBytes))
				return
			}
			c.writeDiscussionError(w, r, patterns.AttachmentsErrorRegion,
				http.StatusBadRequest, "That upload could not be read. Please try again.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func oversizeMessage(maxBytes int64) string {
	return patterns.OversizeMessage(maxBytes)
}

// maxDrainBytes bounds how much of a rejected upload is read and discarded.
const maxDrainBytes = 32 << 20

// drainRemainder consumes what is left of an oversize request body.
//
// When the body cap trips, Go sets Connection: close and stops reading. If the
// client is still uploading, the connection is torn down before it can read
// the response, so the rejection never reaches it. Consuming the rest lets the
// client finish and receive the reply. The bound keeps a deliberately huge
// upload from costing us the whole transfer — past that the connection simply
// closes, which is the correct outcome for a client ignoring the limit.
//
// Browsers never reach this: the upload form checks the size before sending.
func drainRemainder(body io.Reader) {
	_, _ = io.CopyN(io.Discard, body, maxDrainBytes)
}

// discussionErrorHeader marks a response as a rejection even though it carries
// a 200 status, so the form knows not to reset and discard the user's input.
const discussionErrorHeader = "HX-Nexmap-Error"

// writeDiscussionError sends the message as an out-of-band swap into region.
//
// The status is deliberately 200. htmx does not process non-2xx responses at
// all, and its htmx:beforeSwap escape hatch fires on the swap target rather
// than on the form, so an error status here reaches the user as nothing but a
// log line. The primary body stays empty, so the list target — which appends
// with hx-swap="beforeend" — receives nothing and only the out-of-band region
// updates. The real status codes are still returned by the JSON API, which has
// no such constraint.
func (c *PageProductController) writeDiscussionError(w http.ResponseWriter, r *http.Request, region string, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set(discussionErrorHeader, http.StatusText(status))
	w.WriteHeader(http.StatusOK)
	_ = patterns.InlineError(region, message).Render(r.Context(), w)
}

// discussionError maps an error to a status and a message written for the
// person who tried the action, not for a log.
func discussionError(err error, maxUploadBytes int64) (int, string) {
	switch {
	case errors.Is(err, service.ErrForbidden):
		return http.StatusForbidden, "You do not have permission to do that."
	case errors.Is(err, service.ErrNotFound), errors.Is(err, storage.ErrNotFound):
		return http.StatusNotFound, "That item no longer exists. Reload the page."
	case errors.Is(err, storage.ErrTooLarge):
		return http.StatusRequestEntityTooLarge, oversizeMessage(maxUploadBytes)
	case errors.Is(err, storage.ErrTypeNotAllowed):
		// The policy error already names the permitted types.
		return http.StatusUnsupportedMediaType, uploadRejectionMessage(err)
	default:
		return http.StatusUnprocessableEntity, "That did not work. Please try again."
	}
}

// uploadRejectionMessage turns a policy error into a sentence.
func uploadRejectionMessage(err error) string {
	msg := strings.TrimPrefix(err.Error(), storage.ErrTypeNotAllowed.Error()+": ")
	if msg == "" || msg == err.Error() {
		return "That file type is not allowed."
	}
	return "That file type is not allowed — " + msg + "."
}

// writePageServiceError maps service errors onto plain-text HTTP responses
// suitable for an htmx swap target.
func writePageServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, service.ErrNotFound), errors.Is(err, storage.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, storage.ErrTooLarge):
		http.Error(w, "file exceeds the maximum upload size", http.StatusRequestEntityTooLarge)
	case errors.Is(err, storage.ErrTypeNotAllowed):
		// The message carries the allow-list, so surface it verbatim.
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
	default:
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	}
}

// --- Per-entity wrappers ---

func (c *PageProductController) RequirementCommentsTab(w http.ResponseWriter, r *http.Request) {
	c.commentsTab(w, r, model.OwnerTypeRequirement, r.PathValue("requirementId"))
}
func (c *PageProductController) RequirementCommentCreate(w http.ResponseWriter, r *http.Request) {
	c.commentCreate(w, r, model.OwnerTypeRequirement, r.PathValue("requirementId"))
}
func (c *PageProductController) RequirementCommentDelete(w http.ResponseWriter, r *http.Request) {
	c.commentDelete(w, r, model.OwnerTypeRequirement, r.PathValue("requirementId"))
}
func (c *PageProductController) RequirementAttachmentsTab(w http.ResponseWriter, r *http.Request) {
	c.attachmentsTab(w, r, model.OwnerTypeRequirement, r.PathValue("requirementId"))
}
func (c *PageProductController) RequirementAttachmentCreate(w http.ResponseWriter, r *http.Request) {
	c.attachmentCreate(w, r, model.OwnerTypeRequirement, r.PathValue("requirementId"))
}
func (c *PageProductController) RequirementAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	c.attachmentDelete(w, r, model.OwnerTypeRequirement, r.PathValue("requirementId"))
}

func (c *PageProductController) DefectCommentsTab(w http.ResponseWriter, r *http.Request) {
	c.commentsTab(w, r, model.OwnerTypeDefect, r.PathValue("defectId"))
}
func (c *PageProductController) DefectCommentCreate(w http.ResponseWriter, r *http.Request) {
	c.commentCreate(w, r, model.OwnerTypeDefect, r.PathValue("defectId"))
}
func (c *PageProductController) DefectCommentDelete(w http.ResponseWriter, r *http.Request) {
	c.commentDelete(w, r, model.OwnerTypeDefect, r.PathValue("defectId"))
}
func (c *PageProductController) DefectAttachmentsTab(w http.ResponseWriter, r *http.Request) {
	c.attachmentsTab(w, r, model.OwnerTypeDefect, r.PathValue("defectId"))
}
func (c *PageProductController) DefectAttachmentCreate(w http.ResponseWriter, r *http.Request) {
	c.attachmentCreate(w, r, model.OwnerTypeDefect, r.PathValue("defectId"))
}
func (c *PageProductController) DefectAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	c.attachmentDelete(w, r, model.OwnerTypeDefect, r.PathValue("defectId"))
}

func (c *PageProductController) ComponentCommentsTab(w http.ResponseWriter, r *http.Request) {
	c.commentsTab(w, r, model.OwnerTypeComponent, r.PathValue("componentId"))
}
func (c *PageProductController) ComponentCommentCreate(w http.ResponseWriter, r *http.Request) {
	c.commentCreate(w, r, model.OwnerTypeComponent, r.PathValue("componentId"))
}
func (c *PageProductController) ComponentCommentDelete(w http.ResponseWriter, r *http.Request) {
	c.commentDelete(w, r, model.OwnerTypeComponent, r.PathValue("componentId"))
}
func (c *PageProductController) ComponentAttachmentsTab(w http.ResponseWriter, r *http.Request) {
	c.attachmentsTab(w, r, model.OwnerTypeComponent, r.PathValue("componentId"))
}
func (c *PageProductController) ComponentAttachmentCreate(w http.ResponseWriter, r *http.Request) {
	c.attachmentCreate(w, r, model.OwnerTypeComponent, r.PathValue("componentId"))
}
func (c *PageProductController) ComponentAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	c.attachmentDelete(w, r, model.OwnerTypeComponent, r.PathValue("componentId"))
}

func (c *PageProductController) ConstraintCommentsTab(w http.ResponseWriter, r *http.Request) {
	c.commentsTab(w, r, model.OwnerTypeConstraint, r.PathValue("constraintId"))
}
func (c *PageProductController) ConstraintCommentCreate(w http.ResponseWriter, r *http.Request) {
	c.commentCreate(w, r, model.OwnerTypeConstraint, r.PathValue("constraintId"))
}
func (c *PageProductController) ConstraintCommentDelete(w http.ResponseWriter, r *http.Request) {
	c.commentDelete(w, r, model.OwnerTypeConstraint, r.PathValue("constraintId"))
}
func (c *PageProductController) ConstraintAttachmentsTab(w http.ResponseWriter, r *http.Request) {
	c.attachmentsTab(w, r, model.OwnerTypeConstraint, r.PathValue("constraintId"))
}
func (c *PageProductController) ConstraintAttachmentCreate(w http.ResponseWriter, r *http.Request) {
	c.attachmentCreate(w, r, model.OwnerTypeConstraint, r.PathValue("constraintId"))
}
func (c *PageProductController) ConstraintAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	c.attachmentDelete(w, r, model.OwnerTypeConstraint, r.PathValue("constraintId"))
}

func (c *PageProductController) TestCaseCommentsTab(w http.ResponseWriter, r *http.Request) {
	c.commentsTab(w, r, model.OwnerTypeTestCase, r.PathValue("testCaseId"))
}
func (c *PageProductController) TestCaseCommentCreate(w http.ResponseWriter, r *http.Request) {
	c.commentCreate(w, r, model.OwnerTypeTestCase, r.PathValue("testCaseId"))
}
func (c *PageProductController) TestCaseCommentDelete(w http.ResponseWriter, r *http.Request) {
	c.commentDelete(w, r, model.OwnerTypeTestCase, r.PathValue("testCaseId"))
}
func (c *PageProductController) TestCaseAttachmentsTab(w http.ResponseWriter, r *http.Request) {
	c.attachmentsTab(w, r, model.OwnerTypeTestCase, r.PathValue("testCaseId"))
}
func (c *PageProductController) TestCaseAttachmentCreate(w http.ResponseWriter, r *http.Request) {
	c.attachmentCreate(w, r, model.OwnerTypeTestCase, r.PathValue("testCaseId"))
}
func (c *PageProductController) TestCaseAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	c.attachmentDelete(w, r, model.OwnerTypeTestCase, r.PathValue("testCaseId"))
}

func (c *PageProductController) CapabilityCommentsTab(w http.ResponseWriter, r *http.Request) {
	c.commentsTab(w, r, model.OwnerTypeCapability, r.PathValue("capabilityId"))
}
func (c *PageProductController) CapabilityCommentCreate(w http.ResponseWriter, r *http.Request) {
	c.commentCreate(w, r, model.OwnerTypeCapability, r.PathValue("capabilityId"))
}
func (c *PageProductController) CapabilityCommentDelete(w http.ResponseWriter, r *http.Request) {
	c.commentDelete(w, r, model.OwnerTypeCapability, r.PathValue("capabilityId"))
}
func (c *PageProductController) CapabilityAttachmentsTab(w http.ResponseWriter, r *http.Request) {
	c.attachmentsTab(w, r, model.OwnerTypeCapability, r.PathValue("capabilityId"))
}
func (c *PageProductController) CapabilityAttachmentCreate(w http.ResponseWriter, r *http.Request) {
	c.attachmentCreate(w, r, model.OwnerTypeCapability, r.PathValue("capabilityId"))
}
func (c *PageProductController) CapabilityAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	c.attachmentDelete(w, r, model.OwnerTypeCapability, r.PathValue("capabilityId"))
}

func (c *PageProductController) MilestoneCommentsTab(w http.ResponseWriter, r *http.Request) {
	c.commentsTab(w, r, model.OwnerTypeMilestone, r.PathValue("milestoneId"))
}
func (c *PageProductController) MilestoneCommentCreate(w http.ResponseWriter, r *http.Request) {
	c.commentCreate(w, r, model.OwnerTypeMilestone, r.PathValue("milestoneId"))
}
func (c *PageProductController) MilestoneCommentDelete(w http.ResponseWriter, r *http.Request) {
	c.commentDelete(w, r, model.OwnerTypeMilestone, r.PathValue("milestoneId"))
}
func (c *PageProductController) MilestoneAttachmentsTab(w http.ResponseWriter, r *http.Request) {
	c.attachmentsTab(w, r, model.OwnerTypeMilestone, r.PathValue("milestoneId"))
}
func (c *PageProductController) MilestoneAttachmentCreate(w http.ResponseWriter, r *http.Request) {
	c.attachmentCreate(w, r, model.OwnerTypeMilestone, r.PathValue("milestoneId"))
}
func (c *PageProductController) MilestoneAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	c.attachmentDelete(w, r, model.OwnerTypeMilestone, r.PathValue("milestoneId"))
}
