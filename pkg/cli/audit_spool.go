package cli

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/adrg/xdg"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/cli/internal/credentials"
	"github.com/obot-platform/obot/pkg/localagents"
)

const (
	defaultAuditSpoolMaxBytes     = 100 * 1024 * 1024
	defaultAuditSpoolLockTimeout  = time.Second
	defaultAuditSpoolDrainTimeout = 5 * time.Second
	defaultAuditSpoolStaleLockAge = 10 * time.Minute
	auditSpoolKeyBytes            = 32
	auditSpoolRecordVersion       = 1
)

var errAuditSpoolLockTimeout = errors.New("audit spool lock timeout")

type auditSubmitFunc func(context.Context, types.LocalAgentAuditLogIngest) error

type auditSpool interface {
	Append(context.Context, types.LocalAgentAuditLogIngest) error
	Drain(context.Context, auditSubmitFunc) error
}

type localAuditSpool struct {
	appURL        string
	path          string
	keyStore      credentials.Store
	maxEventBytes int
	maxSpoolBytes int64
	lockTimeout   time.Duration
}

type auditSpoolRecord struct {
	Version    int    `json:"v"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type auditSpoolLock struct {
	path string
	file *os.File
}

func newDefaultAuditSpool(appURL string) (*localAuditSpool, error) {
	path, err := defaultAuditSpoolPath(appURL)
	if err != nil {
		return nil, err
	}
	return &localAuditSpool{
		appURL:        appURL,
		path:          path,
		keyStore:      credentials.NewAuditSpoolKeyringStore(),
		maxEventBytes: localagents.MaxAuditEventBytes,
		maxSpoolBytes: defaultAuditSpoolMaxBytes,
		lockTimeout:   defaultAuditSpoolLockTimeout,
	}, nil
}

func defaultAuditSpoolPath(appURL string) (string, error) {
	sum := sha256.Sum256([]byte(appURL))
	name := hex.EncodeToString(sum[:]) + ".spool"
	return xdg.StateFile(filepath.Join("obot", "local-agent-audit", name))
}

func (s *localAuditSpool) Append(ctx context.Context, auditLog types.LocalAgentAuditLogIngest) error {
	if s == nil {
		return nil
	}
	payload, err := json.Marshal(auditLog)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}
	if len(payload) > s.eventLimit() {
		return fmt.Errorf("audit event exceeds spool event limit")
	}

	return s.withLock(ctx, func() error {
		key, err := s.encryptionKey()
		if err != nil {
			return err
		}
		record, err := s.encryptRecord(key, payload)
		if err != nil {
			return err
		}
		if int64(len(record)+1) > s.spoolLimit() {
			return fmt.Errorf("encrypted audit event exceeds spool limit")
		}

		lines, err := readAuditSpoolLines(s.path)
		if err != nil {
			return err
		}
		lines = append(lines, record)
		lines = trimAuditSpoolLines(lines, s.spoolLimit())
		return writeAuditSpoolLines(s.path, lines)
	})
}

func (s *localAuditSpool) Drain(ctx context.Context, submit auditSubmitFunc) error {
	if s == nil || submit == nil {
		return nil
	}

	return s.withLock(ctx, func() error {
		lines, err := readAuditSpoolLines(s.path)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			return nil
		}

		key, err := s.encryptionKey()
		if err != nil {
			return err
		}

		events := make([]types.LocalAgentAuditLogIngest, 0, len(lines))
		for _, line := range lines {
			payload, err := s.decryptRecord(key, line)
			if err != nil {
				return err
			}
			var event types.LocalAgentAuditLogIngest
			if err := json.Unmarshal(payload, &event); err != nil {
				return fmt.Errorf("decode spooled audit event: %w", err)
			}
			events = append(events, event)
		}

		for _, event := range events {
			if err := submit(ctx, event); err != nil {
				return err
			}
		}

		if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear audit spool: %w", err)
		}
		return nil
	})
}

func (s *localAuditSpool) withLock(ctx context.Context, fn func() error) error {
	lock, err := acquireAuditSpoolLock(ctx, s.path+".lock", s.lockTimeout)
	if err != nil {
		return err
	}
	defer lock.Close()

	return fn()
}

func acquireAuditSpoolLock(ctx context.Context, path string, timeout time.Duration) (*auditSpoolLock, error) {
	if timeout <= 0 {
		timeout = defaultAuditSpoolLockTimeout
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create audit spool directory: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err == nil {
			_, _ = file.WriteString(strconv.Itoa(os.Getpid()))
			return &auditSpoolLock{path: path, file: file}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("open audit spool lock: %w", err)
		}
		if auditSpoolLockIsStale(path) {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, errAuditSpoolLockTimeout
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (l *auditSpoolLock) Close() error {
	if l == nil {
		return nil
	}
	var err error
	if l.file != nil {
		err = l.file.Close()
	}
	if removeErr := os.Remove(l.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && err == nil {
		err = removeErr
	}
	return err
}

func auditSpoolLockIsStale(path string) bool {
	info, err := os.Stat(path)
	return err == nil && time.Since(info.ModTime()) > defaultAuditSpoolStaleLockAge
}

func (s *localAuditSpool) encryptionKey() ([]byte, error) {
	if s.keyStore == nil {
		return nil, fmt.Errorf("audit spool key store is not configured")
	}

	encoded, err := s.keyStore.Get(s.appURL)
	if err == nil {
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(key) != auditSpoolKeyBytes {
			return nil, fmt.Errorf("stored audit spool encryption key is invalid")
		}
		return key, nil
	}
	if !credentials.IsNotFound(err) {
		return nil, fmt.Errorf("read audit spool encryption key: %w", err)
	}

	key := make([]byte, auditSpoolKeyBytes)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate audit spool encryption key: %w", err)
	}
	if err := s.keyStore.Set(s.appURL, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("store audit spool encryption key: %w", err)
	}
	return key, nil
}

func (s *localAuditSpool) encryptRecord(key, payload []byte) ([]byte, error) {
	gcm, err := auditSpoolGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate audit spool nonce: %w", err)
	}

	record := auditSpoolRecord{
		Version:    auditSpoolRecordVersion,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, payload, []byte(s.appURL))),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal audit spool record: %w", err)
	}
	return data, nil
}

func (s *localAuditSpool) decryptRecord(key, line []byte) ([]byte, error) {
	var record auditSpoolRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return nil, fmt.Errorf("decode audit spool record: %w", err)
	}
	if record.Version != auditSpoolRecordVersion {
		return nil, fmt.Errorf("unsupported audit spool record version %d", record.Version)
	}

	nonce, err := base64.StdEncoding.DecodeString(record.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode audit spool nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(record.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode audit spool ciphertext: %w", err)
	}

	gcm, err := auditSpoolGCM(key)
	if err != nil {
		return nil, err
	}
	payload, err := gcm.Open(nil, nonce, ciphertext, []byte(s.appURL))
	if err != nil {
		return nil, fmt.Errorf("decrypt audit spool record: %w", err)
	}
	return payload, nil
}

func auditSpoolGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create audit spool cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create audit spool gcm: %w", err)
	}
	return gcm, nil
}

func readAuditSpoolLines(path string) ([][]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read audit spool: %w", err)
	}

	rawLines := bytes.Split(bytes.TrimRight(data, "\n"), []byte{'\n'})
	lines := make([][]byte, 0, len(rawLines))
	for _, line := range rawLines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lines = append(lines, append([]byte(nil), line...))
	}
	return lines, nil
}

func trimAuditSpoolLines(lines [][]byte, maxBytes int64) [][]byte {
	for auditSpoolLinesSize(lines) > maxBytes && len(lines) > 0 {
		lines = lines[1:]
	}
	return lines
}

func auditSpoolLinesSize(lines [][]byte) int64 {
	var size int64
	for _, line := range lines {
		size += int64(len(line) + 1)
	}
	return size
}

func writeAuditSpoolLines(path string, lines [][]byte) error {
	if len(lines) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove audit spool: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create audit spool directory: %w", err)
	}
	data := bytes.Join(lines, []byte{'\n'})
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write audit spool: %w", err)
	}
	return nil
}

func (s *localAuditSpool) eventLimit() int {
	if s.maxEventBytes <= 0 {
		return localagents.MaxAuditEventBytes
	}
	return s.maxEventBytes
}

func (s *localAuditSpool) spoolLimit() int64 {
	if s.maxSpoolBytes <= 0 {
		return defaultAuditSpoolMaxBytes
	}
	return s.maxSpoolBytes
}
