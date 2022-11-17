/*
Copyright 2021 The Dapr Authors
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package consul

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"sync"

	nr "github.com/dapr/components-contrib/nameresolution"
	"github.com/dapr/kit/logger"
	consul "github.com/hashicorp/consul/api"

	"github.com/taction/http-provider-go/discovery"
)

const daprMeta string = "DAPR_PORT" // default key for DAPR_PORT metadata

type client struct {
	*consul.Client
}

func (c *client) InitClient(config *consul.Config) error {
	var err error

	c.Client, err = consul.NewClient(config)
	if err != nil {
		return fmt.Errorf("consul api error initing client: %w", err)
	}

	return nil
}

func (c *client) Agent() agentInterface {
	return c.Client.Agent()
}

func (c *client) Health() healthInterface {
	return c.Client.Health()
}

type clientInterface interface {
	InitClient(config *consul.Config) error
	Agent() agentInterface
	Health() healthInterface
}

type agentInterface interface {
	Self() (map[string]map[string]interface{}, error)
	ServiceRegister(service *consul.AgentServiceRegistration) error
	ServiceDeregister(serviceID string) error
}

type healthInterface interface {
	Service(service, tag string, passingOnly bool, q *consul.QueryOptions) ([]*consul.ServiceEntry, *consul.QueryMeta, error)
}

type Resolver struct {
	config resolverConfig
	logger logger.Logger
	client clientInterface
	l      sync.Mutex
	apps   map[string]discovery.App
}

type resolverConfig struct {
	Client          *consul.Config
	QueryOptions    *consul.QueryOptions
	Registration    *consul.AgentServiceRegistration
	DaprPortMetaKey string
}

// NewResolver creates Consul name Resolver.
func NewResolver(logger logger.Logger) *Resolver {
	return newResolver(logger, resolverConfig{}, &client{})
}

func newResolver(logger logger.Logger, resolverConfig resolverConfig, client clientInterface) *Resolver {
	return &Resolver{
		logger: logger,
		config: resolverConfig,
		client: client,
		apps:   make(map[string]discovery.App),
	}
}

// Init will configure component. It will also register service or validate client connection based on config.
func (r *Resolver) Init(metadata nr.Metadata) error {
	var err error

	r.config, err = getConfig(metadata)
	if err != nil {
		return err
	}

	if err = r.client.InitClient(r.config.Client); err != nil {
		return fmt.Errorf("failed to init consul client: %w", err)
	}

	return nil
}

// ResolveID resolves name to address via consul.
func (r *Resolver) ResolveID(req nr.ResolveRequest) (string, error) {
	cfg := r.config
	services, _, err := r.client.Health().Service(req.ID, "", true, cfg.QueryOptions)
	if err != nil {
		return "", fmt.Errorf("failed to query healthy consul services: %w", err)
	}

	if len(services) == 0 {
		return "", fmt.Errorf("no healthy services found with AppID:%s", req.ID)
	}

	shuffle := func(services []*consul.ServiceEntry) []*consul.ServiceEntry {
		for i := len(services) - 1; i > 0; i-- {
			rndbig, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
			j := rndbig.Int64()

			services[i], services[j] = services[j], services[i]
		}

		return services
	}

	svc := shuffle(services)[0]

	addr := ""

	if port, ok := svc.Service.Meta[cfg.DaprPortMetaKey]; ok {
		if svc.Service.Address != "" {
			addr = fmt.Sprintf("%s:%s", svc.Service.Address, port)
		} else if svc.Node.Address != "" {
			addr = fmt.Sprintf("%s:%s", svc.Node.Address, port)
		} else {
			return "", fmt.Errorf("no healthy services found with AppID:%s", req.ID)
		}
	} else {
		return "", fmt.Errorf("target service AppID:%s found but DAPR_PORT missing from meta", req.ID)
	}

	return addr, nil
}

func (r *Resolver) RegisterToDiscovery(a discovery.App) (err error) {
	uname := a.AppID + a.Host
	host, port, err := net.SplitHostPort(a.Address)
	pint, err := strconv.Atoi(port)
	// http health check
	//checks := []*consul.AgentServiceCheck{
	//	{
	//		Name:     "Wasm Http Provider Health Status",
	//		CheckID:  fmt.Sprintf("wasmHealth:%s", a.AppID),
	//		Interval: "15s",
	//		HTTP:     fmt.Sprintf("http://%s/v1.0/healthz", a.Address),
	//	},
	//}
	checks := []*consul.AgentServiceCheck{
		{
			Name:                           "Wasm Http Provider Health Status",
			CheckID:                        fmt.Sprintf("wasmHealth:%s", a.AppID),
			Interval:                       "15s",
			Timeout:                        "5s",
			GRPC:                           fmt.Sprintf(a.Address),
			DeregisterCriticalServiceAfter: "120s",
		},
	}
	Instance := consul.AgentServiceRegistration{
		ID:      a.AppID,
		Name:    uname,
		Address: host,
		Port:    pint,
		Checks:  checks,
		Tags:    []string{"wasmcloud"},
		Meta:    map[string]string{nr.DaprPort: port},
	}

	if err := r.client.Agent().ServiceRegister(&Instance); err != nil {
		return fmt.Errorf("failed to register consul service: %w", err)
	}

	r.l.Lock()
	r.apps[a.AppID] = a
	r.l.Unlock()

	return nil
}

func (r *Resolver) RemoveFromDiscovery(appID string) {
	r.l.Lock()
	delete(r.apps, appID)
	r.l.Unlock()
	err := r.client.Agent().ServiceDeregister(appID)
	if err != nil {
		fmt.Printf("failed to deregister consul service %s: %s", appID, err)
	}
}

// getConfig configuration from metadata, defaults are best suited for self-hosted mode.
func getConfig(metadata nr.Metadata) (resolverConfig, error) {
	var err error
	resolverCfg := resolverConfig{}

	cfg, err := parseConfig(metadata.Configuration)
	if err != nil {
		return resolverCfg, err
	}
	resolverCfg.Client = getClientConfig(cfg)
	resolverCfg.QueryOptions = getQueryOptionsConfig(cfg)

	return resolverCfg, nil
}

func getClientConfig(cfg configSpec) *consul.Config {
	// If no client config use library defaults
	if cfg.Client != nil {
		return cfg.Client
	}

	return consul.DefaultConfig()
}

func getQueryOptionsConfig(cfg configSpec) *consul.QueryOptions {
	// if no query options configured add default filter matching every tag in config
	if cfg.QueryOptions == nil {
		return &consul.QueryOptions{
			UseCache: true,
		}
	}

	return cfg.QueryOptions
}
