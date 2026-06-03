module github.com/chinudotdev/pi-go/sdk

go 1.24.5

require (
	github.com/chinudotdev/pi-go/agent v0.0.0-00010101000000-000000000000
	github.com/chinudotdev/pi-go/ai v0.0.0-00010101000000-000000000000
)

require github.com/goccy/go-yaml v1.19.2 // indirect

replace (
	github.com/chinudotdev/pi-go/agent => ../agent
	github.com/chinudotdev/pi-go/ai => ../ai
)
