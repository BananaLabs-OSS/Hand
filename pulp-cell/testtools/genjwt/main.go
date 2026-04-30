// Tiny helper: prints a JWT signed with the same secret + claim shape
// Hand's JWTAuth middleware expects. Meant for manual curl testing,
// not part of the cell build.
//
//	go run ./testtools/genjwt -secret dev-jwt-secret-change-me -account 11111111-2222-3333-4444-555555555555
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	secret := flag.String("secret", "dev-jwt-secret-change-me", "HMAC signing key")
	accountID := flag.String("account", "", "account UUID to embed in claims (required)")
	sessionID := flag.String("session", "00000000-0000-0000-0000-000000000001", "session UUID")
	ttl := flag.Duration("ttl", time.Hour, "token lifetime")
	flag.Parse()

	if *accountID == "" {
		fmt.Fprintln(os.Stderr, "-account is required")
		os.Exit(2)
	}

	claims := jwt.MapClaims{
		"account_id": *accountID,
		"session_id": *sessionID,
		"exp":        time.Now().Add(*ttl).Unix(),
		"iat":        time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(*secret))
	if err != nil {
		fmt.Fprintf(os.Stderr, "sign: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(signed)
}
