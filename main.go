package main

import (
	_ "github.com/taction/http-provider-go/log"
)

func main() {
	p := NewHttpServerProvider()
	p.Run()
}
