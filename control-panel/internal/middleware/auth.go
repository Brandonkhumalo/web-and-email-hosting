package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Claims holds the JWT payload for authenticated customers.
type Claims struct {
	CustomerID int64  `json:"customer_id"`
	Email      string `json:"email"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT for a customer.
func GenerateToken(customerID int64, email string, secret string, expireHours int) (string, time.Time, error) {
	expiresAt := time.Now().Add(time.Duration(expireHours) * time.Hour)

	claims := &Claims{
		CustomerID: customerID,
		Email:      email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "tishanyq-hosting",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}

	return signed, expiresAt, nil
}

// AuthRequired is a Gin middleware that validates the JWT in the
// Authorization header. Sets "customer_id" and "customer_email"
// in the Gin context for downstream handlers.
func AuthRequired(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from "Authorization: Bearer <token>"
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing Authorization header",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid Authorization header format (expected: Bearer <token>)",
			})
			return
		}

		tokenString := parts[1]

		// Parse and validate the JWT
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or expired token",
			})
			return
		}

		// Set customer info in context for handlers to use
		c.Set("customer_id", claims.CustomerID)
		c.Set("customer_email", claims.Email)

		c.Next()
	}
}

// GetCustomerID extracts the authenticated customer's ID from the Gin context.
func GetCustomerID(c *gin.Context) int64 {
	id, _ := c.Get("customer_id")
	return id.(int64)
}
