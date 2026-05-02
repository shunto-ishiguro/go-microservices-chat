package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"go-microservices-chat/pkg/auth"
)

// refreshTokenTTL: リフレッシュトークンの有効期限。
// access token が 15 分で切れた後、この期間内なら再ログイン無しで新しいペアを取れる。
// 長すぎると漏洩時の被害が大きくなり、短すぎると UX が悪化するので妥協点として 30 日。
const refreshTokenTTL = 30 * 24 * time.Hour

// TokenIssuer は Service が依存する *auth.Issuer の最小インターフェイス。
// 単体テストで fake を差し込めるように interface にしている。
type TokenIssuer interface {
	IssueAccessToken(userID, username string) (string, error)
}

// Service は user-service のビジネスロジックを持つ。
// gRPC トランスポート層と Repository の間に位置する。
type Service struct {
	repo   Repository
	issuer TokenIssuer
	now    func() time.Time
}

func NewService(repo Repository, issuer TokenIssuer) *Service {
	return &Service{repo: repo, issuer: issuer, now: time.Now}
}

// ============================================================
// External API: Web クライアントから呼ばれる (Envoy 経由)
// ============================================================
//
// Register/Login/Refresh は未認証経路 (x-user-id なしで到達)。
// GetMe/UpdateMe は x-user-id 経由で自分のリソースにしか触れない。

// Register は bcrypt でハッシュ化したパスワードで新規ユーザーを作成する。
func (s *Service) Register(ctx context.Context, email, username, displayName, password string) (*User, error) {
	// email は小文字に揃えてから保存 (ログイン時の検索でケース差異による不一致を防ぐ)。
	email = strings.ToLower(strings.TrimSpace(email))
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	if email == "" || username == "" || displayName == "" || password == "" {
		return nil, fmt.Errorf("%w: missing required field", ErrInvalidCreds)
	}
	// bcrypt.DefaultCost = 10。コストは 2^N 回のストレッチ相当で、CPU コストと
	// ブルートフォース耐性のトレードオフ。ハードウェア進化に合わせ 4〜5 年に一度見直す。
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	now := s.now().UTC()
	u := &User{
		ID:           uuid.NewString(),
		Email:        email,
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  displayName,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.CreateUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Login は認証情報を検証し、アクセストークンとリフレッシュトークンを返す。
//
// セキュリティ上のポイント:
//   - ユーザー未存在 / パスワード不一致を区別せず常に ErrInvalidCreds を返す
//     (user enumeration 攻撃を防ぐため、「メールアドレスが登録されてますか?」を
//      エラーの差で答えない)
func (s *Service) Login(ctx context.Context, email, password string) (accessToken, refreshToken string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", "", ErrInvalidCreds
		}
		return "", "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", "", ErrInvalidCreds
	}
	return s.issueTokenPair(ctx, u)
}

// Refresh はリフレッシュトークンをローテーションする。古いトークンを失効させ、
// 新しいトークンペアを返す。
//
// なぜローテーション (rotation) するか:
//   - リフレッシュトークンが漏洩した時、攻撃者が使うと正規ユーザーも旧トークンが
//     revoked になるので、次の Refresh 時に正規ユーザーが気付ける (漏洩検知手段)
//   - 「1 トークンは 1 回しか使えない」原則により、盗まれたトークンの利用が
//     検出しやすくなる
func (s *Service) Refresh(ctx context.Context, refreshToken string) (newAccess, newRefresh string, err error) {
	// 渡された生トークンを sha256 でハッシュしてから検索。DB 側には生値を保存していないので
	// DB 漏洩時に既存トークンが悪用されない (片方向性)。
	hash := hashToken(refreshToken)
	stored, err := s.repo.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", "", ErrTokenInvalid
		}
		return "", "", err
	}
	now := s.now().UTC()
	// 既に revoke 済み or 期限切れなら拒否。
	if stored.RevokedAt != nil || now.After(stored.ExpiresAt) {
		return "", "", ErrTokenInvalid
	}
	// 旧トークンを先に revoke → 新トークンペアを発行。順序が逆だと「新旧両方使える」窓ができる。
	if err := s.repo.RevokeRefreshToken(ctx, stored.ID, now); err != nil {
		return "", "", err
	}
	u, err := s.repo.GetUserByID(ctx, stored.UserID)
	if err != nil {
		return "", "", err
	}
	return s.issueTokenPair(ctx, u)
}

// GetMe は呼び出し元自身のプロフィールを返す。対象 ID はゲートウェイが
// 注入した x-user-id から解決するので、他人のデータは構造上取れない。
func (s *Service) GetMe(ctx context.Context) (*User, error) {
	id, ok := auth.RequesterID(ctx)
	if !ok {
		return nil, ErrInvalidCreds
	}
	return s.repo.GetUserByID(ctx, id)
}

// UpdateMe は呼び出し元自身のプロフィールを更新する。x-user-id から
// 対象を解決するので、リソース所有者認可は不要 (型レベルで他人は触れない)。
func (s *Service) UpdateMe(ctx context.Context, displayName, avatarURL, statusText *string) (*User, error) {
	id, ok := auth.RequesterID(ctx)
	if !ok {
		return nil, ErrInvalidCreds
	}
	u, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if displayName != nil {
		u.DisplayName = *displayName
	}
	if avatarURL != nil {
		u.AvatarURL = *avatarURL
	}
	if statusText != nil {
		u.StatusText = *statusText
	}
	u.UpdatedAt = s.now().UTC()
	if err := s.repo.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// ============================================================
// Internal API: 他サービス (chat-service 等) から呼ばれる
// ============================================================
//
// 画面 #9 のメンバー一覧などで chat-service が member enrich する用途。
// Web クライアントには直接公開しない想定 (Phase 3 以降に Envoy 側で
// 外部パスから外すかどうかを検討)。

// GetUser は他ユーザー 1 件を取得する (画面 #8 メンバー詳細モーダル)。
func (s *Service) GetUser(ctx context.Context, id string) (*User, error) {
	return s.repo.GetUserByID(ctx, id)
}

// BatchGetUsers は他ユーザー N 件を 1 回の DB 呼び出しで取得する。
// chat-service の `ListRoomMembers` がメンバー一覧の enrich (display_name / avatar_url を引く)
// に使う。Message の sender 側は Phase 2 では enrich しない設計なのでここでは関与しない。
// 存在しない ID は結果から欠落する (部分成功を許容)。
func (s *Service) BatchGetUsers(ctx context.Context, ids []string) ([]User, error) {
	return s.repo.GetUsersByIDs(ctx, ids)
}

// issueTokenPair はアクセストークン + リフレッシュトークンのペアを発行する。
//
// 2 種類のトークンを使い分ける理由:
//   - Access token (短命 15 分, RS256 JWT): 各リクエストに付与。漏洩しても短時間で無効化
//   - Refresh token (長命 30 日, 不透明文字列): access 更新時のみ使用。DB に hash 保存、revoke 可能
//
// JWT と違って refresh token は中身に意味を持たない (不透明)。だから署名ではなく
// 32 byte の乱数 + DB に sha256 保存 + revocable という設計。
func (s *Service) issueTokenPair(ctx context.Context, u *User) (string, string, error) {
	access, err := s.issuer.IssueAccessToken(u.ID, u.Username)
	if err != nil {
		return "", "", err
	}
	raw, err := newRefreshTokenValue()
	if err != nil {
		return "", "", err
	}
	now := s.now().UTC()
	// DB には hash のみ保存、生値はこの関数の戻り値でクライアントに 1 度だけ渡す。
	if err := s.repo.CreateRefreshToken(ctx, &RefreshToken{
		ID:        uuid.NewString(),
		UserID:    u.ID,
		TokenHash: hashToken(raw),
		ExpiresAt: now.Add(refreshTokenTTL),
		CreatedAt: now,
	}); err != nil {
		return "", "", err
	}
	return access, raw, nil
}

// newRefreshTokenValue は 32 byte の暗号学的乱数を hex 化して返す (= 64 文字)。
// 推測不能性が命なので math/rand ではなく crypto/rand を使う。
func newRefreshTokenValue() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// hashToken は検索キー用の sha256 ハッシュを返す。bcrypt ではなく sha256 な理由:
//   - refresh token は 32 byte の乱数 (= 既に推測不能) なので salt + stretch 不要
//   - ログイン時のパスワード突合と違って per-request に高速比較したい
//   - DB インデックスに載せて O(1) で引きたい
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
