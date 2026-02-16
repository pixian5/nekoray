package main

import (
	"context"
	"net"
	"net/http"

	"github.com/matsuridayo/libneko/neko_common"
	"github.com/matsuridayo/libneko/neko_log"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/boxapi"
	boxmain "github.com/sagernet/sing-box/cmd/sing-box"
)

var instance *box.Box
var instance_cancel context.CancelFunc

func setupCore() {
	boxmain.SetDisableColor(true)
	//
	neko_log.SetupLog(50*1024, "./neko.log")
	//
	neko_common.GetCurrentInstance = func() interface{} {
		if instance != nil {
			return instance
		}
		if port := currentExternalSocksPort(); port > 0 {
			return port
		}
		return nil
	}
	neko_common.DialContext = func(ctx context.Context, specifiedInstance interface{}, network, addr string) (net.Conn, error) {
		if i, ok := specifiedInstance.(*box.Box); ok {
			return boxapi.DialContext(ctx, i, network, addr)
		}
		if instance != nil {
			return boxapi.DialContext(ctx, instance, network, addr)
		}
		return neko_common.DialContextSystem(ctx, network, addr)
	}
	neko_common.DialUDP = func(ctx context.Context, specifiedInstance interface{}) (net.PacketConn, error) {
		if i, ok := specifiedInstance.(*box.Box); ok {
			return boxapi.DialUDP(ctx, i)
		}
		if instance != nil {
			return boxapi.DialUDP(ctx, instance)
		}
		return neko_common.DialUDPSystem(ctx)
	}
	neko_common.CreateProxyHttpClient = func(specifiedInstance interface{}) *http.Client {
		if i, ok := specifiedInstance.(*box.Box); ok {
			return boxapi.CreateProxyHttpClient(i)
		}
		if port, ok := specifiedInstance.(int); ok && port > 0 {
			if client, err := createSocks5HttpClient(port); err == nil {
				return client
			}
		}
		if port := currentExternalSocksPort(); port > 0 {
			if client, err := createSocks5HttpClient(port); err == nil {
				return client
			}
		}
		if instance != nil {
			return boxapi.CreateProxyHttpClient(instance)
		}
		return &http.Client{}
	}
}
