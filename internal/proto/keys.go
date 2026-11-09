package proto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// Key strings look like  OPK1-ABCD-EFGH-....  (public key, safe to share)
// or                     OTK1-ABCD-EFGH-....  (teacher secret key, keep private)
// They contain 32 key bytes + a 3 byte checksum, base32 encoded, so typos are detected.
const (
	PublicKeyPrefix = "OPK1"
	SecretKeyPrefix = "OTK1"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func keyChecksum(prefix string, raw []byte) []byte {
	h := sha256.Sum256(append([]byte(prefix+":"), raw...))
	return h[:3]
}

// EncodeKey turns 32 raw bytes into a human friendly key string.
func EncodeKey(prefix string, raw []byte) string {
	data := append(append([]byte{}, raw...), keyChecksum(prefix, raw)...)
	s := b32.EncodeToString(data)
	var sb strings.Builder
	sb.WriteString(prefix)
	for i := 0; i < len(s); i += 4 {
		sb.WriteByte('-')
		sb.WriteString(s[i:min(i+4, len(s))])
	}
	return sb.String()
}

// DecodeKey parses a key string produced by EncodeKey. It is forgiving about
// case, spaces, dashes and the look-alike characters 0/O, 1/I and 8/B.
func DecodeKey(prefix, s string) ([]byte, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.NewReplacer("-", "", " ", "", "\n", "", "\r", "", "\t", "").Replace(s)
	if !strings.HasPrefix(s, prefix) {
		return nil, errors.New("wrong key type: expected a key starting with " + prefix)
	}
	s = strings.NewReplacer("0", "O", "1", "I", "8", "B").Replace(s[len(prefix):])
	data, err := b32.DecodeString(s)
	if err != nil || len(data) != 35 {
		return nil, errors.New("invalid key format")
	}
	raw := data[:32]
	if !bytes.Equal(keyChecksum(prefix, raw), data[32:]) {
		return nil, errors.New("key checksum mismatch - please check for typos")
	}
	return raw, nil
}

// ParsePublicKey decodes an OPK1 key string.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	raw, err := DecodeKey(PublicKeyPrefix, s)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(raw), nil
}

// ParseSecretKey decodes an OTK1 key string into a private key.
func ParseSecretKey(s string) (ed25519.PrivateKey, error) {
	raw, err := DecodeKey(SecretKeyPrefix, s)
	if err != nil {
		return nil, err
	}
	return ed25519.NewKeyFromSeed(raw), nil
}

// NewTeacherKey creates a fresh Ed25519 key pair.
func NewTeacherKey() ed25519.PrivateKey {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return priv
}

// PublicKeyString returns the shareable OPK1 string for a private key.
func PublicKeyString(priv ed25519.PrivateKey) string {
	return EncodeKey(PublicKeyPrefix, priv.Public().(ed25519.PublicKey))
}

// SecretKeyString returns the OTK1 backup string of a private key.
func SecretKeyString(priv ed25519.PrivateKey) string {
	return EncodeKey(SecretKeyPrefix, priv.Seed())
}

// Fingerprint is a short, human comparable id of a public key (e.g. 3F2A-91BC-0D4E).
func Fingerprint(pub []byte) string {
	h := sha256.Sum256(pub)
	x := strings.ToUpper(hex.EncodeToString(h[:6]))
	return x[0:4] + "-" + x[4:8] + "-" + x[8:12]
}

// CertFingerprint is the SHA-256 of a DER certificate (hex).
func CertFingerprint(der []byte) string {
	h := sha256.Sum256(der)
	return hex.EncodeToString(h[:])
}

// AuthPayload is what the master signs. It binds the signature to the agent's
// nonce (no replay) and to the TLS certificate the master actually sees
// (no man-in-the-middle relay).
func AuthPayload(nonce, certFP string) []byte {
	return []byte("orbit-auth-v1|" + nonce + "|" + certFP)
}

// SignAuth creates the base64 signature for an AuthRequest.
func SignAuth(priv ed25519.PrivateKey, nonce, certFP string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, AuthPayload(nonce, certFP)))
}

// VerifyAuth checks a signature created by SignAuth.
func VerifyAuth(pub ed25519.PublicKey, nonce, certFP, sig string) bool {
	raw, err := base64.StdEncoding.DecodeString(sig)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pub, AuthPayload(nonce, certFP), raw)
}

// RandomToken returns n random bytes, base64url encoded.
func RandomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// RandomID returns a short random hex id.
func RandomID() string {
	b := make([]byte, 5)
	rand.Read(b)
	return hex.EncodeToString(b)
}
