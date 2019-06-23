# Install with helm v3

From [Helm v3 releases](https://github.com/helm/helm/releases/tag/v3.0.0-alpha.1).

Or, some of Helm v3 Latest Release on Aliyun OSS:

* [MacOS amd64 tar.gz](https://cloudnativeapphub.oss-cn-hangzhou.aliyuncs.com/helm-v3.0.0-alpha.1-darwin-amd64.tar.gz)
* [MacOS amd64 zip](https://cloudnativeapphub.oss-cn-hangzhou.aliyuncs.com/helm-v3.0.0-alpha.1-darwin-amd64.zip)
* [Linux 386](https://cloudnativeapphub.oss-cn-hangzhou.aliyuncs.com/helm-v3.0.0-alpha.1-linux-386.tar.gz)
* [Linux amd64](https://cloudnativeapphub.oss-cn-hangzhou.aliyuncs.com/helm-v3.0.0-alpha.1-linux-amd64.tar.gz) 
* [Linux arm64](https://cloudnativeapphub.oss-cn-hangzhou.aliyuncs.com/helm-v3.0.0-alpha.1-linux-arm64.tar.gz)
* [Windows amd64](https://cloudnativeapphub.oss-cn-hangzhou.aliyuncs.com/helm-v3.0.0-alpha.1-windows-amd64.zip)

If you are using Helm for first time, you may need to init Helm:

```
helm init
```

Add the [AppHub](https://developer.aliyun.com/hub) repository to your Helm:

```
helm repo add apphub https://apphub.aliyuncs.com

helm repo list
NAME      URL
apphub    https://apphub.aliyuncs.com
```

**Happy Helming in China!**
