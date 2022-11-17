/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/14 2:10 PM
 */
package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestConfig(t *testing.T) {
	c := ProviderConfig{
		ResolverAddress: "http://127.0.0.1:8500",
		ExternalAddress: "127.0.0.1",
	}
	byt, _ := json.Marshal(c)

	fmt.Println(string(byt))
}
