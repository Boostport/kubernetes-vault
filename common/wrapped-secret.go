package common

import (
	"github.com/pkg/errors"
	"time"
)

type WrappedSecretId struct {
	Token        string    `json:"token"`
	CreationTime time.Time `json:"creationTime"`
	TTL          int       `json:"ttl"`
	VaultAddr    string    `json:"vaultAddr"`
}

func (w WrappedSecretId) Validate() error {

	if w.Token == "" {
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
