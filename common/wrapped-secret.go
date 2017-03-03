package common

import (
	"time"

	"github.com/pkg/errors"
)

type WrappedSecretId struct {
	SecretID     string    `json:"token"`
	CreationTime time.Time `json:"creationTime"`
	TTL          int       `json:"ttl"`
	VaultAddr    string    `json:"vaultAddr"`
	VaultCAs     []byte    `json:"vaultCAs"`
}

func (w WrappedSecretId) Validate() error {

	if w.SecretID == "" {
		return errors.New("Token is empty.")
	}

	if w.CreationTime.IsZero() {
		return errors.New("CreationTime is invalid.")
	}

	if w.VaultAddr == "" {
		return errors.New("Vault server address is not set.")
	}

	if w.CreationTime.Add(time.Duration(w.TTL) * time.Second).Before(time.Now()) {
		return errors.New("Token is expired.")
	}

	return nil
}
