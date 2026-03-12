package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type TagHandler struct {
	service *service.TagService
}

func NewTagHandler(service *service.TagService) *TagHandler {
	return &TagHandler{service: service}
}

// ---------------------------------------------------------------------------
// 6.1 Tag CRUD
// ---------------------------------------------------------------------------

type createTagRequest struct {
	Name string `json:"name"`
}

func (h *TagHandler) CreateTag(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req createTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.service.CreateTag(ctx.Request.Context(), service.CreateTagInput{
		Name:       req.Name,
		AdminID:    identity.AdminID,
		OperatorIP: ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTagNameEmpty):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "tag name is required"})
		case errors.Is(err, service.ErrTagNameTooLong):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "tag name is too long"})
		case errors.Is(err, service.ErrTagNameDuplicate):
			ctx.JSON(http.StatusConflict, gin.H{"error": "a tag with this name already exists"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tag"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, item)
}

type updateTagRequest struct {
	Name string `json:"name"`
}

func (h *TagHandler) UpdateTag(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req updateTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.service.UpdateTag(ctx.Request.Context(), service.UpdateTagInput{
		TagID:      ctx.Param("tagID"),
		Name:       req.Name,
		AdminID:    identity.AdminID,
		OperatorIP: ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTagNameEmpty):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "tag name is required"})
		case errors.Is(err, service.ErrTagNameTooLong):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "tag name is too long"})
		case errors.Is(err, service.ErrTagNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		case errors.Is(err, service.ErrTagNameDuplicate):
			ctx.JSON(http.StatusConflict, gin.H{"error": "a tag with this name already exists"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tag"})
		}
		return
	}

	ctx.JSON(http.StatusOK, item)
}

func (h *TagHandler) DeleteTag(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	err := h.service.DeleteTag(ctx.Request.Context(), ctx.Param("tagID"), identity.AdminID, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTagNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete tag"})
		}
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (h *TagHandler) ListTags(ctx *gin.Context) {
	items, err := h.service.ListTags(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tags"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

// ---------------------------------------------------------------------------
// 6.2 File / Folder tag binding
// ---------------------------------------------------------------------------

type bindTagsRequest struct {
	Tags []string `json:"tags"`
}

func (h *TagHandler) BindFileTags(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req bindTagsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.BindFileTags(ctx.Request.Context(), service.BindFileTagsInput{
		FileID:     ctx.Param("fileID"),
		TagNames:   req.Tags,
		AdminID:    identity.AdminID,
		OperatorIP: ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		case errors.Is(err, service.ErrTagBindInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid tags"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to bind file tags"})
		}
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (h *TagHandler) BindFolderTags(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req bindTagsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.BindFolderTags(ctx.Request.Context(), service.BindFolderTagsInput{
		FolderID:   ctx.Param("folderID"),
		TagNames:   req.Tags,
		AdminID:    identity.AdminID,
		OperatorIP: ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderTreeNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		case errors.Is(err, service.ErrTagBindInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid tags"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to bind folder tags"})
		}
		return
	}

	ctx.Status(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// 6.3 Tag inheritance
// ---------------------------------------------------------------------------

func (h *TagHandler) GetFileTagsWithInheritance(ctx *gin.Context) {
	detail, err := h.service.GetFileTagsWithInheritance(ctx.Request.Context(), ctx.Param("fileID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get file tags"})
		}
		return
	}

	ctx.JSON(http.StatusOK, detail)
}

func (h *TagHandler) GetFolderTagsWithInheritance(ctx *gin.Context) {
	detail, err := h.service.GetFolderTagsWithInheritance(ctx.Request.Context(), ctx.Param("folderID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderTreeNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get folder tags"})
		}
		return
	}

	ctx.JSON(http.StatusOK, detail)
}

// ---------------------------------------------------------------------------
// 6.4 User candidate tag submissions
// ---------------------------------------------------------------------------

type submitCandidateTagRequest struct {
	ProposedName string `json:"proposed_name"`
}

func (h *TagHandler) SubmitCandidateTag(ctx *gin.Context) {
	var req submitCandidateTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	result, err := h.service.SubmitCandidateTag(ctx.Request.Context(), service.SubmitCandidateTagInput{
		ProposedName: req.ProposedName,
		SubmitterIP:  ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTagSubmissionNameEmpty):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "proposed_name is required"})
		case errors.Is(err, service.ErrTagNameTooLong):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "proposed_name is too long"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to submit candidate tag"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, result)
}

func (h *TagHandler) ListPendingTagSubmissions(ctx *gin.Context) {
	items, err := h.service.ListPendingTagSubmissions(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list pending tag submissions"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

type approveCandidateTagRequest struct {
	FinalName string `json:"final_name"`
}

func (h *TagHandler) ApproveCandidateTag(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req approveCandidateTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		// Allow empty body — final_name is optional.
		req = approveCandidateTagRequest{}
	}

	item, err := h.service.ApproveCandidateTag(ctx.Request.Context(), service.ApproveCandidateTagInput{
		SubmissionID: ctx.Param("submissionID"),
		FinalName:    req.FinalName,
		AdminID:      identity.AdminID,
		OperatorIP:   ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTagSubmissionNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "tag submission not found"})
		case errors.Is(err, service.ErrTagSubmissionNotPending):
			ctx.JSON(http.StatusConflict, gin.H{"error": "tag submission is not pending"})
		case errors.Is(err, service.ErrTagNameTooLong):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "final_name is too long"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve tag submission"})
		}
		return
	}

	ctx.JSON(http.StatusOK, item)
}

type rejectCandidateTagRequest struct {
	RejectReason string `json:"reject_reason"`
}

func (h *TagHandler) RejectCandidateTag(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req rejectCandidateTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.RejectCandidateTag(ctx.Request.Context(), service.RejectCandidateTagInput{
		SubmissionID: ctx.Param("submissionID"),
		RejectReason: req.RejectReason,
		AdminID:      identity.AdminID,
		OperatorIP:   ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrRejectReasonRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "reject_reason is required"})
		case errors.Is(err, service.ErrTagSubmissionNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "tag submission not found"})
		case errors.Is(err, service.ErrTagSubmissionNotPending):
			ctx.JSON(http.StatusConflict, gin.H{"error": "tag submission is not pending"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject tag submission"})
		}
		return
	}

	ctx.Status(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// 6.5 Tag governance — merge
// ---------------------------------------------------------------------------

type mergeTagsRequest struct {
	SourceTagID string `json:"source_tag_id"`
	TargetTagID string `json:"target_tag_id"`
}

func (h *TagHandler) MergeTags(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req mergeTagsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.MergeTags(ctx.Request.Context(), service.MergeTagsInput{
		SourceTagID: req.SourceTagID,
		TargetTagID: req.TargetTagID,
		AdminID:     identity.AdminID,
		OperatorIP:  ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTagMergeSameTag):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "source and target tag must be different"})
		case errors.Is(err, service.ErrTagNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "source tag not found"})
		case errors.Is(err, service.ErrTagMergeTargetNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "target tag not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to merge tags"})
		}
		return
	}

	ctx.Status(http.StatusNoContent)
}
