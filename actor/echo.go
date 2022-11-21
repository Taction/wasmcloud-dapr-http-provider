package main

import (
	"github.com/wasmcloud/actor-tinygo"
	httpserver "github.com/wasmcloud/interfaces/httpserver/tinygo"
)

func main() {
	me := Echo{}
	actor.RegisterHandlers(httpserver.HttpServerHandler(&me))
}

type Echo struct{}

func (e *Echo) HandleRequest(ctx *actor.Context, req httpserver.HttpRequest) (*httpserver.HttpResponse, error) {
	provider := httpserver.NewProviderHttpServer()
	req.Header["dapr-app-id"] = []string{"order-processor"} // todo clean req outside
	req.Body = append([]byte(`{"Proxy": "by wasmcloud", "origin":`), req.Body...)
	req.Body = append(req.Body, []byte(`}`)...)
	res, err := provider.HandleRequest(ctx, req)
	if err != nil {
		return InternalServerError(err), nil
	}
	return res, nil

}

func InternalServerError(err error) *httpserver.HttpResponse {
	return &httpserver.HttpResponse{
		StatusCode: 500,
		Header:     make(httpserver.HeaderMap, 0),
		Body:       []byte(err.Error()),
	}
}
