/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/14 11:38 AM
 */
package ca

import (
	"crypto/tls"
	"fmt"

	"gitlab.oneitfarm.com/bifrost/casdk"
	"gitlab.oneitfarm.com/bifrost/casdk/pkg/spiffe"
	cv2 "gitlab.oneitfarm.com/bifrost/cilog/v2"
	"gitlab.oneitfarm.com/bifrost/sesdk/discovery"

	"github.com/taction/http-provider-go/config"
)

type CaConfig struct {
	Enable       bool     `json:"enable"`
	VerifySource bool     `json:"verify_source"`
	AuthKey      string   `json:"auth_key"`
	Address      string   `json:"address"`
	AddressOCSP  string   `json:"address_ocsp"`
	Retry        int      `json:"retry"`
	Whitelist    []string `json:"whitelist"`
	SecurityMode uint8    `json:"security_mode"`
}

func NewDiscoveryTLSConfig(conf CaConfig) (tlsc *tls.Config, err error) {
	if !conf.Enable {
		return nil, nil
	}
	fmt.Println()
	l, _ := cv2.NewZapLogger(&cv2.Conf{
		Level: 2,
	})
	c := casdk.NewCAI(
		casdk.WithCAServer(casdk.RoleSidecar, conf.Address),
		casdk.WithAuthKey(conf.AuthKey),
		casdk.WithLogger(l),
		casdk.WithOcspAddr(conf.AddressOCSP),
	)
	identity := &spiffe.IDGIdentity{
		SiteID:    config.Region,
		ClusterID: config.Zone,
		UniqueID:  "provider",
	}
	exchanger, err := c.NewExchanger(identity)
	if err != nil {
		// conf.Enable = false
		// cilog.LogErrorw(cilog.LogNameSidecar, "Exchanger 初始化失败", err)
		return nil, err
		// return errors.New("Exchanger 初始化失败" + err.Error())
	}
	_, err = exchanger.Transport.GetCertificate()
	if err != nil {
		// conf.Enable = false
		// cilog.LogErrorw(cilog.LogNameSidecar, "GetCertificate 初始化失败", err)
		return nil, err
	}
	// 启动证书轮换
	go exchanger.RotateController().Run()

	cfger, err := exchanger.ClientTLSConfig(discovery.ServerName)
	if err != nil {
		return nil, err
	}
	return cfger.TLSConfig(), nil
}
