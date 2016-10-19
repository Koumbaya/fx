// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package rpc

import (
	"fmt"

	"go.uber.org/fx/core"
	"go.uber.org/fx/core/config"
	"go.uber.org/fx/core/metrics"
	"go.uber.org/fx/core/ulog"
	"go.uber.org/fx/modules"

	"github.com/uber/tchannel-go"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport"
	tch "go.uber.org/yarpc/transport/tchannel"
)

// YarpcModule is an implementation of a core module using YARPC
type YarpcModule struct {
	modules.ModuleBase
	rpc      yarpc.Dispatcher
	register registerServiceFunc
	config   yarpcConfig
	log      ulog.Log
}

var _ core.Module = &YarpcModule{}

type registerServiceFunc func(module *YarpcModule)

// RPCModuleType represents the type of an RPC module
const RPCModuleType = "rpc"

type yarpcConfig struct {
	modules.ModuleConfig
	Bind          string `yaml:"bind"`
	AdvertiseName string `yaml:"advertise_name"`
}

func newYarpcModule(mi core.ModuleCreateInfo, reg registerServiceFunc, options ...modules.Option) (*YarpcModule, error) {
	cfg := &yarpcConfig{
		AdvertiseName: mi.Host.Name(),
		Bind:          ":0",
	}

	name := "yarpc"
	if mi.Name != "" {
		name = mi.Name
	}

	reporter := &metrics.LoggingTrafficReporter{Prefix: mi.Host.Name()}

	module := &YarpcModule{
		ModuleBase: *modules.NewModuleBase(RPCModuleType, name, mi.Host, reporter, []string{}),
		register:   reg,
		config:     *cfg,
	}

	module.log = ulog.Logger().With("moduleName", mi.Name)
	for _, opt := range options {
		if err := opt(&mi); err != nil {
			module.log.With("error", err, "option", opt).Error("Unable to apply option")
			return module, err
		}
	}

	if config.Global().GetValue(fmt.Sprintf("modules.%s", module.Name())).PopulateStruct(cfg) {
		// found values, update module
		module.config = *cfg
	}

	return module, nil
}

// Initialize sets up a YAPR-backed module
func (m *YarpcModule) Initialize(service core.ServiceHost) error {
	return nil
}

// Start begins serving requests over YARPC
func (m *YarpcModule) Start(readyCh chan<- struct{}) <-chan error {
	channel, err := tchannel.NewChannel(m.config.AdvertiseName, nil)
	if err != nil {
		m.log.Fatal("error", err)
	}

	m.rpc = yarpc.NewDispatcher(yarpc.Config{
		Name: m.config.AdvertiseName,
		Inbounds: []transport.Inbound{
			tch.NewInbound(channel, tch.ListenAddr(m.config.Bind)),
		},
	})

	m.register(m)
	ret := make(chan error, 1)
	// TODO update log object to be accessed via context.Context #74
	m.log.With("service", m.config.AdvertiseName, "port", m.config.Bind).Info("Server listening on port")

	ret <- m.rpc.Start()
	readyCh <- struct{}{}
	return ret
}

// Stop shuts down a YARPC module
func (m *YarpcModule) Stop() error {

	// TODO: thread safety
	if m.rpc != nil {
		err := m.rpc.Stop()
		m.rpc = nil
		return err
	}
	return nil
}

// IsRunning returns whether a module is running
func (m *YarpcModule) IsRunning() bool {
	// TODO: thread safety
	return m.rpc != nil
}