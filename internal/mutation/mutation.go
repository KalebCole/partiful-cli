package mutation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"time"
)

const (
	recordVersion = 1
	tokenBytes    = 32
)

var (
	ErrStale       = errors.New("mutation plan is stale")
	ErrUnavailable = errors.New("mutation plan storage is unavailable")
)

type FileSystem interface {
	ReadFile(string) ([]byte, error)
	WriteFileAtomic(string, []byte) error
	WithLock(string, func()) error
}

type Binding struct {
	Command            string          `json:"command"`
	Operation          string          `json:"operation"`
	AccountFingerprint string          `json:"accountFingerprint"`
	Input              json.RawMessage `json:"input"`
	Request            json.RawMessage `json:"request"`
	Preconditions      json.RawMessage `json:"preconditions"`
}

type Record struct {
	Binding   Binding   `json:"binding"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Authority struct {
	Files  FileSystem
	Path   string
	Now    func() time.Time
	Random io.Reader
}

type document struct {
	Version int               `json:"version"`
	Records map[string]Record `json:"records"`
}

func (authority Authority) Create(binding Binding) (string, error) {
	if !authority.configured() || authority.Random == nil || !validBinding(binding) {
		return "", ErrUnavailable
	}
	var (
		token        string
		operationErr error
	)
	if err := authority.Files.WithLock(authority.Path, func() {
		stored, err := authority.load()
		if err != nil {
			operationErr = err
			return
		}
		now := authority.Now().UTC()
		for candidate, record := range stored.Records {
			if !record.ExpiresAt.After(now) {
				delete(stored.Records, candidate)
			}
		}
		random := make([]byte, tokenBytes)
		if _, err := io.ReadFull(authority.Random, random); err != nil {
			operationErr = ErrUnavailable
			return
		}
		token = base64.RawURLEncoding.EncodeToString(random)
		if _, exists := stored.Records[token]; exists {
			operationErr = ErrUnavailable
			return
		}
		stored.Records[token] = Record{
			Binding:   cloneBinding(binding),
			CreatedAt: now,
			ExpiresAt: now.Add(5 * time.Minute),
		}
		operationErr = authority.save(stored)
	}); err != nil {
		return "", ErrUnavailable
	}
	if operationErr != nil {
		return "", operationErr
	}
	return token, nil
}

func (authority Authority) Inspect(
	token string,
	command string,
	operation string,
	accountFingerprint string,
	input json.RawMessage,
) (Record, error) {
	if !authority.configured() || token == "" {
		return Record{}, ErrStale
	}
	var (
		record       Record
		operationErr error
	)
	if err := authority.Files.WithLock(authority.Path, func() {
		stored, err := authority.load()
		if err != nil {
			operationErr = err
			return
		}
		candidate, ok := stored.Records[token]
		if !ok ||
			!candidate.ExpiresAt.After(authority.Now().UTC()) ||
			candidate.Binding.Command != command ||
			candidate.Binding.Operation != operation ||
			candidate.Binding.AccountFingerprint != accountFingerprint ||
			!bytes.Equal(candidate.Binding.Input, input) {
			operationErr = ErrStale
			return
		}
		record = cloneRecord(candidate)
	}); err != nil {
		return Record{}, ErrUnavailable
	}
	if operationErr != nil {
		return Record{}, operationErr
	}
	return record, nil
}

func (authority Authority) Consume(
	token string,
	binding Binding,
) error {
	if !authority.configured() || token == "" || !validBinding(binding) {
		return ErrStale
	}
	var operationErr error
	if err := authority.Files.WithLock(authority.Path, func() {
		stored, err := authority.load()
		if err != nil {
			operationErr = err
			return
		}
		record, ok := stored.Records[token]
		if !ok ||
			!record.ExpiresAt.After(authority.Now().UTC()) ||
			!equalBinding(record.Binding, binding) {
			operationErr = ErrStale
			return
		}
		delete(stored.Records, token)
		operationErr = authority.save(stored)
	}); err != nil {
		return ErrUnavailable
	}
	return operationErr
}

func (authority Authority) configured() bool {
	return authority.Files != nil && authority.Path != "" && authority.Now != nil
}

func (authority Authority) load() (document, error) {
	raw, err := authority.Files.ReadFile(authority.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return document{Version: recordVersion, Records: make(map[string]Record)}, nil
	}
	if err != nil {
		return document{}, ErrUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var stored document
	if decoder.Decode(&stored) != nil ||
		stored.Version != recordVersion ||
		stored.Records == nil {
		return document{}, ErrUnavailable
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return document{}, ErrUnavailable
	}
	for token, record := range stored.Records {
		decoded, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil ||
			len(decoded) != tokenBytes ||
			record.CreatedAt.IsZero() ||
			record.ExpiresAt.IsZero() ||
			!validBinding(record.Binding) {
			return document{}, ErrUnavailable
		}
	}
	return stored, nil
}

func (authority Authority) save(stored document) error {
	raw, err := json.Marshal(stored)
	if err != nil || authority.Files.WriteFileAtomic(authority.Path, raw) != nil {
		return ErrUnavailable
	}
	return nil
}

func validBinding(binding Binding) bool {
	return binding.Command != "" &&
		binding.Operation != "" &&
		binding.AccountFingerprint != "" &&
		json.Valid(binding.Input) &&
		json.Valid(binding.Request) &&
		json.Valid(binding.Preconditions)
}

func equalBinding(left, right Binding) bool {
	return left.Command == right.Command &&
		left.Operation == right.Operation &&
		left.AccountFingerprint == right.AccountFingerprint &&
		bytes.Equal(left.Input, right.Input) &&
		bytes.Equal(left.Request, right.Request) &&
		bytes.Equal(left.Preconditions, right.Preconditions)
}

func cloneBinding(binding Binding) Binding {
	binding.Input = bytes.Clone(binding.Input)
	binding.Request = bytes.Clone(binding.Request)
	binding.Preconditions = bytes.Clone(binding.Preconditions)
	return binding
}

func cloneRecord(record Record) Record {
	record.Binding = cloneBinding(record.Binding)
	return record
}
