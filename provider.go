package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/dapr/components-contrib/nameresolution"
	"github.com/dapr/kit/logger"
	provider "github.com/jordan-rash/wasmcloud-provider"
	httpserver "github.com/wasmcloud/interfaces/httpserver/tinygo"
	msgpack "github.com/wasmcloud/tinygo-msgpack"

	"github.com/taction/http-provider-go/discovery"
	"github.com/taction/http-provider-go/discovery/consul"
	"github.com/taction/http-provider-go/server"
	"github.com/taction/http-provider-go/server/daprserver"
	"github.com/taction/http-provider-go/transport"
)

var log = logger.NewLogger("wasmcloud.httpprovider")

type HttpServerProvider struct {
	l            sync.Mutex
	ExternalHost string
	Actors       map[string]server.HttpServerInterface
	Provider     provider.WasmcloudProvider
	Resolver     discovery.Discover // todo change ResolveID to `ResolveID(req ResolveRequest) ([]string, error)`
}

func NewHttpServerProvider() *HttpServerProvider {
	return &HttpServerProvider{
		Actors: make(map[string]server.HttpServerInterface),
	}
}

func (p *HttpServerProvider) Run() (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		log.Infof("run error %s \n", err.Error())
	}()

	p.Provider, err = provider.Init(ctx)
	if err != nil {
		return err
	}
	log.Infof("provider init success, host data: %+v\n", p.Provider.HostData)
	err = p.initDiscovery()
	if err != nil {
		return err
	}

	// Listen for Shutdown request
	go func() {
		<-p.Provider.Shutdown
		close(p.Provider.ProviderAction)
		close(p.Provider.Links)
		cancel()
	}()

	// Start listening on topic for requests from actor
	p.Provider.ListenForActor("")

	go func() {
		//Wait for valid requests
		for actorRequest := range p.Provider.ProviderAction {
			go evaluateRequest(actorRequest)
		}
	}()

	// Wait for a valid link definiation
	log.Debug("Ready for link definitions")
	// put link
	for actorData := range p.Provider.Links {

		// Configure and start the server
		//lsConfig := validateConfig(actorData.ActorConfig)
		err := p.PutLink(actorData)
		if err != nil {
			log.Error(err)
		}
	}
	return nil
}

func (p *HttpServerProvider) initDiscovery() (err error) {
	c := ProviderConfig{}
	err = json.Unmarshal([]byte(p.Provider.HostData.ConfigJson), &c)
	if err != nil {
		return err
	}
	log.Debugf("init discovery with config: %+v\n", c)
	resolver := consul.NewResolver(log)
	err = resolver.Init(nameresolution.Metadata{
		Configuration: consul.IntermediateConfig{Client: &consul.Config{Address: c.ResolverAddress}},
	})
	if err != nil {
		return err
	}
	p.Resolver = resolver
	p.ExternalHost = c.ExternalAddress
	return nil
}

// send request to outside
func evaluateRequest(actorRequest provider.ProviderAction) error {
	log.Debugf("receive actor request operation: %s\n", actorRequest.Operation)
	resp := provider.ProviderResponse{}
	buf, err := doRequest(actorRequest)
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Msg = buf
	}
	// Send response
	actorRequest.Respond <- resp
	return nil
}

func doRequest(actorRequest provider.ProviderAction) ([]byte, error) {
	// Decode the request from actor
	decoder := msgpack.NewDecoder(actorRequest.Msg)
	req, err := httpserver.MDecodeHttpRequest(&decoder)
	if err != nil {
		return nil, err
	}
	log.Debugf("do actor request : %+v\n", req)

	client := http.DefaultClient
	httpreq, err := transferToHttpRequest(&req)
	if err != nil {
		return nil, err
	}
	// Do the Request
	res, err := client.Do(httpreq)
	if err != nil {
		return nil, err
	}
	pres, err := transferToResponse(res)
	// todo: 比较繁琐，使用泛型优化一下。
	// Encode response to actor
	var sizer msgpack.Sizer
	sizeEnc := &sizer
	pres.MEncode(sizeEnc)
	buf := make([]byte, sizer.Len())
	encoder := msgpack.NewEncoder(buf)
	enc := &encoder
	pres.MEncode(enc)
	return buf, nil
}

func transferToResponse(res *http.Response) (r *httpserver.HttpResponse, e error) {
	body, err := io.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		return nil, err
	}
	h := httpserver.HeaderMap{}
	for k, v := range r.Header {
		h[k] = httpserver.HeaderValues(v)
	}
	r = &httpserver.HttpResponse{
		StatusCode: uint16(res.StatusCode),
		Body:       body,
		Header:     h,
	}
	return r, nil
}

// ----------------------------------------

func (p *HttpServerProvider) PutLink(l provider.LinkDefinition) error {
	tr := transport.NewTransport(l, p.Provider.NatsConnection, p.Provider.HostData)
	c := l.ToActorConfig()
	server := daprserver.New(c, tr)
	err := server.Run()
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(c.ActorConfig["address"])
	if err != nil {
		return err
	}
	// todo  change address to host:port
	err = p.Resolver.RegisterToDiscovery(discovery.App{AppID: server.UniqueID, Address: fmt.Sprintf("%s:%s", p.ExternalHost, port)}) //nolint
	if err != nil {
		return err
	}
	p.l.Lock()
	p.Actors[c.ActorID] = server
	p.l.Unlock()
	return nil
}

func (p *HttpServerProvider) DeleteLink(actorID string) {
	p.l.Lock()
	s := p.Actors[actorID]
	delete(p.Actors, actorID)
	p.l.Unlock()
	go func() {
		p.Resolver.RemoveFromDiscovery(actorID)
		s.Shutdown()
	}()
}

func (p *HttpServerProvider) Shutdown() {
	p.l.Lock()
	for _, s := range p.Actors {
		s.Shutdown()
	}
	p.l.Unlock()
}

func transferToHttpRequest(r *httpserver.HttpRequest) (*http.Request, error) {
	body := bytes.NewBuffer(r.Body)
	req, err := http.NewRequest(r.Method, r.Path, body)
	if err != nil {
		return nil, err
	}
	for k, v := range r.Header {
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}
	return req, nil
}
