package account

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type TokenMetadata struct {
	UID      string
	Label    string
	Email    string
	Expires  int64
}

func MetadataFromToken(token string) TokenMetadata {
	meta := TokenMetadata{}
	parts := strings.Split(token, ".")
	if len(parts) >= 2 {
		if payload, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
			var raw map[string]json.RawMessage
			if json.Unmarshal(payload, &raw) == nil {
				for _, key := range []string{"user_id", "userId", "uid", "sub"} {
					if value := claimString(raw[key]); value != "" {
						meta.UID = value
						break
					}
				}
				for _, key := range []string{"email"} {
					if value := claimString(raw[key]); value != "" {
						meta.Email = value
						break
					}
				}
				for _, key := range []string{"nickname", "name", "preferred_username", "email"} {
					if value := claimString(raw[key]); value != "" {
						meta.Label = value
						break
					}
				}
				if exp := claimInt64(raw["exp"]); exp > 0 {
					meta.Expires = exp
				}
			}
		}
	}
	if meta.UID == "" {
		sum := sha256.Sum256([]byte(token))
		meta.UID = "tok-" + hex.EncodeToString(sum[:4])
	}
	if meta.Label == "" {
		meta.Label = meta.Email
	}
	if meta.Label == "" {
		meta.Label = meta.UID
	}
	return meta
}

func StableAccountID(token, teamID string) string {
	meta := MetadataFromToken(token)
	return StableAccountIDForIdentity(meta.UID, teamID)
}

func StableAccountIDForIdentity(identity, teamID string) string {
	identity = strings.TrimSpace(identity)
	teamID = strings.TrimSpace(teamID)
	if identity == "" {
		identity = "unknown"
	}
	sum := sha256.Sum256([]byte(identity + "|" + teamID))
	return "vd-" + hex.EncodeToString(sum[:6])
}

func NewDeviceID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		sum := sha256.Sum256([]byte("verdent-device-fallback"))
		return hex.EncodeToString(sum[:16])
	}
	return hex.EncodeToString(buf)
}

func claimString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}

func claimInt64(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		value, _ := number.Int64()
		return value
	}
	return 0
}
