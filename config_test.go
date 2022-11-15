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

	"github.com/taction/http-provider-go/ca"
)

func TestConfig(t *testing.T) {
	c := ProviderConfig{
		ResolverAddress: "service-eye.msp:9443",
		ExternalAddress: "10.32.4.215",
		CAConfig: ca.CaConfig{
			Enable:       true,
			VerifySource: false,
			AuthKey:      "2bb6dabb91a14d8bac6bafd9de0bb7813f79e2fddfbb44be9cc8d8220e3d309b",
			Address:      "https://capitalizone-tls.msp:8081",
			AddressOCSP:  "http://capitalizone-ocsp.msp:8082",
			Retry:        3,
			SecurityMode: 1,
		},
	}
	byt, _ := json.Marshal(c)

	fmt.Println(string(byt))
}
