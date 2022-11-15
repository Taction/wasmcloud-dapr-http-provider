package main

import (
	"github.com/wasmcloud/actor-tinygo"
	httpserver "github.com/wasmcloud/interfaces/httpserver/tinygo"
	msgpack "github.com/wasmcloud/tinygo-msgpack"
)

func main() {
	me := Echo{}
	actor.RegisterHandlers(httpserver.HttpServerHandler(&me))
}

type Echo struct{}

func (e *Echo) HandleRequest(ctx *actor.Context, req httpserver.HttpRequest) (*httpserver.HttpResponse, error) {
	return &httpserver.HttpResponse{
		StatusCode: 200,
		Header:     make(httpserver.HeaderMap, 0),
		Body:       MEncodeRequest(req),
	}, nil

}

func MEncodeRequest(req httpserver.HttpRequest) []byte {
	var sizer msgpack.Sizer
	sizeEnc := &sizer
	req.MEncode(sizeEnc)
	buf := make([]byte, sizer.Len())
	encoder := msgpack.NewEncoder(buf)
	enc := &encoder
	req.MEncode(enc)
	return buf
}
