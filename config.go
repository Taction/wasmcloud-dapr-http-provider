/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/14 10:08 AM
 */
package main

import (
	"github.com/taction/http-provider-go/ca"
)

type ProviderConfig struct {
	ResolverAddress string      `json:"resolver_address"`
	ExternalAddress string      `json:"external_address"`
	CAConfig        ca.CaConfig `json:"ca_config"`
}
