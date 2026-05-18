package auth

import (
	"crypto/subtle"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/twitter/shared/httperr"
)

// Services behind Kong do not verify JWTs — Kong injects X-User-ID/Email/Username and
// this middleware reads those headers into Claims.
func HeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			httperr.AbortUnauthorized(c, "missing or invalid token")
			return
		}
		c.Set(string(ctxClaimsKey), &Claims{
			Sub:      userID,
			Email:    c.GetHeader("X-User-Email"),
			Username: c.GetHeader("X-User-Username"),
		})
		c.Next()
	}
}

// SetClaims is used by tests that bypass HeadersMiddleware.
func SetClaims(c *gin.Context, claims *Claims) {
	c.Set(string(ctxClaimsKey), claims)
}

func ClaimsFrom(c *gin.Context) *Claims {
	v, _ := c.Get(string(ctxClaimsKey))
	claims, _ := v.(*Claims)
	return claims
}

func ServiceTokenMiddleware(serviceToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := bearerToken(c)
		if serviceToken == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(serviceToken)) != 1 {
			httperr.AbortUnauthorized(c, "missing or invalid service token")
			return
		}
		c.Next()
	}
}

// Panics outside an authenticated route; gin.Recovery() turns it into a 500 — signals a wiring bug.
func MustClaimsFrom(c *gin.Context) *Claims {
	claims := ClaimsFrom(c)
	if claims == nil {
		panic("MustClaimsFrom called outside an authenticated route")
	}
	return claims
}

func bearerToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if tok, err := c.Cookie("token"); err == nil {
		return tok
	}
	return ""
}
