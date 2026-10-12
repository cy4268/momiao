// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Adapted from official QuantumNous/new-api@f116414284162ad15d8925f7bca494c109b83e93;
// only package wiring and the persistence/HTTP interfaces are changed.
// Source: common/crypto.go.
package identity

import "golang.org/x/crypto/bcrypt"

func Password2Hash(password string) (string, error) {
	passwordBytes := []byte(password)
	hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	return string(hashedPassword), err
}

func ValidatePasswordAndHash(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
