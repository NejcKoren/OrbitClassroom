// keygen prints a new teacher key pair (for administrators who prepare
// agent configurations without Orbit Master).
//
//	go run ./tools/keygen
package main

import (
	"crypto/ed25519"
	"fmt"
	"orbit/internal/proto"
)

func main() {
	k := proto.NewTeacherKey()
	fmt.Println("public :", proto.PublicKeyString(k))
	fmt.Println("secret :", proto.SecretKeyString(k))
	fmt.Println("fingerprint:", proto.Fingerprint(k.Public().(ed25519.PublicKey)))
}
