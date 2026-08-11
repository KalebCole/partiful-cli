package auth

import "encoding/json"

func saveCredentials(files FileSystem, path string, credentials credentialRecord) error {
	if files == nil || path == "" {
		return ErrNotConfigured
	}
	var saveError error
	if err := files.WithLock(path, func() {
		saveError = saveCredentialsUnlocked(files, path, credentials)
	}); err != nil {
		return ErrUnavailable
	}
	return saveError
}

func saveCredentialsUnlocked(files FileSystem, path string, credentials credentialRecord) error {
	document, err := json.Marshal(credentials)
	if err != nil {
		return ErrUnavailable
	}
	return files.WriteFileAtomic(path, document)
}
