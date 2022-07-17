---
title: ResourceDistribution Generator
authors:
  - "@dong4325"
reviewers:
  - "@Fei-Guo"
  - "@FillZpp"
creation-date: 2022-07-17
last-updated: 2022-07-19
status: implementable
---

# resourcedistribution-generator

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Title](#title)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Proposal](#proposal)
    - [API Definition](#api-definition)
    - [NameSuffixHash](#namesuffixhash)
  - [User Cases](#user-Cases)
    - [A Sample Case](#a-sample-case)
      - [Options and ResourceOptions](#options-and-resourceoptions)
      - [Resource Source](#resource-source)
      - [Type](#type)
      - [Targets](#targets)
      - [Behavior](#behavior)
  - [Implementation History](#implementation-history)

## Motivation

`ResourceDistribution` is a resource provided by OpenKruise that is used to distribute and synchronize `ConfigMap`, `Secret`, and other resources across multiple namespaces. Using `ResourceDistribution` requires a full description of the resource being distributed, and manually modifying and maintaining the YAML file format is very difficult. `ConfigMapGenerator` avoids this problem by importing data directly from a file to create a confimap. We need a similar tool to create a `ResourceDistribution` by importing data directly from a file. Kustomize supports custom plugin to help us do this.

## Proposal

### API Definition

```go
type resourceDistributionPlugin struct{
  types.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
  ResourceArgs     `json:"resource,omitempty" yaml:"resource,omitempty"`
  TargetsArgs      `json:"targets,omitempty" yaml:"targets,omitempty"`

  // Options for the resourcedistribution.
  // GeneratorOptions same as configmap and secret generator Options
  Options *types.GeneratorOptions `json:"options,omitempty" yaml:"options,omitempty"`

  // Behavior of generated resource, must be one of:
  //   'create': create a new one
  //   'replace': replace the existing one
  //   'merge': merge with the existing one
  Behavior string `json:"behavior,omitempty" yaml:"behavior,omitempty"`
}

// ResourceArgs contain arguments for the resource to be distributed.
type ResourceArgs struct {
  // Name of the resource to be distributed.
  ResourceName string `json:"resourceName,omitempty" yaml:"resourceName,omitempty"`
  // Only configmap and secret are available
  ResourceKind string `json:"resourceKind,omitempty" yaml:"resourceKind,omitempty"`

  // KvPairSources defines places to obtain key value pairs.
  // same as configmap and secret generator KvPairSources
  types.KvPairSources `json:",inline,omitempty" yaml:",inline,omitempty"`

  // Options for the resource to be distributed.
  // GeneratorOptions same as configmap and secret generator Options
  ResourceOptions *types.GeneratorOptions `json:"resourceOptions,omitempty" yaml:"resourceOptions,omitempty"`

  // Type of the secret. It can be "Opaque" (default), or "kubernetes.io/tls".
  //
  // If type is "kubernetes.io/tls", then "literals" or "files" must have exactly two
  // keys: "tls.key" and "tls.crt"
  Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

// TargetsArgs defines places to obtain target namespace args.
type TargetsArgs struct {
  // AllNamespaces if true distribute all namespaces
  AllNamespaces bool `json:"allNamespaces,omitempty" yaml:"allNamespaces,omitempty"`

  // ExcludedNamespaces is a list of excluded namespaces name.
  ExcludedNamespaces []string `json:"excludedNamespaces,omitempty" yaml:"excludedNamespaces,omitempty"`

  // IncludedNamespaces is a list of included namespaces name.
  IncludedNamespaces []string `json:"includedNamespaces,omitempty" yaml:"includedNamespaces,omitempty"`

  // NamespaceLabelSelector for the generator.
  NamespaceLabelSelector *metav1.LabelSelector `json:"namespaceLabelSelector,omitempty" yaml:"namespaceLabelSelector,omitempty"`
}
```

### NameSuffixHash
If both resourcedistribution and resource's names are suffixed with hash values, they will have the same hash value.
> The name suffix hash function is temporarily not implemented

## User Cases

Make a place to work

```bash
tmpGoPath=$(mktemp -d)
```

Install kustomize

```bash
export GOPATH=$tmpGoPath
GO111MODULE=on go get sigs.k8s.io/kustomize/kustomize/v3
```

After pulling the plugin code, build the plugin object.

```bash
go build -o resourcedistributiongenerator ./main.go
```

### A Sample Case

1. Create an empty directory demo. Make a config file for the ResourceDistributionGenerator plugin in the demo directory, and the file named rdGenerator.yaml.
   The `config.kubernetes.io/function` annotation is used to find the plugin object on your filesystem. 
   The use of `options`, `literals` and other resource fields can also refer to the `options` and `literals` fields of [configmapGenerator](https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/configmapgenerator/).
```yaml
# The apiVersion and kind fields are required because a kustomize plugin configuration objects are also Kubernetes objects.
apiVersion: apps.kruise.io/v1alpha1
kind: ResourceDistributionGenerator
metadata:
  name: rdname
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: ./plugins/resourcedistributiongenerator
resource:
  resourceKind: ConfigMap # Only configmap and secret are available
  resourceName: cmname
  # The sources of the resource must be literals, files and envs
  files:
    - file.properties
  literals: # literals is a list of literal pair sources. Each literal source should be a key and literal value, e.g. `key=value`
    - JAVA_HOME=/opt/java/jdk
  resourceOptions: # resourceOptions for resource to be distributed
    annotations:
      dashboard: "1"
    disableNameSuffixHash: true
options: # options for resourceDistribution
  labels:
    app.kubernetes.io/name: "app1"
targets: # The target namespaces rules must be allNamespaces, includedNamespaces, excludedNamespaces or namespaceLabelSelector
  includedNamespaces:
    - ns-1
  namespaceLabelSelector:
    matchLabels:
      group: "test"
```

2. Create application.properties file in the demo directory with the following contents.

```properties
FOO=Bar
```

3. Make a kustomization file referencing the plugin config in the demo directory.

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  ...
generators:
- rdGenerator.yaml
```
4. Use the `$tmpGoPath/bin/kustomize build --enable-alpha-plugins --enable-exec demo` command to build your app from plugin

```yaml
...
apiVersion: apps.kruise.io/v1alpha1
kind: ResourceDistribution
metadata:
  name: rdname-c22bfmgk5g
  labels:
    app.kubernetes.io/name: app1
spec:
  resource:
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: cmname
      annotations:
        dashboard: "1"
    data:
      JAVA_HOME: /opt/java/jdk
      file.properties: |
        FOO=Bar
  targets:
    includedNamespaces:
      list:
        - name: ns-1
    namespaceLabelSelector:
      matchLabels:
        group: test
...
```

#### Options and ResourceOptions

The options field modify resourcedistribution,  and has three field  `labels`, `annotations` and `disableNameSuffixHash`.

The resourceOptions field modify resource to be distributed, and has one more `immutable` filed than options.

```yaml
resource:
  resourceOptions:
    labels:
      app.kubernetes.io/name: "app1"
    annotations:
      dashboard: "1"
    disableNameSuffixHash: true
    immutable: ture
```

#### Resource Source

The sources of the resource must be `literals`, `files` or `envs`，the same as configmap and secret generator. The contents of env files should be one key=value pair per line.

```yaml
resource:
  envs:
    - text.env
```

#### Type

Can only be used when `resourceKind` is `secret`.

It can be "Opaque" (default), or "kubernetes.io/tls". If type is "kubernetes.io/tls", then `literals` or `files` must have exactly two keys: "tls.key" and "tls.crt"

```yaml
resource:
  type: Opaque
```

#### Targets

Target namespaces rules has other two options except for `includedNamespaces` and `namespaceLabelSelector`

```yaml
targets:
  allNamespace: true
  excludedNamespaces:
    - ns-2
```

The `namespaceLabelSelector` field has `matchExpression` option except for `matchLabels`:

```yaml
targets:
  namespaceLabelSelector:
    matchExpressions:
      - key: app
        operator: In
        values:
          - dev
```

Operator must be one of: 'In', 'NotIn', 'Exists' and 'DoesNotExist'.

#### Behavior

Behavior represents the strategy for merging resources with the same name as the superior,  must be one of: create, replace, merge.

> The behavior function is temporarily not implemented

```yaml
behavior: create
```

## Implementation History

- [ ] 08/01/2022: Proposal submission
