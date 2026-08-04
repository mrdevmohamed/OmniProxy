package engine

import (
	"context"

	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/adapter/outbound"
	boxService "github.com/sagernet/sing-box/adapter/service"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/dns"
	dnsTransport "github.com/sagernet/sing-box/dns/transport"
	"github.com/sagernet/sing-box/dns/transport/local"
	_ "github.com/sagernet/sing-box/experimental/clashapi" // registers the clash server (log observable); no listener without ExternalController
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/block"
	"github.com/sagernet/sing-box/protocol/direct"
	"github.com/sagernet/sing-box/protocol/http"
	"github.com/sagernet/sing-box/protocol/mixed"
	"github.com/sagernet/sing-box/protocol/shadowsocks"
	"github.com/sagernet/sing-box/protocol/socks"
	"github.com/sagernet/sing-box/protocol/ssh"
	"github.com/sagernet/sing-box/protocol/tun"
	"github.com/sagernet/sing-box/protocol/vless"
	"github.com/sagernet/sing-box/protocol/vmess"
)

// newContext builds a context with registries for exactly the protocols
// OmniProxy needs (keeps the module lean compared to sing-box/include).
func newContext(ctx context.Context) context.Context {
	inboundRegistry := inbound.NewRegistry()
	outboundRegistry := outbound.NewRegistry()
	endpointRegistry := endpoint.NewRegistry()
	dnsTransportRegistry := dns.NewTransportRegistry()
	serviceRegistry := boxService.NewRegistry()

	inbound.Register[option.TunInboundOptions](inboundRegistry, C.TypeTun, tun.NewInbound)
	inbound.Register[option.HTTPMixedInboundOptions](inboundRegistry, C.TypeMixed, mixed.NewInbound)

	outbound.Register[option.DirectOutboundOptions](outboundRegistry, C.TypeDirect, direct.NewOutbound)
	outbound.Register[option.VLESSOutboundOptions](outboundRegistry, C.TypeVLESS, vless.NewOutbound)
	outbound.Register[option.VMessOutboundOptions](outboundRegistry, C.TypeVMess, vmess.NewOutbound)
	outbound.Register[option.ShadowsocksOutboundOptions](outboundRegistry, C.TypeShadowsocks, shadowsocks.NewOutbound)
	outbound.Register[option.SOCKSOutboundOptions](outboundRegistry, C.TypeSOCKS, socks.NewOutbound)
	outbound.Register[option.HTTPOutboundOptions](outboundRegistry, C.TypeHTTP, http.NewOutbound)
	outbound.Register[option.SSHOutboundOptions](outboundRegistry, C.TypeSSH, ssh.NewOutbound)
	// block drops routed traffic and logs each dropped destination; used by the
	// IPv6 block rule in Disable IPv6 mode (IPv6 leak detection).
	outbound.Register[option.StubOptions](outboundRegistry, C.TypeBlock, block.New)

	local.RegisterTransport(dnsTransportRegistry)
	dnsTransport.RegisterUDP(dnsTransportRegistry)

	return box.Context(ctx, inboundRegistry, outboundRegistry, endpointRegistry, dnsTransportRegistry, serviceRegistry)
}
