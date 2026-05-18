package auth

type contextKey string

const ctxClaimsKey contextKey = "claims"

type Claims struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Username string `json:"username"`
}
