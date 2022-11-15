/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/14 10:33 AM
 */
package discovery

import (
	"context"
	"crypto/tls"
	"strings"
	"sync"
	"time"

	"github.com/dapr/components-contrib/nameresolution"
	"gitlab.oneitfarm.com/bifrost/cilog"
	"gitlab.oneitfarm.com/bifrost/sesdk"
	"gitlab.oneitfarm.com/bifrost/sesdk/discovery"

	"github.com/taction/http-provider-go/config"
	"github.com/taction/http-provider-go/log"
)

type Discovery struct {
	l    sync.Mutex
	apps map[string]App
	dis  *discovery.Discovery
}

type App struct {
	AppID   string
	Version string
	Address string
}

type Discover interface {
	nameresolution.Resolver
	RegisterToDiscovery(a App) (err error)
	RemoveFromDiscovery(appID string)
}

func NewDiscovery(disAddress string, tlsConfig *tls.Config) (*Discovery, error) {
	// 构建服务注册 - discovery集群节点地址
	conf := &discovery.Config{
		Nodes:     strings.Split(disAddress, ","),
		Zone:      config.Zone,
		Env:       config.Env,
		Region:    config.Region,
		Host:      "Hostname",
		RenewGap:  time.Second * 30,
		TLSConfig: tlsConfig,
	}
	// 启动注册中心客户端
	dis, err := discovery.New(conf)
	if err != nil {
		log.Log.Error(cilog.LogNameSidecar, "初始化动态注册中心配置错误", err)
		return nil, err
	}
	return &Discovery{
		apps: make(map[string]App),
		dis:  dis,
	}, nil
}

func (d *Discovery) Init(metadata nameresolution.Metadata) error {
	return nil
}
func (d *Discovery) ResolveID(req nameresolution.ResolveRequest) (string, error) {
	ep, err := d.dis.GetEndpoint(req.ID)
	if err != nil {
		return "", err
	}
	return ep.Host, nil
}

func (d *Discovery) RegisterToDiscovery(a App) (err error) {
	// 心跳时上报属性信息
	// sc.Discovery.RenewSet = sc.RenewSetAttribute
	// 构建服务注册 - discovery实例信息
	Instance := &sesdk.Instance{
		Zone:     config.Zone,
		Env:      "release",
		AppID:    a.AppID,
		Region:   config.Region,
		Addrs:    []string{a.Address},
		LastTs:   time.Now().Unix(),
		Hostname: a.AppID,
		Status:   discovery.InstanceStatusNotReceive,
		Version:  a.Version,
	}

	// 直接上线
	Instance.Status = discovery.InstanceStatusReceive

	_, err = d.dis.Register(context.TODO(), Instance)
	if err != nil {
		log.Log.Error("注册实例到动态注册中心失败", err)
		return
	}
	d.l.Lock()
	d.apps[a.AppID] = a
	d.l.Unlock()

	return nil
}

func (d *Discovery) RemoveFromDiscovery(appID string) {
	d.l.Lock()
	delete(d.apps, appID)
	d.l.Unlock()
	d.dis.Cancel(appID)
}
