---
title: AddSidecarForProbeAndHotUpdate
authors:
  - "@lizhipeng629"
reviewers:
  - "@furykerry"
  - "@zmberg"
creation-date: 2024-11-26
last-updated: 2024-11-26
status: 

---
# Add Sidecar to Address Service Quality Probing and Hot Updates in Serverless Scenarios
## Table of Contents

- [Add Sidecar to Address Service Quality Probing and Hot Updates in Serverless Scenarios](#add-sidecar-to-address-service-quality-probing-and-hot-updates-in-serverless-scenarios)
    - [Table of Contents](#table-of-contents)
    - [Abstract](#abstract)
    - [Motivation](#motivation)
        - [Goals](#goals)
        - [Non-goals](#non-goals)
    - [Proposal](#proposal)
        - [User Stories](#user-stories)
        - [Notes/Constraints/Caveats/Warnings](#notesconstraintscaveatswarnings)
        - [Risks and Mitigations](#risks-and-mitigations)
    - [Sidecar Design](#sidecar-design)
        - [Sidecar Architecture](#sidecar-architecture)
        - [Sidecar Interfaces](#sidecar-interfaces)
        - [Sidecar Injection](#sidecar-injection)
    - [Plugin Design](#plugin-design)
        - [Service Quality Probing](#service-quality-probing)
            - [Implementation Process](#implementation-process)
            - [Discussion Points](#discussion-points)
        - [Hot Updates:](#hot-updates)
            - [Implementation Process](#implementation-process-1)
            - [Discussion Points](#discussion-points-1)
    - [Graduation Criteria](#graduation-criteria)

---

## Abstract

This KEP proposes adding a sidecar to Kruise to address service quality probing and hot updates in serverless scenarios, where some capabilities of Kruise cannot be utilized. Additionally, through a plugin mechanism, this sidecar can integrate solutions for various issues in other serverless scenarios, making it easy to implement different functionalities for similar scenarios by adhering to the provided generic interfaces.

## Motivation
Currently, Kruise provides capabilities for service quality probing and hot updates, but these are implemented within kruise-daemon, which cannot function properly in serverless scenarios. This KEP aims to resolve such issues through sidecar capabilities.

### Goals

- Implement two plugins via sidecar capabilities to solve service quality probing and hot update problems in serverless scenarios.
- Provide a set of generic, extensible plugin management mechanisms to facilitate the addition of plugins for solving similar issues.

### Non-goals

- Integrating existing capabilities supported by Kruise in serverless scenarios into this sidecar.

## Proposal

### User Stories

- **Story 1**: I want to use the custom service quality capabilities provided by Kruise to solve automatic scaling based on player numbers in game services under serverless scenarios.
- **Story 2**: I wish to reload/update certain configurations/resources while pods are running continuously in serverless scenarios, ensuring that pods do not restart.

### Notes/Constraints/Caveats/Warnings

- The sidecar must ensure compatibility with the service quality probing and hot update capabilities provided by kruise-daemon.
- Strictly limit RBAC permissions for plugins within the sidecar to prevent excessive privileges that could affect pod security.

### Risks and Mitigations

- **Risk**: Excessive permissions for the sidecar might pose risks to the main containers of users.
- **Mitigation**: Restrict RBAC permissions within the sidecar.

## Sidecar Design

### Sidecar Architecture
![sidecar-struct](../img/sidecar-struct.png)
1. Inject the sidecar into selected pods using the sidecarset provided by Kruise.
2. Create a ConfigMap to store the configuration required by each plugin within the sidecar. Each plugin can define its own configuration and store it in this ConfigMap.
```yaml
apiVersion: v1
data:
  config.yaml: |
    plugins:
        - name: hot_update
          config:
            bootOrder: 1
            fileDir: /app/downloads
            loadPatchType: signal
            signal:
                processName: 'nginx: master process nginx'
                signalName: SIGHUP
            storageConfig:
                inKube:
                    annotationKey: sidecar.vke.volcengine.com/hot-update-result
                type: InKube
        - name: http_probe
          config:
            startDelaySeconds: 60
            endpoints:
              - url: "http://localhost:8080"               # Target URL
                method: "GET"                           # HTTP Method
                # headers:                               # Request Headers
                #   Content-Type: "application/json"
                #   Authorization: "Bearer your_token"
                timeout: 30                             # Timeout (seconds)
                expectedStatusCode: 200                 # Expected HTTP status code
                storageConfig:                          # Storage Configuration
                  type: InKube
                  inKube:
                  annotationKey: http_probe
                   target:
                       group:  game.kruise.io
                       version: v1alpha1
                       resource: gameservers
                       name: ${SELF:POD_NAME}
                       namespace: ${SELF:POD_NAMESPACE}
                   jsonPath: /spec/opsState
                    markerPolices:
                      - state: idle
                        labels:
                          gameserver-idle: 'true'
                        annotations:
                          controller.kubernetes.io/pod-deletion-cost: '-10'
                      - state: allocated
                        labels:
                          gameserver-idle: 'false'
                        annotations:
                          controller.kubernetes.io/pod-deletion-cost: '10'
                      - state: unknown
                        labels:
                          gameserver-idle: 'false'
                        annotations:
                          controller.kubernetes.io/pod-deletion-cost: '5'
                bootorder: 0
    restartpolicy: Always
    resources:
        CPU: 100m
        Memory: 128Mi
    sidecarstartorder: Before
kind: ConfigMap
metadata:
  name: sidecar-config
  namespace: kube-system

```
3. The sidecar will start and run each plugin according to the plugin configuration in the ConfigMap.
4. After the plugins run, they can persist their results. The sidecar predefines several result storage mechanisms, allowing results to be stored in pod anno/labels or specific locations of custom CRDs; this can be configured in the storageConfig of the plugins in the ConfigMap. Alternatively, plugin developers can implement their own result persistence, storing plugin results as needed.
```yaml 
apiVersion: v1
kind: ConfigMap
metadata:
  name: sidecar-config
  namespace: kube-system
data:
  config.yaml: |
    plugins:
        - name: plugin-a
          config:
            '''
            storageConfig: ## Use storageConfig to declare where to save results
                inKube:
                    annotationKey: xxx ## Set results to the specified anno of the pod
                type: InKube
        - name: plugin-b
          config:
            '''
            storageConfig: 
              type: InKube
              inKube:
               target: # Save results to a specific location of a specified CR
                   group:  game.kruise.io
                   version: v1alpha1
                   resource: gameservers
                   name: ${SELF:POD_NAME}
                   namespace: ${SELF:POD_NAMESPACE}
               jsonPath: /spec/opsState
    
```

### Sidecar Interfaces
The sidecar provides a series of interfaces for unified management of plugins.

| Interface Name                                               | Function                                                   |
|------------------------------------------------------------|-----------------------------------------------------------|
| `AddPlugin(plugin Plugin) error`                              | - Add a specified plugin<br>- Obtain the configuration required by the plugin<br>- Initialize the plugin |
| `RemovePlugin(pluginName string) error`                       | - Remove a specified plugin                                             |
| `GetVersion() string`                                        | - Obtain version number                                                  |
| `PluginStatus(pluginName string) (*PluginStatus, error)`      | - Obtain the status of a specified plugin                                 |
| `Start(ctx context.Context) error`                            | - Start all plugins                                                   |
| `Stop(ctx context.Context) error`                             | - Stop all plugins                                                   |
| `SetupWithManager(mgr SidecarManager) error`                  | - Start the manager                                              |
| `LoadConfig(path string) error`                               | - Load configuration                                                |
| `StoreData(factory StorageFactory, data string) error`        | - Store plugin results                                         |

For the implementation of plugins, a set of interfaces is also designed; new plugins need only implement this set of interfaces.

| Interface Name | Function                      |
|---------------|------------------------------|
| `Name()`      | - Obtain the name of the plugin     |
| `Init()`      | - Initialize the plugin         |
| `Start()`     | - Run the plugin, implementing specific logic |
| `Stop()`      | - Stop the plugin            |
| `Version()`   | - Obtain the version of the plugin  |
| `Status()`    | - Obtain the status of the plugin  |
| `GetConfigType()` | - Obtain the configuration type of the plugin |

### Sidecar Injection
Since the sidecarset supports injecting only containers and not other pod information, service accounts and shared process namespaces need to be injected into the pod when implementing hot updates and service quality probing. A separate KEP has been proposed to enhance this functionality (https://github.com/openkruise/kruise/pull/1820), which has been accepted.

## Plugin Design
### Service Quality Probing
Kruise provides custom Probe capabilities through PodProbeMarker, returning results to the Pod Status, allowing users to decide subsequent actions based on these results. However, this capability is not usable in serverless scenarios. This KEP designs a PodProbe plugin to be implemented in the sidecar to address issues in serverless scenarios.

#### Implementation Process
![pod-probe](../img/sidecar-pod-probe.png)
- Configure the necessary settings for the podProbe plugin in the ConfigMap; the configuration mainly includes:
    - url: The probing address, currently supporting HTTP interface probing
    - method: HTTP request method
    - timeout: HTTP request timeout
    - expectedStatusCode: Expected HTTP status code
    - storageConfig: Configuration for storing probing results, supporting saving results to pod labels/annotations/conditions;
- The plugin probes the specified port cyclically according to the configuration and stores the results.
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sidecar-config
  namespace: kube-system
data:
  config.yaml: |
    plugins:
        - name: http_probe
          config:
            startDelaySeconds: 60
            endpoints:
              - url: "http://localhost:8080"             # Target URL
                method: "GET"                            # HTTP Method
                # headers:                               # Request Headers
                #   Content-Type: "application/json"
                #   Authorization: "Bearer your_token"
                timeout: 30                             # Timeout (seconds)
                expectedStatusCode: 200                 # Expected HTTP status code
                storageConfig:                          # Storage Configuration
                  type: InKube
                  inKube:
                  annotationKey: http_probe
                   target:
                       group:  game.kruise.io
                       version: v1alpha1
                       resource: gameservers
                       name: ${SELF:POD_NAME}
                       namespace: ${SELF:POD_NAMESPACE}
                   jsonPath: /spec/opsState
                    markerPolices:
                      - state: idle
                        labels:
                          gameserver-idle: 'true'
                        annotations:
                          controller.kubernetes.io/pod-deletion-cost: '-10'
                      - state: allocated
                        labels:
                          gameserver-idle: 'false'
                        annotations:
                          controller.kubernetes.io/pod-deletion-cost: '10'
                      - state: unknown
                        labels:
                          gameserver-idle: 'false'
                        annotations:
                          controller.kubernetes.io/pod-deletion-cost: '5'
                bootorder: 0
    restartpolicy: Always
    resources:
        CPU: 100m
        Memory: 128Mi
    sidecarstartorder: Before
```
#### Discussion Points
- Whether the probing performed by the pod probe plugin in the sidecar conflicts with the native podProbeMarker provided by Kruise
    - It does not conflict; during sidecar injection, the required pods are selected separately from those using Kruise's PodProbeMarker.
- Storing results of the pod probe plugin in the sidecar
    - Option One: Store results on the pod (labels/annotations/conditions); the sidecar needs to have patch permissions for the pod.
    - Option Two: Store results in a new Custom Resource (CR), with one CR corresponding to each pod. This method does not require patch permissions for the pod, but it involves managing a separate CR, which is more costly and differs from the existing PodProbeMarker result storage method, making it difficult for users to combine it with PodProbeMarker.

### Hot Updates
During the operation of a pod, it may be necessary to update resources/configuration files within the pod without restarting the pod, achieving a hot update effect. This sidecar implements a hot update plugin.
#### Implementation Process
![sidecar-hot-update](../img/sidecar-hot-update.png)
- Currently, the hot update plugin supports triggering hot updates via signals.
- The configuration for the hot update plugin is as follows:
    - Declare the process name of the main container and the signal name to be sent in the configuration. During a hot update, the sidecar sends the specified signal to the process, triggering the main container to re-read the configuration/resource files.
        - The sidecar is responsible only for sending signals to the process of the main container; the main container itself must implement the reloading of configurations.
- When a hot update is required, users send an HTTP request to the sidecar's port within the pod, specifying the path and version of the file to be hot updated.
- Based on the file path in the user's request, the sidecar downloads the file from a remote location and saves it to a designated directory within the sidecar. This directory is mounted using an emptyDir, and the main container also mounts this volume. This allows the main container to access the new hot update file and be triggered to reload it.
- After the hot update is completed, the sidecar stores the hot update results in the pod's annotations/labels and persists them in a specified ConfigMap (sidecar-result);
    - This ConfigMap stores the hot update results for all pods, including the latest version and the file location of the latest version configuration. This ensures that the latest configuration can still be obtained after a pod restarts or scales up.
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sidecar-config
  namespace: kube-system
data:
  config.yaml: |
    plugins:
        - name: hot_update
          config:
            fileDir: /app/downloads # Directory for storing hot update files in the sidecar
            loadPatchType: signal # Hot update method
            signal:
                processName: 'nginx: master process nginx' # Process name of the main container to receive the signal
                signalName: SIGHUP # Signal
            storageConfig: # Result storage configuration
                inKube:
                    annotationKey: sidecar.vke.volcengine.com/hot-update-result
                type: InKube
    restartpolicy: Always
    resources:
        CPU: 100m
        Memory: 128Mi
    sidecarstartorder: Before
```
#### Discussion Points
- Methods for triggering hot updates
    - Currently, hot updates are triggered by users sending HTTP requests to the sidecar's port of the specified pod, with the HTTP request carrying the address and version number of the hot update configuration file.
    - Other methods for triggering hot updates can also be considered, such as using ConfigMapSets to write new configuration file addresses and generating a ConfigMap for each version of the configuration information. The sidecar can trigger a hot update by listening for changes to this ConfigMap.

## Graduation Criteria
+ Alpha: The sidecar can manage plugins normally, and the hot update/service quality probing plugins operate correctly. Documentation for using the sidecar is complete.
+ Beta: Stability improvements and validation testing for extensions.
+ Stable: Comprehensive user feedback has been incorporated, and the features are ready for general use.
