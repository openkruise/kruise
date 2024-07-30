package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"
	"k8s.io/kubelet/pkg/apis/credentialprovider/install"
	v1 "k8s.io/kubelet/pkg/apis/credentialprovider/v1"
)

var (
	scheme = runtime.NewScheme()
	codecs = serializer.NewCodecFactory(scheme)
)

func init() {
	install.Install(scheme)
}

func getCredentials(ctx context.Context, image string, args []string) (*v1.CredentialProviderResponse, error) {
	response := &v1.CredentialProviderResponse{
		CacheKeyType: v1.RegistryPluginCacheKeyType,
		Auth: map[string]v1.AuthConfig{
			"registry.plugin.com/test": {
				Username: "user",
				Password: "password",
			},
		},
	}
	return response, nil
}

func runPlugin(ctx context.Context, r io.Reader, w io.Writer, args []string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	_, err = json.DefaultMetaFactory.Interpret(data)
	if err != nil {
		return err
	}

	request, err := decodeRequest(data)
	if err != nil {
		return err
	}

	if request.Image == "" {
		return errors.New("image in plugin request was empty")
	}

	// Deny all requests except for those where the image URL contains registry.plugin.com
	// to test whether kruise could get expected auths if plugin fails to run
	if !strings.Contains(request.Image, "registry.plugin.com") {
		return errors.New("image in plugin request not supported: " + request.Image)
	}

	response, err := getCredentials(ctx, request.Image, args)
	if err != nil {
		return err
	}

	if response == nil {
		return errors.New("CredentialProviderResponse from plugin was nil")
	}

	encodedResponse, err := encodeResponse(response)
	if err != nil {
		return err
	}

	writer := bufio.NewWriter(w)
	defer writer.Flush()
	if _, err := writer.Write(encodedResponse); err != nil {
		return err
	}

	return nil
}

func decodeRequest(data []byte) (*v1.CredentialProviderRequest, error) {
	obj, gvk, err := codecs.UniversalDecoder(v1.SchemeGroupVersion).Decode(data, nil, nil)
	if err != nil {
		return nil, err
	}

	if gvk.Kind != "CredentialProviderRequest" {
		return nil, fmt.Errorf("kind was %q, expected CredentialProviderRequest", gvk.Kind)
	}

	if gvk.Group != v1.GroupName {
		return nil, fmt.Errorf("group was %q, expected %s", gvk.Group, v1.GroupName)
	}

	request, ok := obj.(*v1.CredentialProviderRequest)
	if !ok {
		return nil, fmt.Errorf("unable to convert %T to *CredentialProviderRequest", obj)
	}

	return request, nil
}

func encodeResponse(response *v1.CredentialProviderResponse) ([]byte, error) {
	mediaType := "application/json"
	info, ok := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), mediaType)
	if !ok {
		return nil, fmt.Errorf("unsupported media type %q", mediaType)
	}

	encoder := codecs.EncoderForVersion(info.Serializer, v1.SchemeGroupVersion)
	data, err := runtime.Encode(encoder, response)
	if err != nil {
		return nil, fmt.Errorf("failed to encode response: %v", err)
	}

	return data, nil
}

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	if err := newCredentialProviderCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newCredentialProviderCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "acr-credential-provider",
		Short: "ACR credential provider for kubelet",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runPlugin(context.TODO(), os.Stdin, os.Stdout, os.Args[1:]); err != nil {
				klog.ErrorS(err, "Error running credential provider plugin")
				os.Exit(1)
			}
		},
	}
	return cmd
}
