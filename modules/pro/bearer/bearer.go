package bearer

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt"
	"github.com/gozelle/gin"
	"github.com/gozelle/mix"
)

type Bearer struct {
	Id   int64
	Mail string
}

type ck string

const contextKey ck = "bearer"
const secret = "12345678"

var whiteList = map[string]struct{}{
	"/pro/v1/Login":                  {},
	"/pro/v1/SendVerificationCode":   {},
	"/pro/v1/MailExists":             {},
	"/pro/v1/ResetPasswordByCode":    {},
	"/pro/v1/ValidInvite":            {},
	"/pro/v1/CapitalAddrTransaction": {},
	"/pro/v1/CapitalAddrInfo":        {},
	"/pro/v1/EvaluateAddr":           {},
}

func UseBearer(ctx context.Context) *Bearer {
	b := ctx.Value(contextKey)
	if b == nil {
		panic("cant't fetch bearer by context key")
	}
	v, ok := b.(*Bearer)
	if !ok {
		panic("assert *Bearer failed")
	}
	return v
}

func Authentication() gin.HandlerFunc {
	return func(c *gin.Context) {

		if _, ok := whiteList[c.Request.URL.Path]; ok {
			return
		}

		token := c.GetHeader("Authorization")
		if token == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		b := new(Bearer)
		ok, err := VerifyJWT(secret, strings.TrimPrefix(token, "bearer "), b)
		if err != nil {
			_ = c.AbortWithError(http.StatusUnauthorized, err)
			return
		}
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), contextKey, b))
		mix.SetBearer(c, fmt.Sprintf("%d:%s", b.Id, b.Mail))
	}
}

type JWTClaims struct {
	jwt.StandardClaims
	UserID uint
	Mail   string
}

// VerifyJWT 验证JWT Token
// 一般如果验证在客户端做验证就可以了，不需要在服务端通过API验证。
// 这个方法是工具方法，提供本地测试使用。
// secret 服务端和客户端约定的密钥
// token  JWT Token
// out  传到外部的用户
// Output: true || false, true 表示验证通过，反之不通过
func VerifyJWT(secret, token string, out *Bearer) (bool, error) {
	jwtToken, err := jwt.ParseWithClaims(
		token,
		&JWTClaims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}

			return []byte(secret), nil

		})

	if err != nil {
		return false, err
	}

	if claim, ok := jwtToken.Claims.(*JWTClaims); ok && jwtToken.Valid {
		if out == nil {
			out = new(Bearer)
		}
		out.Id = int64(claim.UserID)
		out.Mail = claim.Mail
		return true, nil
	}

	return false, nil
}
