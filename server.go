package main

import (
	"bytes"
	"context"
	"io"
	logos "log"
	"net/http"
	"time"

	provider "github.com/jordan-rash/wasmcloud-provider"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/wasmcloud/actor-tinygo"
	httpserver "github.com/wasmcloud/interfaces/httpserver/tinygo"

	"github.com/taction/http-provider-go/log"
	"github.com/taction/http-provider-go/transport"
)

type HttpServer struct {
	Conf     provider.ActorConfig
	UniqueID string
	server   *http.Server
	tp       transport.Transport
}

type HttpServerInterface interface {
	Run() error
	Shutdown()
}

func New(conf provider.ActorConfig, tp transport.Transport) *HttpServer {
	uniqueId := conf.ActorConfig["unique_id"]
	return &HttpServer{Conf: conf, UniqueID: uniqueId, tp: tp}
}

func (h *HttpServer) Run() error {
	address := h.Conf.ActorConfig["address"]
	h.server = &http.Server{Addr: address, Handler: h}
	go func() {
		h.server.ListenAndServe()
	}()
	return nil
}

func (h *HttpServer) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	err := h.server.Shutdown(ctx)
	log.Log.Errorf("Error shutting down server for actor [%s] err: %s", h.Conf.ActorID, err)
	cancel()
}

func (h *HttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Log.Infof("Received request")
	logos.Println("Received request to actor", h.Conf.ActorID)
	//sender := httpserver.NewActorHttpServerSender(h.Conf.ActorID)
	req, err := transferRequest(r)
	if err != nil {
		h.handleError(w, err)
		return
	}
	log.Log.Infof("Sending request to actor with request: %+v", req)
	logos.Printf("Sending request to actor with request: %+v\n", req)
	body, err := msgpack.Marshal(req)
	if err != nil {
		h.handleError(w, err)
		return
	}
	res, err := h.tp.Send(actor.Message{Method: "HttpServer.HandleRequest", Arg: body})
	if err != nil {
		h.handleError(w, err)
		return
	}
	resp := httpserver.HttpResponse{}
	err = msgpack.Unmarshal(res, &resp)
	if err != nil {
		h.handleError(w, err)
		return
	}
	for k, v := range resp.Header {
		w.Header().Set(k, v[0])
	}
	w.WriteHeader(int(resp.StatusCode))
	w.Write(resp.Body)
}

func (h *HttpServer) handleError(w http.ResponseWriter, err error) {
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
