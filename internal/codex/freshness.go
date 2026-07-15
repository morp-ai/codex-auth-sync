package codex

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func AuthFreshness(snapshot []byte) int64 {
	if len(snapshot) == 0 {
		return 0
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(snapshot, &root); err != nil {
		return 0
	}
	if raw, ok := root["last_refresh"]; ok {
		var value string
		if json.Unmarshal(raw, &value) == nil {
			if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value)); err == nil {
				return parsed.Unix()
			}
		}
	}
	var tokens map[string]json.RawMessage
	if raw, ok := root["tokens"]; ok {
		_ = json.Unmarshal(raw, &tokens)
	}
	for _, source := range []map[string]json.RawMessage{tokens, root} {
		for _, name := range []string{"access_token", "id_token"} {
			if raw, ok := source[name]; ok {
				var token string
				if json.Unmarshal(raw, &token) == nil {
					if expiry := jwtExpiry(token); expiry > 0 {
						return expiry
					}
				}
			}
		}
	}
	return 0
}

func ShouldReplaceAuth(local, remote []byte, remoteFreshness int64) bool {
	if len(remote) == 0 || string(local) == string(remote) {
		return false
	}
	if len(local) == 0 {
		return true
	}
	localFreshness := AuthFreshness(local)
	if localFreshness > 0 {
		if remoteFreshness == 0 {
			return false
		}
		return remoteFreshness >= localFreshness
	}
	if remoteFreshness > 0 {
		return true
	}
	return true
}

func jwtExpiry(token string) int64 {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return 0
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0
	}
	var claims map[string]json.RawMessage
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0
	}
	raw, ok := claims["exp"]
	if !ok {
		return 0
	}
	var integer int64
	if json.Unmarshal(raw, &integer) == nil {
		return integer
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		integer, _ = strconv.ParseInt(number.String(), 10, 64)
		return integer
	}
	return 0
}
