# BUILDING MULTI-ARCHITECTURE IMAGES

While the test Docker images for OpenKruise can be built by running:

```
make docker-build
```

the stable images are build for multiple architectures (and automatically pushed to the repository) by

```
make docker-multiarch
```

However, there are several options and requirements for building these images, which are explained here.

## BUILDERS

We use `docker buildx` to build images for multiple architectures. At the moment, this is considered an
experimental feature by Docker, and as such it requires access to experimental features to be enabled
in the daemon.json file.

By default, this will use BuildKit and QEMU emulation support to build the image for each architecture
locally. However, for faster building, it is possible to use native nodes to build each architecture,
either simple remote Docker instances or a Kubernetes cluster. For information on how to set this up,
please see the blog article here:

<https://www.docker.com/blog/multi-arch-images/>

And/or the command reference here:

<https://docs.docker.com/engine/reference/commandline/buildx_create/>

## BUILD OPTIONS

### PLATFORMS

The PLATFORMS variable can be set to a comma-separated list of platforms for which images should be
built when running the multi-arch build. If unset, it defaults to _linux/amd64,linux/arm64,linux/arm_.
