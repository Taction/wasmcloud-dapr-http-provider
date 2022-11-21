package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dapr/components-contrib/nameresolution"
	invokev1 "github.com/dapr/dapr/pkg/messaging/v1"
	"github.com/dapr/dapr/pkg/modes"
	internalv1pb "github.com/dapr/dapr/pkg/proto/internals/v1"
	"github.com/dapr/kit/logger"
	provider "github.com/jordan-rash/wasmcloud-provider"
	httpserver "github.com/wasmcloud/interfaces/httpserver/tinygo"
	msgpack "github.com/wasmcloud/tinygo-msgpack"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/taction/http-provider-go/discovery"
	"github.com/taction/http-provider-go/discovery/consul"
	"github.com/taction/http-provider-go/server"
	"github.com/taction/http-provider-go/server/daprserver"
	"github.com/taction/http-provider-go/transport"
)

const (
	// needed to load balance requests for target services with multiple endpoints, ie. multiple instances.
	grpcServiceConfig = `{"loadBalancingPolicy":"round_robin"}`
	dialTimeout       = time.Second * 30
	daprAppID         = "dapr-app-id"
)

var log = logger.NewLogger("wasmcloud.httpprovider")

type HttpServerProvider struct {
	l              sync.Mutex
	lock           sync.RWMutex
	connectionPool *connectionPool
	ExternalHost   string
	Actors         map[string]server.HttpServerInterface
	Provider       provider.WasmcloudProvider
	Resolver       discovery.Discover // todo change ResolveID to `ResolveID(req ResolveRequest) ([]string, error)`
}
type remoteApp struct {
	id        string
	namespace string
	address   string
}

func NewHttpServerProvider() *HttpServerProvider {
	return &HttpServerProvider{
		Actors:         make(map[string]server.HttpServerInterface),
		connectionPool: newConnectionPool(),
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
			go p.evaluateRequest(actorRequest)
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
		Configuration: consul.IntermediateConfig{Client: &consul.Config{Address: c.ResolverAddress}, DaprPortMetaKey: nameresolution.DaprPort},
	})
	if err != nil {
		return err
	}
	p.Resolver = resolver
	p.ExternalHost = c.ExternalAddress
	return nil
}

// send request to outside
func (p *HttpServerProvider) evaluateRequest(actorRequest provider.ProviderAction) {
	log.Debugf("receive actor request operation: %s\n", actorRequest.Operation)
	resp := provider.ProviderResponse{}
	buf, err := p.daprRequest(actorRequest)
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Msg = buf
	}
	// Send response
	actorRequest.Respond <- resp
}

func (p *HttpServerProvider) daprRequest(actorRequest provider.ProviderAction) ([]byte, error) {
	operationL := strings.Split(actorRequest.Operation, ".")
	switch operationL[len(operationL)-1] {
	case "HandleRequest":
		// Decode the request from actor
		dec := msgpack.NewDecoder(actorRequest.Msg)
		req, err := httpserver.MDecodeHttpRequest(&dec)
		if err != nil {
			return nil, err
		}
		pres, err := p.callDaprRemote(context.TODO(), req)
		var sizer msgpack.Sizer
		sizeEnc := &sizer
		pres.MEncode(sizeEnc)
		buf := make([]byte, sizer.Len())
		encoder := msgpack.NewEncoder(buf)
		enc := &encoder
		pres.MEncode(enc)
		return buf, nil
	default:
		log.Errorf("unknown operation: %s\n", actorRequest.Operation)
		return nil, errors.New("Invalid Operation")
	}
}

func (g *HttpServerProvider) callDaprRemote(ctx context.Context, r httpserver.HttpRequest) (*httpserver.HttpResponse, error) {
	log.Debugf("actor call provider req: %+v\n", r)
	// todo check why header has empty string
	mh := metadata.MD{}
	if len(r.Header) > 0 {
		for k, v := range r.Header {
			for _, vv := range v {
				if vv != "" {
					mh.Append(k, vv)
				}
			}
		}
	}
	appId := ""
	if appIDs := mh[daprAppID]; len([]string(appIDs)) == 0 {
		return nil, errors.New("dapr-app-id not found")
	} else {
		appId = appIDs[0]
	}
	contentType := ""
	if len(mh) > 0 && len(mh["content-type"]) > 0 {
		contentType = r.Header["content-type"][0]
	}

	invokeMethodName := r.Path
	verb := strings.ToUpper(r.Method)
	// Construct internal invoke method request
	req := invokev1.NewInvokeMethodRequest(invokeMethodName).WithHTTPExtension(verb, r.QueryString)
	req.WithRawData(r.Body, contentType)
	// Save headers to internal metadata
	req.WithMetadata(mh)

	a, err := g.getRemoteApp(appId)
	conn, err := g.GetGRPCConnection(context.TODO(), a.address)
	if err != nil {
		return nil, err
	}
	clientV1 := internalv1pb.NewServiceInvocationClient(conn)
	var opts []grpc.CallOption
	opts = append(opts, grpc.MaxCallRecvMsgSize(4*1024*1024), grpc.MaxCallSendMsgSize(4*1024*1024))

	response, err := clientV1.CallLocal(ctx, req.Proto(), opts...)
	if err != nil {
		return nil, err
	}
	resp, err := invokev1.InternalInvokeResponse(response)
	if err != nil {
		return nil, err
	}
	// Convert response to HTTPServer response
	res := httpserver.HttpResponse{Header: map[string]httpserver.HeaderValues{}}
	contentType, body := resp.RawData()
	res.Header["content-type"] = []string{contentType}
	statusCode := int(resp.Status().Code)
	//statusCode = invokev1.HTTPStatusFromCode(codes.Code(statusCode))
	//if statusCode != http.StatusOK {
	//	var rErr error
	//	if body, rErr = invokev1.ProtobufToJSON(resp.Status()); rErr != nil {
	//		body = []byte(fmt.Sprintf("ERR_MALFORMED_RESPONSE %s", rErr.Error()))
	//		statusCode = fasthttp.StatusInternalServerError
	//	}
	//}
	res.Body = body
	res.StatusCode = uint16(statusCode)
	return &res, nil
}

func (d *HttpServerProvider) getRemoteApp(appID string) (remoteApp, error) {
	//id, namespace, err := d.requestAppIDAndNamespace(appID)
	//if err != nil {
	//	return remoteApp{}, err
	//}

	request := nameresolution.ResolveRequest{ID: appID}
	address, err := d.Resolver.ResolveID(request)
	if err != nil {
		return remoteApp{}, err
	}

	return remoteApp{
		id:      appID,
		address: address,
	}, nil
}

func (g *HttpServerProvider) GetGRPCConnection(parentCtx context.Context, address string, customOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	g.lock.RLock()
	if conn, ok := g.connectionPool.Share(address); ok {
		g.lock.RUnlock()
		return conn, nil
	}
	g.lock.RUnlock()

	g.lock.RLock()
	// read the value once again, as a concurrent writer could create it
	if conn, ok := g.connectionPool.Share(address); ok {
		g.lock.RUnlock()
		return conn, nil
	}
	g.lock.RUnlock()
	opts := []grpc.DialOption{
		grpc.WithDefaultServiceConfig(grpcServiceConfig),
		grpc.WithTransportCredentials(insecure.NewCredentials()), // todo fix tls or mtls usage
	}
	dialPrefix := GetDialAddressPrefix(modes.StandaloneMode) // todo define in config
	ctx, cancel := context.WithTimeout(parentCtx, dialTimeout)
	conn, err := grpc.DialContext(ctx, dialPrefix+address, opts...)
	cancel()
	if err != nil {
		return nil, err
	}
	g.lock.Lock()
	g.connectionPool.Register(address, conn)
	g.lock.Unlock()
	return conn, nil
}

// GetDialAddressPrefix returns a dial prefix for a gRPC client connections
// For a given DaprMode.
func GetDialAddressPrefix(mode modes.DaprMode) string {
	if runtime.GOOS == "windows" {
		return ""
	}

	switch mode {
	case modes.KubernetesMode:
		return "dns:///"
	default:
		return ""
	}
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
