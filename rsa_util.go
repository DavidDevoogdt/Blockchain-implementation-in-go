//copied from https://gist.github.com/miguelmota/3ea9286bd1d3c2a985b67cac4ba2130a
//adapted for fixed keysize

package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// KeySize is used for all user keys
const KeySize = 16

// GenerateKeyPair generates a new key pair
func GenerateKeyPair() (*rsa.PrivateKey, *rsa.PublicKey) {
	privkey, err := rsa.GenerateKey(rand.Reader, KeySize)
	if err != nil {
		fmt.Println(err)

	}
	return privkey, &privkey.PublicKey
}

// PrivateKeyToBytes private key to bytes
func PrivateKeyToBytes(priv *rsa.PrivateKey) [KeySize]byte {
	privBytes := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(priv),
		},
	)

	var output [KeySize]byte
	copy(output[0:KeySize], privBytes[0:KeySize])
	return output
}

// PublicKeyToBytes public key to bytes
func PublicKeyToBytes(pub *rsa.PublicKey) [KeySize]byte {
	pubASN1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		fmt.Println(err)
	}

	pubBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pubASN1,
	})
	var output [KeySize]byte
	copy(output[0:KeySize], pubBytes[0:KeySize])
	return output
}

// BytesToPrivateKey bytes to private key
func BytesToPrivateKey(private [KeySize]byte) *rsa.PrivateKey {

	priv := make([]byte, KeySize)
	copy(priv[0:KeySize], private[0:KeySize])

	block, _ := pem.Decode(priv)
	enc := x509.IsEncryptedPEMBlock(block)
	b := block.Bytes
	var err error
	if enc {
		fmt.Println("is encrypted pem block")
		b, err = x509.DecryptPEMBlock(block, nil)
		if err != nil {
			fmt.Println(err)
		}
	}
	key, err := x509.ParsePKCS1PrivateKey(b)
	if err != nil {
		fmt.Println(err)
	}
	return key
}

// BytesToPublicKey bytes to public key
func BytesToPublicKey(public [KeySize]byte) *rsa.PublicKey {
	pub := make([]byte, KeySize)
	copy(pub[0:KeySize], public[0:KeySize])

	block, _ := pem.Decode(pub)
	enc := x509.IsEncryptedPEMBlock(block)
	b := block.Bytes
	var err error
	if enc {
		fmt.Println("is encrypted pem block")
		b, err = x509.DecryptPEMBlock(block, nil)
		if err != nil {
			fmt.Println(err)
		}
	}
	ifc, err := x509.ParsePKIXPublicKey(b)
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
func SignWithPrivateKey(hashed [32]byte, priv *rsa.PrivateKey) [32]byte {
	hash := crypto.SHA256
	ciphertext, err := rsa.SignPSS(rand.Reader, priv, hash, hashed[:], nil)
	if err != nil {
		fmt.Println(err)
	}
	var a [32]byte
	copy(a[0:32], ciphertext[0:32])
	return a
}

// VerifyWithPublicKey verifies SignWithPrivateKey
func VerifyWithPublicKey(hashed [32]byte, sig [32]byte, pub *rsa.PublicKey) bool {
	hash := crypto.SHA256
	err := rsa.VerifyPSS(pub, hash, hashed[:], sig[:], nil)
	return err == nil
}
