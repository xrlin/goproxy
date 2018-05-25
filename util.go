package proxy

import (
	"encoding/base64"
	"strings"
)

// ParseBasicAuth parses an HTTP Basic Authentication string.
// "Basic dXNlcjpwYXNzd29yZA==" returns ("user", "password", true).
func ParseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	if !strings.HasPrefix(auth, prefix) {
		return
	}
	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return
	}
	cs := string(c)
	s := strings.IndexByte(cs, ':')
	if s < 0 {
		return
	}
	return cs[:s], cs[s+1:], true
}
