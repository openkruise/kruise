# CHANGE LOGS

# v0.1.0
### Kruise-controller-manager
#### Features
- Support to run kruise-controller-manager locally
- Allow selectively install required CRDs for kruise controllers
#### Bugs
- Remove `sideEffects` in kruise-manager all-in-one YAML file to avoid start failure

### Advanced Statefulset
#### Features
- Add MaxUnavailable Rolling Update Strategy 
- Add In-Place pod update strategy

### Broadcast Job
#### Features
- Add BroadcastJob that runs pods on all nodes to completion
- Add `Never` termination policy to have jobs running after it finishes all pods
- Add `ttlSecondsAfterFinished` to delete the job after it finishes in x seconds.

#### Bugs
- Make broadcastjob hornor node unschedulable condition

### Sidecar Set
#### Features
- Add SidecarSet that automatically injects sidecar container into selected pods
- Support sidecar update functionality for SidecarSet
