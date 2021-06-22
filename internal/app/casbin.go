package app

import (
	"time"

	"github.com/LyricTian/gin-admin/v7/internal/app/config"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/persist"
)

// InitCasbin 初始化casbin
func InitCasbin(adapter persist.Adapter) (*casbin.SyncedEnforcer, func(), error) {
	cfg := config.C.Casbin
	if cfg.Model == "" {
		return new(casbin.SyncedEnforcer), nil, nil
	}

	e, err := casbin.NewSyncedEnforcer(cfg.Model, adapter)
	if err != nil {
		return nil, nil, err
	}
	e.EnableLog(cfg.Debug)
	e.EnableEnforce(cfg.Enable)

	cleanFunc := func() {}
	if cfg.AutoLoad {
		e.StartAutoLoadPolicy(time.Duration(cfg.AutoLoadInternal) * time.Second)
		cleanFunc = func() {
			e.StopAutoLoadPolicy()
		}
	}

	return e, cleanFunc, nil
}
