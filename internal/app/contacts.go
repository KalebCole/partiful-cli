package app

import (
	"encoding/base64"
	"errors"
	"io"
)

func randomDeviceID(random io.Reader) (string, error) {
	if random == nil {
		return "", errors.New("device identity random source is unavailable")
	}
	value := make([]byte, 16)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", errors.New("device identity random source is unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
