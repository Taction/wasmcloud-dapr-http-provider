/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/16 11:15 PM
 */
package daprserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/dapr/dapr/pkg/messages"
	invokev1 "github.com/dapr/dapr/pkg/messaging/v1"
	commonv1pb "github.com/dapr/dapr/pkg/proto/common/v1"
	internalv1pb "github.com/dapr/dapr/pkg/proto/internals/v1"
	"github.com/dapr/kit/logger"
	provider "github.com/jordan-rash/wasmcloud-provider"
	commonmsgpack "github.com/vmihailenco/msgpack/v5"
	"github.com/wasmcloud/actor-tinygo"
	httpserver "github.com/wasmcloud/interfaces/httpserver/tinygo"
	msgpack "github.com/wasmcloud/tinygo-msgpack"
	grpcGo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/taction/http-provider-go/encode"
	"github.com/taction/http-provider-go/transport"
)

var log = logger.NewLogger("wasmcloud.dapr.server")

type API interface {
	// DaprInternal Service methods
	CallActor(ctx context.Context, in *internalv1pb.InternalInvokeRequest) (*internalv1pb.InternalInvokeResponse, error)
	CallLocal(ctx context.Context, in *internalv1pb.InternalInvokeRequest) (*internalv1pb.InternalInvokeResponse, error)
}

type header httpserver.HeaderMap

func (h header) Set(key, value string) {
	h[key] = []string{value}
}

type Api struct {
	internalv1pb.UnimplementedServiceInvocationServer
	Conf     provider.ActorConfig
	UniqueID string
	tp       transport.Transport
	server   *grpcGo.Server
}

func New(conf provider.ActorConfig, tp transport.Transport) *Api {
	uniqueId := conf.ActorConfig["unique_id"]
	return &Api{Conf: conf, UniqueID: uniqueId, tp: tp}
}

func (a *Api) Run() error {
	addr := a.Conf.ActorConfig["address"]
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	var opts []grpcGo.ServerOption
	opts = append(opts,
		grpcGo.MaxRecvMsgSize(4<<20),             //4MB
		grpcGo.MaxSendMsgSize(4<<20),             //4MB
		grpcGo.MaxHeaderListSize(uint32(64<<10)), //64KB
	)

	//opts = append(opts, grpcGo.UnknownServiceHandler(s.proxy.Handler()))
	s := grpcGo.NewServer(opts...)
	a.server = s
	go func() {
		internalv1pb.RegisterServiceInvocationServer(s, a)
		healthpb.RegisterHealthServer(s, health.NewServer())
		err := s.Serve(ln)
		log.Infof("Server for actor [%s] stopped with err: %s", a.Conf.ActorID, err)
	}()
	return nil
}

func (a *Api) Shutdown() {
	a.server.GracefulStop()
}

// CallLocal is used for internal dapr to dapr calls. It is invoked by another Dapr instance with a request to the local app.
func (a *Api) CallLocal(ctx context.Context, in *internalv1pb.InternalInvokeRequest) (*internalv1pb.InternalInvokeResponse, error) {

	req, err := invokev1.InternalInvokeRequest(in)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, messages.ErrInternalInvokeRequest, err.Error())
	}

	resp, err := a.InvokeMethod(ctx, req)
	if err != nil {
		err = status.Errorf(codes.Internal, messages.ErrChannelInvoke, err)
		return nil, err
	}
	return resp.Proto(), err
}

func (a *Api) InvokeMethod(ctx context.Context, req *invokev1.InvokeMethodRequest) (*invokev1.InvokeMethodResponse, error) {

	// Check if HTTP Extension is given. Otherwise, it will return error.
	httpExt := req.Message().GetHttpExtension()
	if httpExt == nil {
		return nil, status.Error(codes.InvalidArgument, "missing HTTP extension field")
	}
	// Go's net/http library does not support sending requests with the CONNECT method
	if httpExt.Verb == commonv1pb.HTTPExtension_NONE || httpExt.Verb == commonv1pb.HTTPExtension_CONNECT { //nolint:nosnakecase
		return nil, status.Error(codes.InvalidArgument, "invalid HTTP verb")
	}

	var rsp *invokev1.InvokeMethodResponse
	var err error
	switch req.APIVersion() {
	case internalv1pb.APIVersion_V1: //nolint:nosnakecase
		rsp, err = a.invokeMethodV1(ctx, req)

	default:
		// Reject unsupported version
		err = status.Error(codes.Unimplemented, fmt.Sprintf("Unsupported spec version: %d", req.APIVersion()))
	}

	return rsp, err
}

func (a *Api) invokeMethodV1(ctx context.Context, r *invokev1.InvokeMethodRequest) (*invokev1.InvokeMethodResponse, error) {
	req, err := a.constructRequest(ctx, r)
	if err != nil {
		return nil, err
	}

	//var resp *http.Response

	log.Debugf("Sending request to actor with request: %+v", req)
	body, err := encode.Encode(req)
	if err != nil {
		log.Warnf("Sending request to actor encode request err: %s", err)
		return nil, err
	}
	res, err := a.tp.Send(actor.Message{Method: "HttpServer.HandleRequest", Arg: body})
	if err != nil {
		log.Warnf("Sending request to actor err: %s", err)
		return nil, err
	}
	resp := httpserver.HttpResponse{}
	b := msgpack.NewDecoder(res)
	resp, err = httpserver.MDecodeHttpResponse(&b)
	//err = msgpack.Unmarshal(res, &resp)
	if err != nil {
		log.Warnf("Sending request to actor decode resp err: %s", err)
		return nil, err
	}

	rsp, err := a.parseChannelResponse(resp)
	if err != nil {
		return nil, err
	}

	return rsp, nil
}

func (a *Api) parseChannelResponse(resp httpserver.HttpResponse) (*invokev1.InvokeMethodResponse, error) {
	contentType := ""
	if len(resp.Header) > 0 && len(resp.Header["Content-Type"]) > 0 {
		contentType = resp.Header["content-type"][0]
	}

	mh := metadata.MD{}
	if len(resp.Header) > 0 {
		for k, v := range resp.Header {
			mh.Append(k, v...)
		}
	}
	// Convert status code
	rsp := invokev1.NewInvokeMethodResponse(int32(resp.StatusCode), "", nil).
		WithHeaders(mh).
		WithRawData(tryTransferMsgpackToJSON(resp.Body), contentType)

	return rsp, nil
}

func tryTransferMsgpackToJSON(m []byte) []byte {
	var v map[string]interface{}
	err := commonmsgpack.Unmarshal(m, &v)
	if err != nil {
		return m
	}
	j, err := json.Marshal(v)
	if err != nil {
		return m
	}
	return j
}

func (a *Api) constructRequest(ctx context.Context, req *invokev1.InvokeMethodRequest) (*httpserver.HttpRequest, error) {
	// Construct app channel URI: VERB http://localhost:3000/method?query1=value1
	msg := req.Message()
	verb := msg.HttpExtension.Verb.String()

	qs := req.EncodeHTTPQueryString()

	ct, body := req.RawData()

	he := header{}
	// Recover headers
	invokev1.InternalMetadataToHTTPHeader(ctx, req.Metadata(), he.Set)
	he.Set("content-type", ct)

	//return channelReq, nil
	return &httpserver.HttpRequest{
		Method:      verb,
		Path:        msg.Method,
		QueryString: qs,
		Body:        body,
		Header:      httpserver.HeaderMap(he),
	}, nil
}

func (a *Api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received request")
	if r.URL.Path == "/v1.0/healthz" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	req, err := transferRequest(r)
	if err != nil {
		a.handleError(w, err)
		return
	}
	log.Infof("Sending request to actor with request: %+v", req)
	body, err := encode.Encode(req)
	if err != nil {
		a.handleError(w, err)
		return
	}
	res, err := a.tp.Send(actor.Message{Method: "HttpServer.HandleRequest", Arg: body})
	if err != nil {
		a.handleError(w, err)
		return
	}
	resp := httpserver.HttpResponse{}
	b := msgpack.NewDecoder(res)
	resp, err = httpserver.MDecodeHttpResponse(&b)
	//err = msgpack.Unmarshal(res, &resp)
	if err != nil {
		a.handleError(w, err)
		return
	}
	if len(resp.Header) > 0 {
		for k, v := range resp.Header {
			w.Header().Set(k, v[0])
		}
	}
	w.WriteHeader(int(resp.StatusCode))
	// try transfer msgpack to json
	var v map[string]interface{}
	err = commonmsgpack.Unmarshal(resp.Body, &v)
	if err != nil {
		w.Write(resp.Body)
		return
	}
	j, err := json.Marshal(v)
	if err != nil {
		w.Write(resp.Body)
		return
	}
	w.Write(j)
}

func (a *Api) handleError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(err.Error()))
}

func transferRequest(r *http.Request) (*httpserver.HttpRequest, error) {
	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		return nil, err
	}
	h := httpserver.HeaderMap{}
	for k, v := range r.Header {
		h[k] = httpserver.HeaderValues(v)
	}
	return &httpserver.HttpRequest{
		Method: r.Method,
		Path:   r.URL.String(),
		Body:   body,
		Header: h,
	}, nil
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
