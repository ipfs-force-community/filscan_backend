package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt"
	logging "github.com/ipfs/go-log/v2"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/bearer"
	"golang.org/x/crypto/bcrypt"
)

var (
	logger = logging.Logger("auth")
)

const (
	Secret              = "cocacola"
	BearerContextKey    = "bearer"
	UserResourcePermKey = "ResourcePerm"
)

// ParseBearer 获取JWTToken里的用户信息，得到用户ID和名称
// 这里需要确保的是，API需要由auth拦截器保存用户信息，如果没有设置auth拦截器，这里返回的结果将是nil。
// 同时需要设置 ContextWrapper 来设置context.Context 否则是个空指针
func ParseBearer(ctx context.Context) *bearer.Bearer {
	user := ctx.Value(BearerContextKey)
	if user == nil {
		return nil
	}
	return user.(*bearer.Bearer)
}

// EncodePassword encode the password.
func EncodePassword(pwd string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(hash)
}

// ComparePasswords 验证用户登录时数据的密码和数据库中记录的密码的哈希比较
func ComparePasswords(plainPass, pwdHash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(pwdHash), []byte(plainPass))
	if err != nil {
		return false
	}
	return true
}

func GenRandKey() (string, error) {
	var data [32]byte
	_, err := rand.Read(data[:])
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

// GenJWT 生成JWT Token
// secret 服务端和客户端约定好的密钥
// user   用户信息
// Output： JWT Token
func GenJWT(secret string, user *bearer.Bearer) (string, time.Time, error) {
	now := time.Now()
	expireAt := now.Add(864000 * time.Second)
	id, err := GenRandKey()
	if err != nil {
		logger.Errorf("get jwt token rand failed: %w", err)
		return "", time.Time{}, err
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, bearer.JWTClaims{
		StandardClaims: jwt.StandardClaims{
			Id:        id,              // 编号
			ExpiresAt: expireAt.Unix(), // 过期时间，从当前时间开始后10天
			Issuer:    "FILSCAN",       //签发人
			Subject:   "API Auth",      // 主题
			NotBefore: now.Unix(),      // 生效时间
			IssuedAt:  now.Unix(),      // 签发时间
		},
		UserID: uint(user.Id),
		Mail:   user.Mail,
	})

	jwtToken, err := token.SignedString([]byte(secret))
	if err != nil {
		logger.Errorf("get jwt token failed: %w", err)
		return "", time.Time{}, err
	}

	return jwtToken, expireAt, nil
}
