// tools.go
//go:build tools

package tools

import (
	_ "github.com/atombender/go-jsonschema"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
