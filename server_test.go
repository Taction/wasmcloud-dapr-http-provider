/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/14 6:52 PM
 */
package main

import (
	"testing"
	"time"

	provider "github.com/jordan-rash/wasmcloud-provider"
)

// generate a HttpServer and run.
func TestServer(t *testing.T) {
	conf := provider.ActorConfig{ActorID: "a", ActorConfig: map[string]string{"address": "0.0.0.0:8888", "unique_id": "a"}}
	server := New(conf, nil)
	err := server.Run()
	if err != nil {
		t.Errorf("Error starting server: %s", err)
	}
	time.Sleep(time.Second * 500)
}
