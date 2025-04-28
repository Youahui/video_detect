package ctl

import (
	"context"
	"sync"
)

type Context struct {
	Ctx        context.Context
	CancelFunc context.CancelFunc
	env        map[string]any
	rwLock     sync.RWMutex
}

func NewDefaultContext() *Context {
	ctx, cancel := context.WithCancel(context.Background())
	return &Context{
		Ctx:        ctx,
		CancelFunc: cancel,
		env:        make(map[string]any),
		rwLock:     sync.RWMutex{},
	}
}

func (c *Context) Set(key string, value any) {
	c.rwLock.Lock()
	defer c.rwLock.Unlock()
	c.env[key] = value
}

func (c *Context) Get(key string) (any, bool) {
	c.rwLock.RLock()
	defer c.rwLock.RUnlock()
	value, ok := c.env[key]
	return value, ok
}
