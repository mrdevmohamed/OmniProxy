module omniproxy/core

go 1.26.4

require omniproxy/engine v0.0.0

require (
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/zalando/go-keyring v0.2.8 // indirect
	golang.org/x/sys v0.27.0 // indirect
)

replace omniproxy/engine => ../engine
