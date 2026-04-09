module github.com/openkruise/kruise/hack/calculator-helper/validator

go 1.23.0

require (
	github.com/openkruise/kruise v0.0.0
	golang.org/x/image v0.18.0
	k8s.io/apimachinery v0.32.10
)

require (
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
)

replace github.com/openkruise/kruise => ../../..
