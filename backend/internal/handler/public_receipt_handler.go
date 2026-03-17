package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
)

type PublicReceiptHandler struct {
	receiptCodes *service.ReceiptCodeService
}

func NewPublicReceiptHandler(receiptCodes *service.ReceiptCodeService) *PublicReceiptHandler {
	return &PublicReceiptHandler{receiptCodes: receiptCodes}
}

func (h *PublicReceiptHandler) Ensure(ctx *gin.Context) {
	receiptCode, err := h.receiptCodes.ResolveForSession(ctx.Request.Context(), readPublicReceiptCode(ctx))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrReceiptCodeGenerate):
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate receipt code"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load receipt code"})
		}
		return
	}

	writePublicReceiptCode(ctx, receiptCode)
	ctx.JSON(http.StatusOK, gin.H{"receipt_code": receiptCode})
}
