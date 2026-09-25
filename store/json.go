package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/m1chlcz/localize/workbook"
)

const maxStoreBytes = 16 << 20

type JSONStore struct {
	Path string
}

func (s JSONStore) Snapshot(ctx context.Context) (workbook.Workbook, error) {
	state, err := s.load()
	if err != nil {
		return workbook.Workbook{}, err
	}
	return Snapshot(state), nil
}

func (s JSONStore) Apply(ctx context.Context, incoming workbook.Workbook, dryRun bool) (Result, error) {
	var result Result
	err := s.withLock(func() error {
		state, err := s.load()
		if err != nil {
			return err
		}
		next, changes, err := Merge(state, incoming)
		if err != nil {
			return err
		}
		result = Result{Applied: !dryRun, Changes: changes, Revision: next.Revision}
		if dryRun {
			return nil
		}
		return s.save(next)
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func (s JSONStore) load() (State, error) {
	file, err := os.Open(s.Path)
	if err != nil {
		return State{}, fmt.Errorf("open store: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxStoreBytes+1))
	if err != nil {
		return State{}, fmt.Errorf("read store: %w", err)
	}
	if len(data) > maxStoreBytes {
		return State{}, errors.New("store is too large")
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, errors.New("store is not valid JSON")
	}
	if state.Version != workbook.Version {
		return State{}, fmt.Errorf("unsupported store version %q", state.Version)
	}
	if state.Revision < 1 {
		return State{}, errors.New("store revision must be positive")
	}
	if err := workbook.Validate(Snapshot(state)); err != nil {
		return State{}, fmt.Errorf("store is invalid: %w", err)
	}
	return state, nil
}

func (s JSONStore) save(state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode store: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(s.Path)
	temporary, err := os.CreateTemp(directory, ".store-*")
	if err != nil {
		return fmt.Errorf("create temporary store: %w", err)
	}
	path := temporary.Name()
	defer func() { _ = os.Remove(path) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write store: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync store: %w", err)
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set store permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close store: %w", err)
	}
	if err := os.Rename(path, s.Path); err != nil {
		return fmt.Errorf("publish store: %w", err)
	}
	handle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open store directory: %w", err)
	}
	defer handle.Close()
	return handle.Sync()
}

func (s JSONStore) withLock(run func() error) error {
	directory := filepath.Dir(s.Path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create store directory: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(directory, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open store lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock store: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return run()
}
