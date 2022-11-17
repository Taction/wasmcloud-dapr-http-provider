/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/14 10:33 AM
 */
package discovery

import (
	"github.com/dapr/components-contrib/nameresolution"
)

type App struct {
	AppID   string
	Version string
	Address string
	Host    string
}

type Discover interface {
	nameresolution.Resolver
	RegisterToDiscovery(a App) (err error)
	RemoveFromDiscovery(appID string)
}
