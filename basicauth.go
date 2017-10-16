package tokay

import (
	"encoding/base64"
)

// AuthUserKey is the cookie name for user credential in basic auth.
const AuthUserKey = "basicAuthUuser"

type authPair struct {
	Value string
	User  string
}

type authPairs []authPair

func (a authPairs) search(authValue string) (string, bool) {
	if authValue == "" {
		return "", false
	}
	for _, pair := range a {
		if pair.Value == authValue {
			return pair.User, true
		}
	}
	return "", false
}

// BasicAuth returns a Basic HTTP Authorization middleware.
// It takes even number of string arguments (username1, password1, username2, password2, etc...)
func BasicAuth(accounts ...string) Handler {
	pairs := processAccounts(accounts...)
	return func(c *Context) {
		user, found := pairs.search(c.GetHeader("Authorization"))
		if !found {
			c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
			c.AbortWithStatus(401)
			return
		}

		c.Set(AuthUserKey, user)
	}
}

func processAccounts(accounts ...string) authPairs {
	accLen := len(accounts)
	if accLen < 2 || accLen%2 != 0 {
		panic("The number of arguments must be even.")
	}
	pairs := make(authPairs, 0, accLen/2)
	for i := 0; i < accLen; i += 2 {
		user, password := accounts[i], accounts[i+1]
		if user == "" {
			panic("User can not be empty")
		}
		value := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
		pairs = append(pairs, authPair{
			Value: value,
			User:  user,
		})
	}
	return pairs
}
