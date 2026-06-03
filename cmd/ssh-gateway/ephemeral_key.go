package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"

	"golang.org/x/crypto/ssh"
)

func generateEphemeralSSHKey() (ssh.Signer, string, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, "", err
	}
	return signer, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), nil
}
