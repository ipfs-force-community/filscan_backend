package bearer

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/gozelle/gin"
	"github.com/stretchr/testify/require"
)

// signToken 用给定密钥签发一个合法 JWT，模拟登录侧 GenJWT 的产物。
func signToken(t *testing.T, secret string) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, JWTClaims{
		StandardClaims: jwt.StandardClaims{
			Id:        "test-jti",
			ExpiresAt: now.Add(time.Hour).Unix(),
			Issuer:    "FILSCAN",
			Subject:   "API Auth",
			NotBefore: now.Add(-time.Minute).Unix(),
			IssuedAt:  now.Unix(),
		},
		UserID: 42,
		Mail:   "victim@example.com",
	})
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

// newRouter 挂上待测中间件与一个业务终结点，reached 反映请求是否穿过中间件到达业务。
func newRouter(secret string) (engine *gin.Engine, reached *bool) {
	gin.SetMode(gin.TestMode)
	hit := false
	r := gin.New()
	r.Use(Authentication(secret))
	r.GET("/pro/v1/UserInfo", func(c *gin.Context) {
		hit = true
		c.Status(http.StatusOK)
	})
	r.POST("/pro/v1/Login", func(c *gin.Context) {
		hit = true
		c.Status(http.StatusOK)
	})
	return r, &hit
}

// ① 用正确密钥签发的 token 能通过。
func TestAuthenticationAcceptsTokenSignedWithConfiguredSecret(t *testing.T) {
	const configured = "configured-jwt-secret"
	r, reached := newRouter(configured)

	req := httptest.NewRequest(http.MethodGet, "/pro/v1/UserInfo", nil)
	req.Header.Set("Authorization", "bearer "+signToken(t, configured))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.True(t, *reached, "正确密钥签发的 token 应当放行到业务处理")
	require.Equal(t, http.StatusOK, rec.Code)
}

// ② 用旧硬编码密钥 `12345678` 签发的 token 必须被拒（401）。
// 这是本次修复的核心回归用例：现场漏洞即通过该常量自签 token 拿到真实用户数据。
func TestAuthenticationRejectsTokenSignedWithOldHardcodedSecret(t *testing.T) {
	const configured = "configured-jwt-secret"
	oldHardcodedSecret := "12345678"

	r, reached := newRouter(configured)

	req := httptest.NewRequest(http.MethodGet, "/pro/v1/UserInfo", nil)
	req.Header.Set("Authorization", "bearer "+signToken(t, oldHardcodedSecret))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.False(t, *reached, "旧硬编码密钥签发的 token 不得进入业务处理")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ③ 空密钥时一律 401（除白名单），且不得用空密钥验签。
func TestAuthenticationFailsClosedWhenSecretEmpty(t *testing.T) {
	r, reached := newRouter("")

	cases := []struct {
		name  string
		authz string
	}{
		{"no token", ""},
		{"token signed with old hardcoded secret", "bearer " + signToken(t, "12345678")},
		{"token signed with empty secret", "bearer " + signToken(t, "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			*reached = false
			req := httptest.NewRequest(http.MethodGet, "/pro/v1/UserInfo", nil)
			if tc.authz != "" {
				req.Header.Set("Authorization", tc.authz)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			require.False(t, *reached, "空密钥必须 fail-closed")
			require.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

// ④ 白名单路径无需 token 也能过（即便密钥为空）。
func TestAuthenticationWhitelistBypassesToken(t *testing.T) {
	r, reached := newRouter("")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/pro/v1/Login", nil))

	require.True(t, *reached, "白名单路径应当放行")
	require.Equal(t, http.StatusOK, rec.Code)
}
