---
title: Proposal Template
authors:
- "@hoopoe61"
reviewers:
- "@jiuzhu"
creation-date: 2021-04-02
last-updated: 2021-04-20
status: implementable
---

# UnitedObject: describe similar k8s objects unitedly

Motivation:
  In now k8s implementation，similar objects are needed to be created sperately. This way produces duplicated and distributed content in k8s, which increases maintenance cost.

Goal:
  A common way is required to abstract and encapsulate similar objects in k8s, so objects with slight difference can be described unitedly in one object.

Proposal:
  This proposal try to abstract and encapsulate similar objects in k8s.
  1. Adding a new CRD and controller for UnitedObject.
  2. UnitedObject can be used to abstract all kinds of k8s objects.
  3. Use jsonPatch to describe the differences among objects in one UnitedObject.

Use case
  1. k8s objects with similar attributes, for example:
      1) services with only different label selector or serviceTypes
      2) pods with only different ENV or command
      3) the clusterrole binding for defalut serviceaccounts in different namespace
  2. k8s objects with different deployment requirements, for example:
      1) different resoure requirements
      2) different deployment zones

CRD example:
  this following crd will create 2 services with different label seletors
```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: UnitedObject
metadata:
  name: sample-uo-service
spec:
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: sample-uo-service
  template:
      # special attention 0: the apiVersion, kind and namespace in template cannot be updated
      apiVersion: v1
      kind: Service
      metadata:
        labels:
          app: sample-uo-service
        name: sample-uo-example
        namespace: default
      spec:
        ports:
        - name: http
          port: 15001
          protocol: TCP
          targetPort: http
        - name: tcp
          port: 5000
          protocol: TCP
          targetPort: 5001
        selector:
          app: sample-uo-service
  instances:
  # special attention 2: the gvk, name, namespace set in instances will not take effect.
  # gvk: same with the corresponding values set in template
  # name: same with the name set in instances
  # namespace: same with the namespace of corresponding UnitedObject
  - name: sample-uo-example-1
    instanceValues:
    # special attention 3: the '.' in filedPath but not used as path seprator should be replaced with '\.'
    # for example, to set the selector labels 'app.alibaba.com', the filedPath should be 'spec.selector.app\.alibaba\.com'
    - fieldPath: spec.selector.app\.alibaba\.com
      value: '"sample-uo-example-alibaba"'
  - name: sample-uo-example-2
    instanceValues:
    # special attention 4: the '~' and '/' in filedPath should be replaced with '~0' and '~1' respectively, according the rules in http://jsonpatch.com/
    # for example, to set the selector labels 'app/alibaba.com', the filedPath should be 'app~1alibaba\.com'
    - fieldPath: spec.selector.app~1alibaba\.com
      value: '"sample-uo-example-alibaba"'
```
