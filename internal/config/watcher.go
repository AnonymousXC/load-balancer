package config

import (
	"context"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

type Watcher struct {
	mgr    *Manager
	logger *zap.Logger
}

func NewWatcher(mgr *Manager, logger *zap.Logger) *Watcher {
	return &Watcher{mgr: mgr, logger: logger}
}

func (w *Watcher) Start(ctx context.Context, path string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(path); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if evt.Has(fsnotify.Write) {
				w.logger.Info("config changed, reloading")
				if err := w.mgr.Reload(); err != nil {
					w.logger.Error("reload failed", zap.Error(err))
				} else {
					w.logger.Info("config reloaded")
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			w.logger.Error("fsnotify error", zap.Error(err))
		}
	}
}
