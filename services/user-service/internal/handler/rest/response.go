package rest

import (
	"encoding/json"
	"net/http"

	apperrors "go-microservices-chat/pkg/errors"
	"go-microservices-chat/pkg/middleware"
)

// SuccessResponse は成功時の標準レスポンス形式
type SuccessResponse struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

// ListResponse はページネーション付きの一覧レスポンス形式
type ListResponse struct {
	Data any                `json:"data"`
	Meta MetaWithPagination `json:"meta"`
}

// ErrorResponse はエラー時の標準レスポンス形式
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
	Meta  Meta      `json:"meta"`
}

// ErrorBody はエラーの詳細情報
type ErrorBody struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details []apperrors.FieldError `json:"details,omitempty"`
}

// Meta はリクエストのメタ情報（リクエストID等）
type Meta struct {
	RequestID string `json:"request_id"`
}

// MetaWithPagination はページネーション情報付きのメタ情報
type MetaWithPagination struct {
	RequestID  string      `json:"request_id"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination はページネーションの状態
type Pagination struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// writeJSON はレスポンスをJSON形式で書き出す
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// respondSuccess は成功レスポンスを返す
func respondSuccess(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, status, SuccessResponse{
		Data: data,
		Meta: Meta{RequestID: middleware.GetRequestID(r.Context())},
	})
}

// respondList はページネーション付きの一覧レスポンスを返す
func respondList(w http.ResponseWriter, r *http.Request, data any, pagination *Pagination) {
	writeJSON(w, http.StatusOK, ListResponse{
		Data: data,
		Meta: MetaWithPagination{
			RequestID:  middleware.GetRequestID(r.Context()),
			Pagination: pagination,
		},
	})
}

// respondError はエラーレスポンスを返す
func respondError(w http.ResponseWriter, r *http.Request, status int, appErr *apperrors.AppError) {
	writeJSON(w, status, ErrorResponse{
		Error: ErrorBody{
			Code:    appErr.Code,
			Message: appErr.Message,
			Details: appErr.Details,
		},
		Meta: Meta{RequestID: middleware.GetRequestID(r.Context())},
	})
}

// handleError はエラーの種類に応じて適切なHTTPステータスコードでレスポンスを返す
func handleError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *apperrors.AppError
	if apperrors.As(err, &appErr) {
		switch {
		case apperrors.Is(err, apperrors.ErrNotFound):
			respondError(w, r, http.StatusNotFound, appErr)
		case apperrors.Is(err, apperrors.ErrValidation):
			respondError(w, r, http.StatusBadRequest, appErr)
		case apperrors.Is(err, apperrors.ErrConflict):
			respondError(w, r, http.StatusConflict, appErr)
		default:
			respondError(w, r, http.StatusInternalServerError, appErr)
		}
		return
	}

	// AppError以外のエラーは内部エラーとして処理
	respondError(w, r, http.StatusInternalServerError, apperrors.NewInternal(err))
}
