package config

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Store struct {
	path   string
	active atomic.Pointer[Config]
	logger *slog.Logger
}

func NewStore(path string, cfg *Config, logger *slog.Logger) *Store {
	s := &Store{path: path, logger: logger}
	s.active.Store(cfg)
	return s
}

func (s *Store) Current() *Config {
	return s.active.Load()
}

func (s *Store) Watch(ctx context.Context, onChange func(*Config)) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	if err := watcher.Add(s.path); err != nil {
		return err
	}
	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("watcher closed")
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			debounce.Reset(300 * time.Millisecond)
		case <-debounce.C:
			cfg, err := Load(s.path)
			if err != nil {
				s.logger.Warn("config reload failed", "error", err)
				continue
			}
			prev := s.Current()
			s.active.Store(cfg)
			s.logger.Info("config reloaded", "path", s.path)
			if onChange != nil {
				onChange(cfg)
			}
			_ = prev
		case err, ok := <-watcher.Errors:
			if !ok {
				return fmt.Errorf("watcher error channel closed")
			}
			s.logger.Warn("config watch error", "error", err)
		}
	}
}
