package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go-microservices-chat/pkg/logger"
	"go-microservices-chat/services/user-service/internal/domain"
	"go-microservices-chat/services/user-service/internal/service"
)

// --- テスト用のインメモリリポジトリ ---

type memoryRepo struct {
	mu    sync.RWMutex
	users map[string]*domain.User
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{users: make(map[string]*domain.User)}
}

func (m *memoryRepo) Create(_ context.Context, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user.ID] = user
	return nil
}

func (m *memoryRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *memoryRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, nil
}

func (m *memoryRepo) GetByUsername(_ context.Context, username string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, nil
}

func (m *memoryRepo) List(_ context.Context, limit, offset int) ([]*domain.User, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	all := make([]*domain.User, 0, len(m.users))
	for _, u := range m.users {
		all = append(all, u)
	}

	total := len(all)
	if offset >= total {
		return []*domain.User{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (m *memoryRepo) Update(_ context.Context, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user.ID] = user
	return nil
}

func (m *memoryRepo) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.users, id)
	return nil
}

// setupRouter はテスト用のルーターを組み立てる
func setupRouter() http.Handler {
	repo := newMemoryRepo()
	svc := service.NewUserService(repo)
	log := logger.New("error")
	return NewRouter(svc, log)
}

// --- ハンドラーテスト ---

func TestHealthEndpoint(t *testing.T) {
	router := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ステータス: 期待=200, 実際=%d", w.Code)
	}
}

func TestCreateUserHandler(t *testing.T) {
	router := setupRouter()

	body := `{"email":"handler@example.com","username":"handleruser","display_name":"Handler Test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("ステータス: 期待=201, 実際=%d; body: %s", w.Code, w.Body.String())
	}

	var resp SuccessResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("レスポンスのデコードに失敗: %v", err)
	}
	if resp.Meta.RequestID == "" {
		t.Error("meta に request_id が含まれていない")
	}
}

func TestCreateUserHandler_ValidationError(t *testing.T) {
	router := setupRouter()

	body := `{"email":"","username":"","display_name":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ステータス: 期待=400, 実際=%d; body: %s", w.Code, w.Body.String())
	}
}

func TestGetUserHandler_NotFound(t *testing.T) {
	router := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/nonexistent-id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("ステータス: 期待=404, 実際=%d", w.Code)
	}
}

func TestListUsersHandler(t *testing.T) {
	router := setupRouter()

	// まずユーザーを1人作成
	body := `{"email":"list@example.com","username":"listuser","display_name":"List User"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 一覧取得
	req = httptest.NewRequest(http.MethodGet, "/api/v1/users?limit=10&offset=0", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ステータス: 期待=200, 実際=%d", w.Code)
	}
}

func TestCreateAndGetUser(t *testing.T) {
	router := setupRouter()

	// 作成
	body := `{"email":"crudtest@example.com","username":"cruduser","display_name":"CRUD Test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("作成: 期待=201, 実際=%d; body: %s", w.Code, w.Body.String())
	}

	var createResp struct {
		Data domain.User `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&createResp)
	userID := createResp.Data.ID

	// 取得
	req = httptest.NewRequest(http.MethodGet, "/api/v1/users/"+userID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("取得: 期待=200, 実際=%d", w.Code)
	}
}

func TestUpdateUserHandler(t *testing.T) {
	router := setupRouter()

	// 作成
	body := `{"email":"upd@example.com","username":"upduser","display_name":"Original"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var createResp struct {
		Data domain.User `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&createResp)
	userID := createResp.Data.ID

	// 更新
	updateBody := `{"display_name":"Updated Name"}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/users/"+userID, bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("更新: 期待=200, 実際=%d; body: %s", w.Code, w.Body.String())
	}
}

func TestDeleteUserHandler(t *testing.T) {
	router := setupRouter()

	// 作成
	body := `{"email":"del@example.com","username":"deluser","display_name":"Delete Me"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var createResp struct {
		Data domain.User `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&createResp)
	userID := createResp.Data.ID

	// 削除
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+userID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("削除: 期待=204, 実際=%d", w.Code)
	}

	// 削除後に取得 → 404 になるべき
	req = httptest.NewRequest(http.MethodGet, "/api/v1/users/"+userID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("削除後の取得: 期待=404, 実際=%d", w.Code)
	}
}

func TestCreateUserHandler_InvalidJSON(t *testing.T) {
	router := setupRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ステータス: 期待=400, 実際=%d", w.Code)
	}
}
