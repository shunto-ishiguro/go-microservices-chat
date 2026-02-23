package rest

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"go-microservices-chat/services/user-service/internal/domain"
	"go-microservices-chat/services/user-service/internal/service"
)

// UserHandler はユーザー関連のHTTPリクエストを処理する
type UserHandler struct {
	svc *service.UserService
}

// NewUserHandler は UserHandler を生成する
func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// Create は POST /api/v1/users を処理する（ユーザー作成）
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input domain.CreateUserInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":{"code":"BAD_REQUEST","message":"invalid JSON body"}}`, http.StatusBadRequest)
		return
	}

	user, err := h.svc.Create(r.Context(), input)
	if err != nil {
		handleError(w, r, err)
		return
	}

	respondSuccess(w, r, http.StatusCreated, user)
}

// Get は GET /api/v1/users/{id} を処理する（ユーザー詳細取得）
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	user, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		handleError(w, r, err)
		return
	}

	respondSuccess(w, r, http.StatusOK, user)
}

// List は GET /api/v1/users を処理する（ユーザー一覧取得、ページネーション付き）
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	users, total, err := h.svc.List(r.Context(), domain.ListUsersParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		handleError(w, r, err)
		return
	}

	respondList(w, r, users, &Pagination{
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// Update は PUT /api/v1/users/{id} を処理する（ユーザー更新）
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input domain.UpdateUserInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":{"code":"BAD_REQUEST","message":"invalid JSON body"}}`, http.StatusBadRequest)
		return
	}

	user, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		handleError(w, r, err)
		return
	}

	respondSuccess(w, r, http.StatusOK, user)
}

// Delete は DELETE /api/v1/users/{id} を処理する（ユーザー削除）
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.svc.Delete(r.Context(), id); err != nil {
		handleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
