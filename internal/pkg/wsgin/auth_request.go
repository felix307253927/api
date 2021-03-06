package wsgin

import (
	"strings"

	"github.com/gin-gonic/gin"

	"welfare-sign/internal/pkg/jwt"
)

// AuthRequest 尝试解析用户token，如果未传入不会报错
type AuthRequest struct {
	BaseRequest

	TokenParames *jwt.TokenParames `json:"-"`
}

// Extract .
func (r *AuthRequest) Extract(c *gin.Context) (code APICode, err error) {
	return r.DefaultExtract(r, c)
}

// DefaultExtract default extract
func (r *AuthRequest) DefaultExtract(data interface{}, c *gin.Context) (code APICode, err error) {
	return r.ExtractWithBindFunc(data, c, c.ShouldBind)
}

// ExtractWithBindFunc default ExtractWithBindFunc
func (r *AuthRequest) ExtractWithBindFunc(data interface{}, c *gin.Context, bindFunc BindFunc) (code APICode, err error) {
	code, err = r.BaseRequest.ExtractWithBindFunc(data, c, bindFunc)
	if err != nil {
		return
	}
	params, code, err := authFunc(c)
	if err != nil {
		return
	}
	r.TokenParames = params
	return
}

func authFunc(c *gin.Context) (*jwt.TokenParames, APICode, error) {
	token := strings.TrimSpace(c.Query("access_token"))
	if token == "" {
		token = strings.TrimSpace(c.GetHeader("Authorization"))
	}
	if token == "" {
		return nil, APICodeSuccess, nil
	}
	tokenParams, err := jwt.ParseToken(token)
	if err != nil {
		return nil, APICodeSuccess, nil
	}
	return tokenParams, APICodeSuccess, nil
}
