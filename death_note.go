package main

import "sync"

type DeathNote struct {
	paramSet map[string]bool
	rwMutex sync.RWMutex
}

func NewDeathNote() *DeathNote {
	return &DeathNote{
		paramSet: map[string]bool{},
	}
}

func (d *DeathNote) AddParam(param string) {
	d.rwMutex.Lock()
	defer d.rwMutex.Unlock()

	d.paramSet[param] = true
}

func (d *DeathNote) GetParams() []string {
	d.rwMutex.RLock()
	defer d.rwMutex.RUnlock()

	params := make([]string, 0, len(d.paramSet))

	for param := range d.paramSet {
		params = append(params, param)
	}

	return params
}
