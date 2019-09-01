//copied from https://gist.github.com/miguelmota/3ea9286bd1d3c2a985b67cac4ba2130a
//adapted for fixed keysize

package rsautil

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"fmt"
)

// InternalKeySize is the size of the actual key
const InternalKeySize = 128

// KeySize is the size of the serialized key
const KeySize = 162

// SignatureSize is number of bytes 1 signature takes
const SignatureSize = 128

// GenerateKeyPair generates a new key pair
func GenerateKeyPair() (*rsa.PrivateKey, *rsa.PublicKey) {
	privkey, err := rsa.GenerateKey(rand.Reader, InternalKeySize*8)
	if err != nil {
		fmt.Println(err)

	}
	//fmt.Printf("%d", privkey.D)
	return privkey, &privkey.PublicKey
}

// PublicKeyToBytes public key to bytes
func PublicKeyToBytes(pub *rsa.PublicKey) [KeySize]byte {

	pubASN1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		fmt.Println(err)
	}

	var output [KeySize]byte
	copy(output[0:KeySize], pubASN1[0:KeySize])
	return output
}

// BytesToPublicKey bytes to public key
func BytesToPublicKey(public [KeySize]byte) *rsa.PublicKey {
	ifc, err := x509.ParsePKIXPublicKey(public[:])
	if err != nil {
		fmt.Println(err)
	}

	key, ok := ifc.(*rsa.PublicKey)
	if !ok {
		fmt.Println("not ok")
	}
	return key
}

// EncryptWithPublicKey encrypts data with public key
func EncryptWithPublicKey(msg []byte, pub *rsa.PublicKey) []byte {
	hash := sha512.New()
	ciphertext, err := rsa.EncryptOAEP(hash, rand.Reader, pub, msg, nil)
	if err != nil {
		fmt.Println(err)
	}
	return ciphertext
}

// DecryptWithPrivateKey decrypts data with private key
func DecryptWithPrivateKey(ciphertext []byte, priv *rsa.PrivateKey) []byte {
	hash := sha512.New()
	plaintext, err := rsa.DecryptOAEP(hash, rand.Reader, priv, ciphertext, nil)
	if err != nil {
		fmt.Println(err)
	}
	return plaintext
}

// SignWithPrivateKey signs hashed byte array
func SignWithPrivateKey(hashed [32]byte, priv *rsa.PrivateKey) [SignatureSize]byte {
	hash := crypto.SHA256
	ciphertext, err := rsa.SignPSS(rand.Reader, priv, hash, hashed[:], nil)
	if err != nil {
		fmt.Println(err)
	}

	//fmt.Printf("size of signature is %d\n", len(ciphertext))
	var a [SignatureSize]byte
	copy(a[0:SignatureSize], ciphertext[0:SignatureSize])
	return a
}

// VerifyWithPublicKey verifies SignWithPrivateKey
func VerifyWithPublicKey(hashed [32]byte, sig [SignatureSize]byte, pub *rsa.PublicKey) bool {
	hash := crypto.SHA256
	err := rsa.VerifyPSS(pub, hash, hashed[:], sig[:], nil)
	return err == nil
}
