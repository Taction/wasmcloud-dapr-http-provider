package server

type HttpServerInterface interface {
	Run() error
	Shutdown()
}
