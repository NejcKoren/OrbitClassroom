package proto

import (
	"bytes"
	"crypto/ed25519"
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestKeyRoundTrip(t *testing.T) {
	k := NewTeacherKey()
	pub := PublicKeyString(k)
	if !strings.HasPrefix(pub, "OPK1-") {
		t.Fatal(pub)
	}
	p, err := ParsePublicKey(strings.ToLower(strings.ReplaceAll(pub, "-", " ")))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(p, k.Public().(ed25519.PublicKey)) {
		t.Fatal("pub mismatch")
	}
	s, err := ParseSecretKey(SecretKeyString(k))
	if err != nil || !bytes.Equal(s, k) {
		t.Fatal("secret mismatch", err)
	}
	// a typo must be detected by the checksum
	bad := []byte(pub)
	if bad[10] == 'A' {
		bad[10] = 'B'
	} else {
		bad[10] = 'A'
	}
	if _, err := ParsePublicKey(string(bad)); err == nil {
		t.Fatal("typo not detected")
	}
	if _, err := ParsePublicKey(SecretKeyString(k)); err == nil {
		t.Fatal("secret accepted as public key")
	}
}

func TestAuthSignature(t *testing.T) {
	k := NewTeacherKey()
	pub, _ := ParsePublicKey(PublicKeyString(k))
	sig := SignAuth(k, "nonce1", "fp1")
	if !VerifyAuth(pub, "nonce1", "fp1", sig) {
		t.Fatal("valid signature rejected")
	}
	if VerifyAuth(pub, "nonce1", "fp-of-mitm", sig) {
		t.Fatal("signature accepted for other certificate (relay attack)")
	}
	if VerifyAuth(pub, "nonce2", "fp1", sig) {
		t.Fatal("replayed signature accepted")
	}
}

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func TestStreamDeltaRoundTrip(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 640, 400))
	fill(img, img.Rect, color.RGBA{20, 40, 200, 255})
	enc := &Encoder{Quality: 90}
	var buf bytes.Buffer
	data, key, err := enc.Encode(img, false)
	if err != nil || !key || data == nil {
		t.Fatal("first frame must be a key frame", err)
	}
	buf.Write(data)
	// nothing changed -> no frame
	if d, _, _ := enc.Encode(img, false); d != nil {
		t.Fatal("unchanged frame produced output")
	}
	// small change -> small delta frame
	fill(img, image.Rect(300, 200, 340, 230), color.RGBA{250, 160, 30, 255})
	d2, key2, _ := enc.Encode(img, false)
	if key2 || d2 == nil || len(d2) > len(data) {
		t.Fatalf("expected small delta frame, key=%v len=%d (key frame %d)", key2, len(d2), len(data))
	}
	buf.Write(d2)
	c := &Canvas{}
	for i := 0; i < 2; i++ {
		f, err := ReadFrame(&buf)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Apply(f); err != nil {
			t.Fatal(err)
		}
	}
	out := c.RGBA()
	p := out.RGBAAt(320, 215)
	if p.R < 200 || p.B > 90 {
		t.Fatalf("changed region not applied: %v", p)
	}
	q := out.RGBAAt(10, 10)
	if q.B < 170 {
		t.Fatalf("background wrong: %v", q)
	}
}

func TestDeltaBeforeKeyRejected(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	enc := &Encoder{}
	enc.Encode(img, false)
	fill(img, image.Rect(0, 0, 10, 10), color.RGBA{255, 0, 0, 255})
	d, _, _ := enc.Encode(img, false)
	f, err := ReadFrame(bytes.NewReader(d))
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Canvas{}).Apply(f); err != ErrNeedKey {
		t.Fatalf("expected ErrNeedKey, got %v", err)
	}
}

func TestMagicPacket(t *testing.T) {
	p, err := MagicPacket("01:23:45:67:89:ab")
	if err != nil || len(p) != 102 || p[0] != 0xFF || p[6] != 0x01 || p[101] != 0xab {
		t.Fatal("bad magic packet", err)
	}
}
