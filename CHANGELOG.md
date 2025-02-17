# Change Log

## v1.8.0

> Change log since v1.7.3
### Upgrade Notice
> No, really, you must read this before you upgrade
- **Disable** the following feature gates by default: ResourcesDeletionProtection ([#1919](https://github.com/openkruise/kruise/pull/1919), [@ABNER-1](https://github.com/ABNER-1))
- Promote these feature gates to beta:  
  `ResourcesDeletionProtection`, `WorkloadSpread`, `PodUnavailableBudgetDeleteGate`, `InPlaceUpdateEnvFromMetadata`,
  `StatefulSetAutoDeletePVC`,
  `PodProbeMarkerGate` ([#1919](https://github.com/openkruise/kruise/pull/1919), [@ABNER-1](https://github.com/ABNER-1))
- Update Kubernetes dependency to v1.30.10 and Golang to v1.22 ([#1896](https://github.com/openkruise/kruise/pull/1896), [@ABNER-1](https://github.com/ABNER-1), [#1924](https://github.com/openkruise/kruise/pull/1924), [@furykerry](https://github.com/furykerry)))
- Prior to Kruise 1.7.3, `helm uninstall` is a **high-risk** operation that deletes Kruise, its CRDs, and associated CRs. Starting from Kruise 1.7.3, it uses a pre-delete hook to check for existing Kruise CRs before uninstallation and blocks the process to prevent accidental deletion.

### Key Features
- Support in-place expansion of StatefulSet volumes ([#1674](https://github.com/openkruise/kruise/pull/1674), [#1714](https://github.com/openkruise/kruise/pull/1714), [@ABNER-1](https://github.com/ABNER-1))
- Enable in-place resource resizing for CloneSet, Advanced StatefulSet, and Advanced DaemonSet ([#1353](https://github.com/openkruise/kruise/pull/1353), [#1866](https://github.com/openkruise/kruise/pull/1866), [@LavenderQAQ](https://github.com/LavenderQAQ), [@ABNER-1](https://github.com/ABNER-1))
- Support adaptive scheduling strategy for UnitedDeployment ([#1720](https://github.com/openkruise/kruise/pull/1720),  [@AiRanthem](https://github.com/AiRanthem))
- Add WorkloadSpread support for AI workload like TFJob in KubeFlow ([#1838](https://github.com/openkruise/kruise/pull/1838), [@AiRanthem](https://github.com/AiRanthem))

### Performance Improvements
- Optimize CA bundle updates to reduce unnecessary changes ([#1717](https://github.com/openkruise/kruise/pull/1717), [@zmberg](https://github.com/zmberg))
- Add disableDeepCopy for BroadcastJob ([#1696](https://github.com/openkruise/kruise/pull/1696), [@Prepmachine4](https://github.com/Prepmachine4))

### Resilience Enhancement
- Add Helm pre-delete hook to preserve Kruise CRs during uninstallation ([#1843](https://github.com/openkruise/kruise/pull/1843), [@AiRanthem](https://github.com/AiRanthem))

### Other Notable Changes
#### Advanced Workload
- Add lifecycle hooks and tests for Advanced StatefulSet ([#1858](https://github.com/openkruise/kruise/pull/1858), [@mingzhou.swx](https://github.com/mingzhou.swx), [@ABNER-1](https://github.com/ABNER-1))
- Add range-based reserveOrdinals support for Advanced StatefulSet ([#1873](https://github.com/openkruise/kruise/pull/1873), [@AiRanthem](https://github.com/AiRanthem))
- Redefined partition semantics to represent non-updated pod count ([#1819](https://github.com/openkruise/kruise/pull/1819), [@ABNER-1](https://github.com/ABNER-1); [#1751](https://github.com/openkruise/kruise/pull/1751), [@zybtakeit](https://github.com/zybtakeit), [@ABNER-1](https://github.com/ABNER-1))

#### Sidecar Management
- Support inject both stable and updated version sidecar according to updateStrategy ([#1689](https://github.com/openkruise/kruise/pull/1689), [#1856](https://github.com/openkruise/kruise/pull/1856), [@AiRanthem](https://github.com/AiRanthem)) 
- Refine SidecarSet initContainer handling ([#1719](https://github.com/openkruise/kruise/pull/1719), [@zmberg](https://github.com/zmberg))

#### Multi-domain management
- Introduce `pub.kruise.io/disable-fetch-replicas-from-workload=true` annotation for CRD compatibility ([#1758](https://github.com/openkruise/kruise/pull/1758), [@zmberg](https://github.com/zmberg))
- Extend PodProbeMarker to serverless pods ([#1875](https://github.com/openkruise/kruise/pull/1875), [@zmberg](https://github.com/zmberg))
- Enable priorityClassName patching in WorkloadSpread ([#1877](https://github.com/openkruise/kruise/pull/1877), [@AiRanthem](https://github.com/AiRanthem))
- Sync all fields in UnitedDeployment spec to subset workload spec ([#1798](https://github.com/openkruise/kruise/pull/1798), [@AiRanthem](https://github.com/AiRanthem))

### Bug Fixes
- Resolve token permission and dependency pinning issues ([#1707](https://github.com/openkruise/kruise/pull/1707),  [@harshitasao](https://github.com/harshitasao))
- Fix PyTorchJob pod creation failures ([#1864](https://github.com/openkruise/kruise/pull/1864), [@zmberg](https://github.com/zmberg))
- Correct ImagePullJob timeout handling (>1800s) ([#1874](https://github.com/openkruise/kruise/pull/1874), [@zmberg](https://github.com/zmberg))
- Resolve cri-dockerd runtime detection issues ([#1899](https://github.com/openkruise/kruise/pull/1899), [@FlikweertvisionVadym](https://github.com/FlikweertvisionVadym))
- Remove pod ownerRef requirement in pub webhook ([#1869](https://github.com/openkruise/kruise/pull/1869), [@zmberg](https://github.com/zmberg))
- Address maxUnavailable blocking in SidecarSet updates ([#1834](https://github.com/openkruise/kruise/pull/1834), [@zmberg](https://github.com/zmberg))
- Fix CloneSet controller block from scale expectation leaks ([#1829](https://github.com/openkruise/kruise/pull/1829), [@zmberg](https://github.com/zmberg))
- Enforce imagePullPolicy=Always for ImagePullJob ([#1830](https://github.com/openkruise/kruise/pull/1830), [@zmberg](https://github.com/zmberg))
- Fix WorkloadSpread webhook panics ([#1807](https://github.com/openkruise/kruise/pull/1807), [@AiRanthem](https://github.com/AiRanthem))

### Misc (Chores and tests)
- Standardize on CRI for image pulls ([#1867](https://github.com/openkruise/kruise/pull/1867), [@furykerry](https://github.com/furykerry))
- Introduce JSON log formatting ([#1703](https://github.com/openkruise/kruise/pull/1703), [@zmberg](https://github.com/zmberg))
- Remove Docker runtime dependency ([#1870](https://github.com/openkruise/kruise/pull/1870),[@furykerry](https://github.com/furykerry))
- Improve test parallelism and reliability ([#1743](https://github.com/openkruise/kruise/pull/1743), [@MichaelRren](https://github.com/MichaelRren))
- Enhance WorkloadSpread validation logic ([#1740](https://github.com/openkruise/kruise/pull/1740), [@AiRanthem](https://github.com/AiRanthem))
- Launche Kruise Guru on Gurubase.io ([#1800](https://github.com/openkruise/kruise/pull/1800), [@kursataktas](https://github.com/kursataktas))
- Improve documentation accuracy ([#1824](https://github.com/openkruise/kruise/pull/1824), [@furykerry](https://github.com/furykerry))
- Fix KIND installation issues ([#1688](https://github.com/openkruise/kruise/pull/1688),[@ABNER-1](https://github.com/ABNER-1))
- Avoid overriding namespace config after deploying ([#1772](https://github.com/openkruise/kruise/pull/1772),[@hantmac](https://github.com/hantmac))
- Fix WorkloadSpread test flakiness by removing dependencies ([#1895](https://github.com/openkruise/kruise/pull/1895), [@AiRanthem](https://github.com/AiRanthem))
- Address SidecarSet e2e test failures ([#1724](https://github.com/openkruise/kruise/pull/1724), [@zmberg](https://github.com/zmberg))
- Enhance unit test stability ([#1784](https://github.com/openkruise/kruise/pull/1784), [@AiRanthem](https://github.com/AiRanthem))

## v1.7.3
> Change log since v1.7.2

### Bug fixes
- Fix kubeflow PyTorchJob create pod failure due to pod webhook. ([#1734](https://github.com/openkruise/kruise/pull/1864), [@zmberg](https://github.com/zmberg))

## v1.7.2
> Change log since v1.7.1

### Advanced Workload
- Support specified-delete in AdvancedStatefulSet and handle specified deleted pod under maxUnavailable constrain. ([#1734](https://github.com/openkruise/kruise/pull/1734), [@ABNER-1](https://github.com/ABNER-1))

## v1.6.4
> Change log since v1.6.3

### Advanced Workload
- Support specified-delete in AdvancedStatefulSet and handle specified deleted pod under maxUnavailable constrain. ([#1734](https://github.com/openkruise/kruise/pull/1734), [@ABNER-1](https://github.com/ABNER-1))

## v1.5.5
> Chang log since v1.5.4

### Advanced Workload
- Support specified-delete in AdvancedStatefulSet and handle specified deleted pod under maxUnavailable constrain. ([#1734](https://github.com/openkruise/kruise/pull/1734), [@ABNER-1](https://github.com/ABNER-1))
- Advanced StatefulSet maxUnavailable now counts unavailable pods with smaller ordinal in the update order during rolling upgrade. ([#1480](https://github.com/openkruise/kruise/pull/1480), [@Yesphet](https://github.com/Yesphet))

## v1.7.1
> Change log since v1.7.0

### Bug fixes
- When update crd webhook caBundle, if caBundle does not change, do not update crd again. ([#1717](https://github.com/openkruise/kruise/pull/1717), [@zmberg](https://github.com/zmberg))
- Remove normal init container in pod's sidecarSet in-place update annotation. ([#1719](https://github.com/openkruise/kruise/pull/1719), [@zmberg](https://github.com/zmberg))

## v1.7.0
> Change log since v1.6.3

### Key Features
- When CloneSet volumeClaimTemplates changed, always recreate pods and related volumes. ([#1561](https://github.com/openkruise/kruise/pull/1561), [@ABNER-1](https://github.com/ABNER-1))
- Bump K8s dependency to 1.28, and OpenKruise still works with Kubernetes Version >= 1.18. ([#1598](https://github.com/openkruise/kruise/pull/1598), [@ABNER-1](https://github.com/ABNER-1))
- SidecarSet support k8s 1.28 Sidecar Containers(initContainers[x].restartPolicy=Always), and significantly improves the lifecycle management of Sidecar containers,
  refer to the [community documentation](https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/) for details. ([#1613](https://github.com/openkruise/kruise/pull/1613), [@zmberg](https://github.com/zmberg))
- ImagePullJob support for credential provider plugin, e.g. aws. ([#1383](https://github.com/openkruise/kruise/pull/1383), [@Kuromesi](https://github.com/Kuromesi))
- Advanced StatefulSet support [start ordinal](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#start-ordinal). ([#1643](https://github.com/openkruise/kruise/pull/1643), [@ABNER-1](https://github.com/ABNER-1))
- Support webhook CA injection using external certification management tool, e.g. [cert-manager](https://cert-manager.io/). ([#1665](https://github.com/openkruise/kruise/pull/1665), [@Kuromesi](https://github.com/Kuromesi))
- Kruise-daemon support cri-docker.sock for kubernetes clusters that use docker runtime. ([#1631](https://github.com/openkruise/kruise/pull/1631), [@BraceCY](https://github.com/BraceCY))
- Advanced StatefulSet add pod index label `statefulset.kubernetes.io/pod-index`. ([#1667](https://github.com/openkruise/kruise/pull/1667), [@cr7258](https://github.com/cr7258))
- Add Structured logging support. ([#1565](https://github.com/openkruise/kruise/pull/1565), [@MajLuu](https://github.com/MajLuu)); ([#1629](https://github.com/openkruise/kruise/pull/1629), [@jairuigou](https://github.com/jairuigou)); ([#1669](https://github.com/openkruise/kruise/pull/1669), [@AiRanthem](https://github.com/AiRanthem))

### Performance Enhancement
- Optimizing Pod SidecarSet webhook and controller performance when lots of namespace scoped sidecarSet exists ([#1547](https://github.com/openkruise/kruise/pull/1547), [@ls-2018](https://github.com/ls-2018))
- Pod readiness controller use Patch instead of Update, thus reducing updating conflict when creating a large number of Pods. ([#1560](https://github.com/openkruise/kruise/pull/1560), [@BruceAko](https://github.com/BruceAko))

### Bug fixes
- Multi-domain Management
  - Fixes workloadSpread validation message when using adaptive strategy type. ([#1553](https://github.com/openkruise/kruise/pull/1553), [@voron](https://github.com/voron))
  - When feature-gate WorkloadSpread=false, the WorkloadSpread Controller is no longer started and the creation of workloadSpread CR is not allowed. ([#1566](https://github.com/openkruise/kruise/pull/1566), [@ls-2018](https://github.com/ls-2018))
- Application Protection
  - In some extreme scenarios, fix PodUnavailableBudget blocking KCM recycling Pods. ([#1567](https://github.com/openkruise/kruise/pull/1567), [@Spground](https://github.com/Spground))
- Sidecar Container
  - Fix SidecarSet invalid update status. ([#1641](https://github.com/openkruise/kruise/pull/1641), [@Spground](https://github.com/Spground))
- Advanced Workload
  - Fix potential nil panic in CloneSet validating webhook when Pod's controller owner ref is nil. ([#1678](https://github.com/openkruise/kruise/pull/1678), [@Spground](https://github.com/Spground))

### Misc (cleanup and Flake)
- Optimized Advanced StatefulSet code structure based on upstream community code(k8s 1.28). ([#1648](https://github.com/openkruise/kruise/pull/1648), [@ABNER-1](https://github.com/ABNER-1))
- Reduce github workflow action permission. ([#1523](https://github.com/openkruise/kruise/pull/1523), [@furykerry](https://github.com/furykerry))
- Bug fix for Makefile envtest failed. ([#1548](https://github.com/openkruise/kruise/pull/1548), [@BH4AWS](https://github.com/BH4AWS))
- Fix UT TestRevisionManage. ([#1555](https://github.com/openkruise/kruise/pull/1555), [@furykerry](https://github.com/furykerry))
- Upgrade opencontainers/runc (1.1.12) and controller-gen (0.14.0). ([#1562](https://github.com/openkruise/kruise/pull/1562), [@ppbits](https://github.com/ppbits))
- Remove vendor directory. ([#1554](https://github.com/openkruise/kruise/pull/1554), [@liangyuanpeng](https://github.com/liangyuanpeng))
- Add dependabot config for auto-update github-actions. ([#1570](https://github.com/openkruise/kruise/pull/1570), [@liangyuanpeng](https://github.com/liangyuanpeng))
- Add permission of security-events write for ghaction golangci-lint. ([#1582](https://github.com/openkruise/kruise/pull/1582), [@liangyuanpeng](https://github.com/liangyuanpeng))
- Fix vendor error while running command make docker-multiarch. ([#1601](https://github.com/openkruise/kruise/pull/1601), [@MichaelRren](https://github.com/MichaelRren))
- Change e2e centos image from 6.7 to 7, then e2e can work on arm node. ([#1623](https://github.com/openkruise/kruise/pull/1623), [@Colvin-Y](https://github.com/Colvin-Y))
- Fix slice declarations that are not initialized with zero length. ([#1628](https://github.com/openkruise/kruise/pull/1628), [@alingse](https://github.com/alingse))
- Fix UT TestMatchRegistryAuths failed. ([#1583](https://github.com/openkruise/kruise/pull/1583), [@ABNER-1](https://github.com/ABNER-1))
- Changes the scorecard badge link from old format to the Standard human-readable OpenSSF Scorecard Report. ([#1657](https://github.com/openkruise/kruise/pull/1657), [@harshitasao](https://github.com/harshitasao))

## v1.6.3
> Change log since v1.6.2

### CVE FIX
- fix potential security issues of dependent packages ([#1586](https://github.com/openkruise/kruise/pull/1586), [ABNER-1](https://github.com/ABNER-1)) ([#1591](https://github.com/openkruise/kruise/pull/1591), [ABNER-1](https://github.com/ABNER-1))

## v1.6.2
> Change log since v1.6.1

### CloneSet
- Fix new version of Pods released by cloneSet that doesn't match spec.updateStrategy.partition. ([#1549](https://github.com/openkruise/kruise/pull/1549), [@qswksp](https://github.com/qswksp))

## v1.5.4
> Chang log since v1.5.3

### CloneSet
- Fix new version of Pods released by cloneSet that doesn't match spec.updateStrategy.partition. ([#1549](https://github.com/openkruise/kruise/pull/1549), [@qswksp](https://github.com/qswksp))

## v1.4.2
> Change log since v1.4.1

### CloneSet
- Fix new version of Pods released by cloneSet that doesn't match spec.updateStrategy.partition. ([#1549](https://github.com/openkruise/kruise/pull/1549), [@qswksp](https://github.com/qswksp))

## v1.6.1
> Change log since v1.6.0

### Upgrade Notice
- FeatureGate PodWebhook=false will not disable ResourcesDeletionProtection. ([#1526](https://github.com/openkruise/kruise/pull/1526), [@zmberg](https://github.com/zmberg))
- Update go.mod require k8s version from 1.29 to 1.26, and remove go mod replace. ([#1527](https://github.com/openkruise/kruise/pull/1527), [KaiShi](https://github.com/BH4AWS))

### Advanced Workload
- Fix when StatefulSet reserveOrdinals exist and whenScaled=Delete, scale down pvc failed. ([#1531](https://github.com/openkruise/kruise/pull/1531), [@zmberg](https://github.com/zmberg))

## v1.5.3
> Chang log since v1.5.2

### Advanced Workload
- Fix when StatefulSet reserveOrdinals exist and whenScaled=Delete, scale down pvc failed. ([#1531](https://github.com/openkruise/kruise/pull/1531), [@zmberg](https://github.com/zmberg))

## v1.6.0
> Change log since v1.5.2

### Upgrade Notice

> No, really, you must read this before you upgrade
- OpenKruise no longer supports Kubernetes versions 1.16, 1.17.
  However it's still possible to use OpenKruise with Kubernetes versions 1.16 and 1.17 as long as KruiseDaemon is not enabled(install/upgrade kruise charts with featureGates="KruiseDaemon=false")
- Kruise-Daemon will no longer support v1alpha2 CRI runtimes.
  However it's still possible to use OpenKruise on Kubernetes with nodes that only support v1alpha2 CRI as long as KruiseDaemon is not enabled(install/upgrade kruise charts with featureGates="KruiseDaemon=false")
- OpenKruise leader election default to use leases mode. ([#1407](https://github.com/openkruise/kruise/pull/1407), [dsxing](https://github.com/dsxing))
  For users with OpenKruise version 1.3.0 or lower, please first upgrade your OpenKruise to version 1.4 or 1.5 before upgrading to 1.6.0, so as to avoid unexpected multiple leader problem during the installation.
- Bump Kubernetes dependency to 1.26.10. ([#1511](https://github.com/openkruise/kruise/pull/1511), [KaiShi](https://github.com/BH4AWS))
- To avoid potential circular dependency problem, features rely on webhook will no longer work for resources under kube-system,
  e.g. SidecarSet, WorkloadSpread, PodUnavailableBudget, ContainerLaunchPriority and PersistentPodState. ([#92](https://github.com/openkruise/charts/pull/92), [@hantmac](https://github.com/hantmac))

### Key Features
- Fix WorkloadSpread incorrect subset allocation after workload rolling updating. ([#1197](https://github.com/openkruise/kruise/pull/1197), [veophi](https://github.com/veophi))
- ImagePullJob support force image pulling for images with the name as previous one. ([#1384](https://github.com/openkruise/kruise/pull/1384), [ls-2018](https://github.com/ls-2018))
- Job Sidecar Terminator reports correct pod phase for sidecar containers with non-zero exit code. ([#1303](https://github.com/openkruise/kruise/pull/1303), [@diannaowa](https://github.com/diannaowa))
- Support the deletion protection of service and ingress resources. ([#1269](https://github.com/openkruise/kruise/pull/1269), [@kevin1689-cloud](https://github.com/kevin1689-cloud))

### Performance Enhancement
- Optimize PodProbeMarker performance. ([#1430](https://github.com/openkruise/kruise/pull/1430), [ls-2018](https://github.com/ls-2018))
- Optimize container launch priority performance. ([#1490](https://github.com/openkruise/kruise/pull/1490), [FillZpp](https://github.com/FillZpp))

### Other Changes

- Enhanced Operation
  - PodProbeMarker:  Container probe support Tcp probing. ([#1474](https://github.com/openkruise/kruise/pull/1474), [KaiShi](https://github.com/BH4AWS))
  - PodProbeMarker:  Sync podCondition when probe message of probeStates changed. ([#1479](https://github.com/openkruise/kruise/pull/1479), [chrisliu1995](https://github.com/chrisliu1995))
  - PersistentPodState: Fix the problem that PersistentPodState can't get spec.replicas from unstructured object. ([#1462](https://github.com/openkruise/kruise/pull/1462), [0xgj](https://github.com/0xgj))
  - Fix PodProbeMarker feature gate dependency . ([#1429](https://github.com/openkruise/kruise/pull/1429), [ls-2018](https://github.com/ls-2018))

- Advanced Workload
  - Enforce Advanced DaemonSet spec.selector is immutable. ([#1505](https://github.com/openkruise/kruise/pull/1505), [@hantmac](https://github.com/hantmac))
  - Advanced StatefulSet maxUnavailable now counts unavailable pods with smaller ordinal in the update order during rolling upgrade. ([#1480](https://github.com/openkruise/kruise/pull/1480), [@Yesphet](https://github.com/Yesphet))
  - Fix EphemeralJob event handler for deleting object. ([#1401](https://github.com/openkruise/kruise/pull/1401), [FillZpp](https://github.com/FillZpp))

- Sidecar Container
  - Fix pod annotations injection abnormal for SidecarSet. ([#1453](https://github.com/openkruise/kruise/pull/1453), [@a932846905](https://github.com/a932846905))

- Application Protection
  - PodUnavailableBudget ignore deletion of not ready or inconsistent pods. ([#1512](https://github.com/openkruise/kruise/pull/1512), [Spground](https://github.com/Spground))

- Others
  - Replace 'github.com/pkg/errors' with the standard Go library 'errors'. ([#1518](https://github.com/openkruise/kruise/pull/1518), [dongjiang1989](https://github.com/dongjiang1989))
  - Upgrade minimum docker api version from 1.23 to 1.24. ([#1510](https://github.com/openkruise/kruise/pull/1510), [hantmac](https://github.com/hantmac))
  - Add UT in controller_revision_test file. ([#1457](https://github.com/openkruise/kruise/pull/1457), [xiangpingjiang](https://github.com/xiangpingjiang))
  - BroadcastJob controller define some parameters as Constant. ([#1414](https://github.com/openkruise/kruise/pull/1414), [lilongfeng0902](https://github.com/lilongfeng0902))
  - Kruise-daemon enable pprof. ([#1416](https://github.com/openkruise/kruise/pull/1416), [dsxing](https://github.com/dsxing))
  - Remove deprecated 'io/ioutil' pkg. ([#1404](https://github.com/openkruise/kruise/pull/1404), [testwill](https://github.com/testwill))
  - Fix unnecessary use of fmt.Sprintf. ([#1403](https://github.com/openkruise/kruise/pull/1403), [testwill](https://github.com/testwill))

## v1.5.2
> Chang log since v1.5.1

### CVE FIX: Enhance kruise-daemon security ([#1482](https://github.com/openkruise/kruise/pull/1482), [veophi](https://github.com/veophi))

### Start kruise-manager as a non-root user
We start kruise-manger with a non-root user to further enhance the security of kruise-manager. ([#1491](https://github.com/openkruise/kruise/pull/1491), [@zmberg](https://github.com/zmberg))

## v1.5.1
> Chang log since v1.5.0

In version 1.5.1, the focus was on enhancing UnitedDeployment and addressing various bug fixes:

- Add the ability to plan the lower and upper bound of capacity to the subsets in UnitedDeployment ([#1428](https://github.com/openkruise/kruise/pull/1428), [@veophi](https://github.com/veophi))

- Fix unexpected job recreation by adding controller-revision-hash label for ImageListPullJob. ([#1441](https://github.com/openkruise/kruise/pull/1428), [@veophi](https://github.com/veophi))

- Add prometheus metrics for pub and deletion protection to enhance observability for pub & deletion protection ([#1398](https://github.com/openkruise/kruise/pull/1398), [@zmberg](https://github.com/zmberg))

- Add enable pprof flag for kruise daemon, now you can disable the pprof of kruise daemon ([#1416](https://github.com/openkruise/kruise/pull/1416), [@chengjoey](https://github.com/chengjoey))

- Fix SidecarSet upgrade exception for UpdateExpectations to solve the problem of updating the image of the sidecar container ([#1435](https://github.com/openkruise/kruise/pull/1435), [@zmberg](https://github.com/zmberg)])

- add audit log for pub and deletion protection to enhance observability for pub & deletion protection  ([#1438](https://github.com/openkruise/kruise/pull/1438), [@zmberg](https://github.com/zmberg)])

## v1.5.0
> Change log since v1.4.0

### Upgrade Notice

> No, really, you must read this before you upgrade

- **Disable** following feature-gates by default: PreDownloadImageForInPlaceUpdate([#1244](https://github.com/openkruise/kruise/pull/1224), [@zmberg](https://github.com/zmberg)), ImagePullJobGate([#1357](https://github.com/openkruise/kruise/pull/1357), [@zmberg](https://github.com/zmberg)), DeletionProtectionForCRDCascadingGate([#1365](https://github.com/openkruise/kruise/pull/1365), [@zmberg](https://github.com/zmberg)), and ResourceDistributionGate([#1360](https://github.com/openkruise/kruise/pull/1360/files), [@zmberg](https://github.com/zmberg))
- Bump Kubernetes dependency to 1.24.16, Golang version to 1.19([#1354](https://github.com/openkruise/kruise/pull/1354), [Kuromesi](https://github.com/Kuromesi))

### Key Features: Enhanced Multi-Domain Management
- WorkloadSpread:
  - Support any customized workloads that have `scale` sub-resource. ([#1286](https://github.com/openkruise/kruise/pull/1286), [veophi](https://github.com/veophi))
  - Add validation for subset patch field. ([#1237](https://github.com/openkruise/kruise/pull/1237), [chengleqi](https://github.com/chengleqi))
- UnitedDeployment:
  - Support `scale` sub-resource. ([#1314](https://github.com/openkruise/kruise/pull/1314)), [diannaowa](https://github.com/diannaowa))
  - Support `patch` field for each subset. ([#1266](https://github.com/openkruise/kruise/pull/1266), [chengleqi](https://github.com/chengleqi))
  - Optimize UnitedDeployment replicas settings. ([#1247](https://github.com/openkruise/kruise/pull/1247), [y-ykcir](https://github.com/y-ykcir))

### ImagePreDownload
- ImageListPullJob:
  - Many users have the need for batch pre-download images, and the current approach, i.e., ImagePullJob, has a relatively high threshold for use, We added a new CRD ImageListPullJob to batch pre-download images.
    You just write a range of images in one ImageListPullJob CR, its controller will generate corresponding ImagePullJob CR for each image automatically. ([1222](https://github.com/openkruise/kruise/pull/1222), [@diannaowa](https://github.com/diannaowa))
- ImagePullJob:
  - Fix the matching logic for the imagePullSecret in ImagePullJob. ([#1241](https://github.com/openkruise/kruise/pull/1241), [#1357](https://github.com/openkruise/kruise/pull/1357))
  - Advanced Workload pre-download image support attach metadata in ImagePullJob. ([#1246](https://github.com/openkruise/kruise/pull/1246), [YTGhost](https://github.com/YTGhost))

### Advanced Workload
- SidecarSet:
  - Add condition and event for not upgradable pods when updating. ([#1309](https://github.com/openkruise/kruise/pull/1309), [MarkLux](https://github.com/MarkLux))
  - Take effect of shareVolumePolicy on initContainers. ([#1229](https://github.com/openkruise/kruise/pull/1229), [y-ykcir](https://github.com/y-ykcir))
  - Allow sidecar containers to mount serviceAccountToken type volume. ([#1238](https://github.com/openkruise/kruise/pull/1238), [y-ykcir](https://github.com/y-ykcir))
  - SidecarSet updateStrategy support priorityStrategy. ([#1325](https://github.com/openkruise/kruise/pull/1325), [y-ykcir](https://github.com/y-ykcir))
- BroadcastJob:
  - Make OnFailure as default restartPolicy. ([#1149](https://github.com/openkruise/kruise/pull/1149), [Shubhamurkade](https://github.com/Shubhamurkade))
  - Fix BroadcastJob doesn't make pod on node that has erased taint. ([#1204](https://github.com/openkruise/kruise/pull/1204), [weldonlwz](https://github.com/weldonlwz))
- CloneSet & StatefulSet:
  - Regard the pod at preparing update state as update revision when scaling. ([#1290](https://github.com/openkruise/kruise/pull/1290), [veophi](https://github.com/veophi))
  - Add `updatedAvailableReplicas` field in status. ([#1317](https://github.com/openkruise/kruise/pull/1317), [nitishchauhan0022](https://github.com/nitishchauhan0022))

### Kruise Daemon
- Connecting to Pouch runtime via CRI interface. ([#1232](https://github.com/openkruise/kruise/pull/1232), [@zmberg](https://github.com/zmberg))
- Compatible with v1 and v1alpha2 CRI API version. ([#1354](https://github.com/openkruise/kruise/pull/1354), [veophi](https://github.com/veophi))

### ResourceProtection
- Reject Namespace deletion when PVCs are included under NS. ([#1228](https://github.com/openkruise/kruise/pull/1228), [kevin1689-cloud](https://github.com/kevin1689-cloud))

And some bugs were fixed by
([#1238](https://github.com/openkruise/kruise/pull/1238), [y-ykcir](https://github.com/y-ykcir)),
([#1335](https://github.com/openkruise/kruise/pull/1335), [ls-2018](https://github.com/ls-2018)),
([#1301](https://github.com/openkruise/kruise/pull/1301), [wangwu50](https://github.com/wangwu50)),
([#1395](https://github.com/openkruise/kruise/pull/1301), [ywdxz](https://github.com/ywdxz)),
([#1304](https://github.com/openkruise/kruise/pull/1304), [kevin1689-cloud](https://github.com/kevin1689-cloud)),
([#1348](https://github.com/openkruise/kruise/pull/1348), [#1343](https://github.com/openkruise/kruise/pull/1343), [Colvin-Y](https://github.com/Colvin-Y)),
thanks!

## v1.4.1
> Change log since v1.4.0

### CVE FIX: Enhance kruise-daemon security ([#1482](https://github.com/openkruise/kruise/pull/1482), [veophi](https://github.com/veophi))

## v1.4.0

> Change log since v1.3.0

### Upgrade Notice

> No, really, you must read this before you upgrade

- Enable following feature-gates by default: ResourcesDeletionProtection, WorkloadSpread, PodUnavailableBudgetDeleteGate, InPlaceUpdateEnvFromMetadata,
  StatefulSetAutoDeletePVC, PodProbeMarkerGate. ([#1214](https://github.com/openkruise/kruise/pull/1214), [@zmberg](https://github.com/zmberg))
- Change Kruise leader election from configmap to configmapsleases, this is a smooth upgrade with no disruption to OpenKruise service.  ([#1184](https://github.com/openkruise/kruise/pull/1184), [@YTGhost](https://github.com/YTGhost))

### New Feature: JobSidecarTerminator

In the Kubernetes world, it is challenging to use long-running sidecar containers for short-term job because there is no straightforward way to
terminate the sidecar containers when the main container exits. For instance, when the main container in a Pod finishes its task and exits, it is expected that accompanying sidecars,
such as a log collection sidecar, will also exit actively, so the Job Controller can accurately determine the completion status of the Pod.
However most sidecar containers lack the ability to discovery the exit of main container.

For this scenario OpenKruise provides the JobSidecarTerminator capability, which can terminate sidecar containers once the main containers exit.

For more detail, please refer to its [proposal](https://github.com/openkruise/kruise/blob/master/docs/proposals/20230130-job-sidecar-terminator.md).

### Advanced Workloads
- Optimized CloneSet Event handler to reduce unnecessary reconciliation. The feature is off by default and controlled by CloneSetEventHandlerOptimization feature-gate. ([#1219](https://github.com/openkruise/kruise/pull/1219), [@veophi](https://github.com/veophi))
- Avoid pod hang in PreparingUpdate state when rollback before update hook. ([#1157](https://github.com/openkruise/kruise/pull/1157), [@shiyan2016](https://github.com/shiyan2016))
- Fix cloneSet update blocking when spec.scaleStrategy.maxUnavailable is not empty. ([#1136](https://github.com/openkruise/kruise/pull/1136), [@ivanszl](https://github.com/ivanszl))
- Add 'disablePVCReuse' field to enable recreation of PVCs when rebuilding pods, which can avoid Pod creation failure due to Node exceptions. ([#1113](https://github.com/openkruise/kruise/pull/1113), [@willise](https://github.com/willise))
- CloneSet 'partition' field support float percent to improve precision. ([#1124](https://github.com/openkruise/kruise/pull/1124), [@shiyan2016](https://github.com/shiyan2016))
- Add PreNormal Lifecycle Hook for CloneSet. ([#1071](https://github.com/openkruise/kruise/pull/1071), [@veophi](https://github.com/veophi))
- Allow to mutate PVCTemplate of Advanced StatefulSet & CloneSet. Note, Only works for new Pods, not for existing Pods. ([#1118](https://github.com/openkruise/kruise/pull/1118), [@veophi](https://github.com/veophi))
- Make ephemeralJob compatible with k8s version 1.20 & 1.21. ([#1127](https://github.com/openkruise/kruise/pull/1127), [@veophi](https://github.com/veophi))
- UnitedDeployment support advanced StatefulSet 'persistentVolumeClaimRetentionPolicy' field. ([#1110](https://github.com/openkruise/kruise/pull/1110), [@yuexian1234](https://github.com/yuexian1234))

### ContainerRecreateRequest
- Add 'forceRecreate' field to ensure the immediate recreation of the container even if the container is starting at that point. ([#1182](https://github.com/openkruise/kruise/pull/1182), [@BH4AWS](https://github.com/BH4AWS))

### ImagePullJob
- Support attach metadata in PullImage CRI interface during ImagePullJob. ([#1190](https://github.com/openkruise/kruise/pull/1190), [@diannaowa](https://github.com/diannaowa))

### SidecarSet
- SidecarSet support namespace selector. ([#1178](https://github.com/openkruise/kruise/pull/1178), [@zmberg](https://github.com/zmberg))

### Others
- Simplify some code, mainly comparison and variable declaration. ([#1209](https://github.com/openkruise/kruise/pull/1209), [@hezhizhen](https://github.com/hezhizhen))
- Update k8s registry references from k8s.gcr.io to registry.k8s.io. ([#1208](https://github.com/openkruise/kruise/pull/1208), [@asa3311](https://github.com/asa3311))
- Fix config/samples/apps_v1alpha1_uniteddeployment.yaml invalid image. ([#1198](https://github.com/openkruise/kruise/pull/1198), [@chengleqi](https://github.com/chengleqi))
- Change kruise base image to alpine. ([#1166](https://github.com/openkruise/kruise/pull/1166), [@fengshunli](https://github.com/fengshunli))
- PersistentPodState support custom workload (like statefulSet). ([#1063](https://github.com/openkruise/kruise/pull/1063), [@baxiaoshi](https://github.com/baxiaoshi))

## v1.3.1

> Change log since v1.3.0

### CVE FIX: Enhance kruise-daemon security ([#1482](https://github.com/openkruise/kruise/pull/1482), [veophi](https://github.com/veophi))

## v1.3.0

> Change log since v1.2.0

### New CRD and Controller: PodProbeMarker

Kubernetes provides three Pod lifecycle management:
- **Readiness Probe** Used to determine whether the business container is ready to respond to user requests. If the probe fails, the Pod will be removed from Service Endpoints.
- **Liveness Probe** Used to determine the health status of the container. If the probe fails, the kubelet will restart the container.
- **Startup Probe** Used to know when a container application has started. If such a probe is configured, it disables liveness and readiness checks until it succeeds.

So the Probe capabilities provided in Kubernetes have defined specific semantics and related behaviors.
**In addition, there is actually a need to customize Probe semantics and related behaviors**, such as:
- **GameServer defines Idle Probe to determine whether the Pod currently has a game match**, if not, from the perspective of cost optimization, the Pod can be scaled down.
- **K8S Operator defines the main-secondary probe to determine the role of the current Pod (main or secondary)**. When upgrading, the secondary can be upgraded first,
  so as to achieve the behavior of selecting the main only once during the upgrade process, reducing the service interruption time during the upgrade process.

So we provides the ability to customize the Probe and return the result to the Pod yaml.

For more detail, please refer to its [documentation](https://openkruise.io/docs/user-manuals/podprobemarker) and [proposal](https://github.com/openkruise/kruise/blob/master/docs/proposals/20220728-pod-probe-marker.md).

### SidecarSet
- SidecarSet support to inject pods under kube-system,kube-public namespace. ([#1084](https://github.com/openkruise/kruise/pull/1084), [@zmberg](https://github.com/zmberg))
- SidecarSet support to inject specific history sidecar container to Pods. ([#1021](https://github.com/openkruise/kruise/pull/1021), [@veophi](https://github.com/veophi))
- SidecarSet support to inject pod annotations.([#992](https://github.com/openkruise/kruise/pull/992), [@zmberg](https://github.com/zmberg))

### AdvancedCronJob
- TimeZone support for AdvancedCronJob from [upstream](https://github.com/kubernetes/kubernetes/pull/108032). ([#1070](https://github.com/openkruise/kruise/pull/1070), [@FillZpp](https://github.com/FillZpp))

### WorkloadSpread
- WorkloadSpread support Native StatefulSet and Kruise Advanced StatefulSet. ([#1056](https://github.com/openkruise/kruise/pull/1056), [@veophi](https://github.com/veophi))

### CloneSet
- CloneSet supports to calculate scale number excluding Pods in PreparingDelete. ([#1024](https://github.com/openkruise/kruise/pull/1024), [@FillZpp](https://github.com/FillZpp))
- Optimize CloneSet queuing when cache has just synced. ([#1026](https://github.com/openkruise/kruise/pull/1026), [@FillZpp](https://github.com/FillZpp))

### PodUnavailableBudget
- Optimize event handler performance for PodUnavailableBudget. ([#1027](https://github.com/openkruise/kruise/pull/1027), [@FillZpp](https://github.com/FillZpp))

### Advanced DaemonSet
- Allow optional filed max unavilable in ads, and set default value 1. ([#1007](https://github.com/openkruise/kruise/pull/1007), [@ABNER-1](https://github.com/ABNER-1))
- Fix DaemonSet surging with minReadySeconds. ([#1014](https://github.com/openkruise/kruise/pull/1014), [@FillZpp](https://github.com/FillZpp))
- Optimize Advanced DaemonSet internal new pod for imitating scheduling. ([#1011](https://github.com/openkruise/kruise/pull/1011), [@FillZpp](https://github.com/FillZpp))
- Advanced DaemonSet support pre-download image. ([#1057](https://github.com/openkruise/kruise/pull/1057), [@ABNER-1](https://github.com/ABNER-1))

### Advanced StatefulSet
- Fix panic cased by statefulset pvc auto deletion. ([#999](https://github.com/openkruise/kruise/pull/999), [@veophi](https://github.com/veophi))

### Others
- Optimize performance of LabelSelector conversion. ([#1068](https://github.com/openkruise/kruise/pull/1068), [@FillZpp](https://github.com/FillZpp))
- Reduce kruise-manager memory allocation. ([#1015](https://github.com/openkruise/kruise/pull/1015), [@FillZpp](https://github.com/FillZpp))
- Pod state from updating to Normal should all hooked. ([#1022](https://github.com/openkruise/kruise/pull/1022), [@shiyan2016](https://github.com/shiyan2016))
- Fix go get in Makefile with go 1.18. ([#1036](https://github.com/openkruise/kruise/pull/1036), [@astraw99](https://github.com/astraw99))
- Fix EphemeralJob spec.replicas nil panic bug. ([#1016](https://github.com/openkruise/kruise/pull/1016), [@hellolijj](https://github.com/openkruise/kruise/pull/1016))
- Fix UnitedDeployment reconcile don't return err bug. ([#991](https://github.com/openkruise/kruise/pull/991), [@huiwq1990](https://github.com/huiwq1990))

## v1.2.0

> Change log since v1.1.0

### New CRD and Controller: PersistentPodState

With the development of cloud native, more and more companies start to deploy stateful services (e.g., Etcd, MQ) using Kubernetes.
K8S StatefulSet is a workload for managing stateful services, and it considers the deployment characteristics of stateful services in many aspects.
However, StatefulSet persistent only limited pod state, such as Pod Name is ordered and unchanging, PVC persistence,
and can not cover other states, e.g. Pod IP retention, priority scheduling to previously deployed Nodes.

So we provide `PersistentPodState` CRD to persistent other states of the Pod, such as "IP Retention".

For more detail, please refer to its [documentation](https://openkruise.io/docs/user-manuals/persistentpodstate) and [proposal](https://github.com/openkruise/kruise/blob/master/docs/proposals/20220421-persistent-pod-state.md).

### CloneSet

- Ensure at least one pod is upgraded if CloneSet has `partition < 100%` **(Behavior Change)**. ([#954](https://github.com/openkruise/kruise/pull/954), [@veophi](https://github.com/veophi))
- Add `expectedUpdatedReplicas` field into CloneSet status. ([#954](https://github.com/openkruise/kruise/pull/954) & [#963](https://github.com/openkruise/kruise/pull/963), [@veophi](https://github.com/veophi))
- Add `markPodNotReady` field into lifecycle hook to support marking Pod as NotReady during preparingDelete or preparingUpdate. ([#979](https://github.com/openkruise/kruise/pull/979), [@veophi](https://github.com/veophi))

### StatefulSet

- Add `markPodNotReady` field into lifecycle hook to support marking Pod as NotReady during preparingDelete or preparingUpdate. ([#979](https://github.com/openkruise/kruise/pull/979), [@veophi](https://github.com/veophi))

### PodUnavailableBudget

- Support to protect any custom workloads with scale subresource. ([#982](https://github.com/openkruise/kruise/pull/982), [@zmberg](https://github.com/zmberg))
- Optimize performance in large-scale clusters by avoiding DeepCopy list. ([#955](https://github.com/openkruise/kruise/pull/955), [@zmberg](https://github.com/zmberg))

### Others

- Remove some commented code and simplify some. ([#983](https://github.com/openkruise/kruise/pull/983), [@hezhizhen](https://github.com/hezhizhen))
- Sidecarset forbid updating of sidecar container name. ([#937](https://github.com/openkruise/kruise/pull/937), [@adairxie](https://github.com/adairxie))
- Optimize the logic of listNamespacesForDistributor func. ([#952](https://github.com/openkruise/kruise/pull/952), [@hantmac](https://github.com/hantmac))

## v1.1.0

> Change log since v1.0.1

### Project

- Bump Kubernetes dependencies to 1.22 and controller-runtime to v0.10.2. ([#915](https://github.com/openkruise/kruise/pull/915), [@FillZpp](https://github.com/FillZpp))
- Disable DeepCopy for some specific cache list. ([#916](https://github.com/openkruise/kruise/pull/916), [@FillZpp](https://github.com/FillZpp))

### InPlace Update

- Support in-place update containers with launch priority, for workloads that supported in-place update, e.g., CloneSet, Advanced StatefulSet. ([#909](https://github.com/openkruise/kruise/pull/909), [@FillZpp](https://github.com/FillZpp))

### CloneSet

- Add `pod-template-hash` label into Pods, which will always be the short hash. ([#931](https://github.com/openkruise/kruise/pull/931), [@FillZpp](https://github.com/FillZpp))
- Support pre-download image after a number of updated pods has been ready. ([#904](https://github.com/openkruise/kruise/pull/904), [@shiyan2016](https://github.com/shiyan2016))
- Make maxUnavailable also limited to pods in new revision. ([#899](https://github.com/openkruise/kruise/pull/899), [@FillZpp](https://github.com/FillZpp))

### Advanced DaemonSet

- Refactor daemonset controller and fetch upstream codebase. ([#883](https://github.com/openkruise/kruise/pull/883), [@FillZpp](https://github.com/FillZpp))
- Support preDelete lifecycle for both scale down and recreate update. ([#923](https://github.com/openkruise/kruise/pull/923), [@FillZpp](https://github.com/FillZpp))
- Fix node event handler that should compare update selector matching changed. ([#920](https://github.com/openkruise/kruise/pull/920), [@LastNight1997](https://github.com/LastNight1997))
- Optimize `dedupCurHistories` func in ReconcileDaemonSet. ([#912](https://github.com/openkruise/kruise/pull/912), [@LastNight1997](https://github.com/LastNight1997))

### Advanced StatefulSet

- Support StatefulSetAutoDeletePVC feature. ([#882](https://github.com/openkruise/kruise/pull/882), [@veophi](https://github.com/veophi))

### SidecarSet

- Support shared volumes in init containers. ([#929](https://github.com/openkruise/kruise/pull/929), [@outgnaY](https://github.com/outgnaY))
- Support transferEnv in init containers. ([#897](https://github.com/openkruise/kruise/pull/897), [@pigletfly](https://github.com/pigletfly))
- Optimize the injection for pod webhook that checks container exists. ([#927](https://github.com/openkruise/kruise/pull/927), [@zmberg](https://github.com/zmberg))
- Fix validateSidecarConflict to avoid a same sidecar container exists in multiple sidecarsets. ([#884](https://github.com/openkruise/kruise/pull/884), [@pigletfly](https://github.com/pigletfly))

### Kruise-daemon

- Support CRI-O and any other common CRI types. ([#930](https://github.com/openkruise/kruise/pull/930), [@diannaowa](https://github.com/diannaowa)) & ([#936](https://github.com/openkruise/kruise/pull/936), [@FillZpp](https://github.com/FillZpp))

### Other

- Add `kruise_manager_is_leader` metric. ([#917](https://github.com/openkruise/kruise/pull/917), [@hatowang](https://github.com/hatowang))

## v1.0.1

> Change log since v1.0.0

### SidecarSet

- Fix panic when SidecarSet manages Pods with sidecar containers that have different update type. ([#850](https://github.com/openkruise/kruise/pull/850), [@veophi](https://github.com/veophi))
- Fix log error when extract container from fieldpath failed. ([#860](https://github.com/openkruise/kruise/pull/860), [@pigletfly](https://github.com/pigletfly))
- Optimization logic of determining whether the pod state is consistent logic. ([#854](https://github.com/openkruise/kruise/pull/854), [@dafu-wu](https://github.com/dafu-wu))
- Replace reflect with generation in event handler. ([#885](https://github.com/openkruise/kruise/pull/885), [@zouyee](https://github.com/zouyee))
- Store history revisions for sidecarset. ([#715](https://github.com/openkruise/kruise/pull/715), [@veophi](https://github.com/veophi))

### Advanced StatefulSet

- Allow updating asts RevisionHistoryLimit. ([#864](https://github.com/openkruise/kruise/pull/864), [@shiyan2016](https://github.com/shiyan2016))
- StatefulSet considers non-available pods when deleting pods. ([#880](https://github.com/openkruise/kruise/pull/880), [@hzyfox](https://github.com/hzyfox))

### ResourceDistribution

- Improve controller updating and watching logic. ([#861](https://github.com/openkruise/kruise/pull/861), [@veophi](https://github.com/veophi))

### CloneSet

- Break the loop when it finds the current revision. ([#887](https://github.com/openkruise/kruise/pull/887), [@shiyan2016](https://github.com/shiyan2016))
- Remove duplicate register fieldindexes in cloneset controller. ([#888](https://github.com/openkruise/kruise/pull/888) & [#889](https://github.com/openkruise/kruise/pull/889), [@shiyan2016](https://github.com/shiyan2016))
- CloneSet refresh pod states before skipping update when paused **(Behavior Change)**. ([#893](https://github.com/openkruise/kruise/pull/893), [@FillZpp](https://github.com/FillZpp))

### PullImageJob

- Update containerd client usage and event handler for controller. ([#894](https://github.com/openkruise/kruise/pull/894), [@FillZpp](https://github.com/FillZpp))

## v0.10.2

> Change log since v0.10.1

### SidecarSet

- Add SourceContainerNameFrom and EnvNames in sidecarset transferenv.

### Advanced StatefulSet

- Fix update expectation to be increased when a pod updated.

### WorkloadSpread

- Fix bug: read conditions from nil old subset status.

### Other

- Do not set timeout for webhook ready.

## v1.0.0

> Change log since v0.10.1

### Project

- Bump CustomResourceDefinition(CRD) from v1beta1 to v1
- Bump ValidatingWebhookConfiguration/MutatingWebhookConfiguration from v1beta1 to v1
- Bump dependencies: k8s v1.18 -> v1.20, controller-runtime v0.6.5 -> v0.8.3
- Generate CRDs with original controller-tools and markers

**So that Kruise can install into Kubernetes 1.22 and no longer support Kubernetes < 1.16.**

### New feature: in-place update with env from metadata

When update `spec.template.metadata.labels/annotations` in CloneSet or Advanced StatefulSet and there exists container env from the changed labels/annotations,
Kruise will in-place update them to renew the env value in containers.

[doc](https://openkruise.io/docs/core-concepts/inplace-update#understand-inplaceifpossible)

### New feature: ContainerLaunchPriority

Container Launch Priority provides a way to help users control the sequence of containers start in a Pod.

It works for Pod, no matter what kind of owner it belongs to, which means Deployment, CloneSet or any other Workloads are all supported.

[doc](https://openkruise.io/docs/user-manuals/containerlaunchpriority)

### New feature: ResourceDistribution

For the scenario, where the namespace-scoped resources such as Secret and ConfigMap need to be distributed or synchronized to different namespaces,
the native k8s currently only supports manual distribution and synchronization by users one-by-one, which is very inconvenient.

Therefore, in the face of these scenarios that require the resource distribution and **continuously synchronization across namespaces**, we provide a tool, namely **ResourceDistribution**, to do this automatically.

Currently, ResourceDistribution supports the two kind resources --- **Secret & ConfigMap**.

[doc](https://openkruise.io/docs/user-manuals/resourcedistribution)

### CloneSet

- Add `maxUnavailable` field in `scaleStrategy` to support rate limiting of scaling up.
- Mark revision stable as `currentRevision` when all pods updated to it, won't wait all pods to be ready **(Behavior Change)**.

### WorkloadSpread

- Manage the pods that were created before WorkloadSpread.
- Optimize webhook update and retry during injection.

### PodUnavailableBudget

- Add pod no pub-protection annotation.
- PUB controller watch workload replicas changed.

### Advanced DaemonSet

- Support in-place update daemon pod.
- Support progressive annotation to control if pods creation should be limited by partition.

### SidecarSet

- Fix SidecarSet filter active pods.

### UnitedDeployment

- Fix pod NodeSelectorTerms length 0 when UnitedDeployment NodeSelectorTerms is nil.

### NodeImage

- Add `--nodeimage-creation-delay` flag to delay NodeImage creation after Node ready.

### Other

- Kruise-daemon watch pods using protobuf.
- Export resync seconds args.
- Fix http checker reload ca.cert.
- Fix E2E for WorkloadSpread, ImagePulling, ContainerLaunchPriority.

## v1.0.0-beta.0

> Change log since v1.0.0-alpha.2

### New feature: ResourceDistribution

For the scenario, where the namespace-scoped resources such as Secret and ConfigMap need to be distributed or synchronized to different namespaces,
the native k8s currently only supports manual distribution and synchronization by users one-by-one, which is very inconvenient.

Therefore, in the face of these scenarios that require the resource distribution and **continuously synchronization across namespaces**, we provide a tool, namely **ResourceDistribution**, to do this automatically.

Currently, ResourceDistribution supports the two kind resources --- **Secret & ConfigMap**.

[doc](https://openkruise.io/docs/next/user-manuals/resourcedistribution)

### CloneSet

- Add `maxUnavailable` field in `scaleStrategy` to support rate limiting of scaling up.
- Mark revision stable when all pods updated to it, won't wait all pods to be ready.

### Advanced DaemonSet

- Support progressive annotation to control if pods creation should be limited by partition.

### UnitedDeployment

- Fix pod NodeSelectorTerms length 0 when UnitedDeployment NodeSelectorTerms is nil.

## v1.0.0-alpha.2

> Change log since v1.0.0-alpha.1

### Project

- Generate CRDs with original controller-tools and markers

### WorkloadSpread

- Add discoveryGVK for WorkloadSpread

### NodeImage

- Add `--nodeimage-creation-delay` flag to delay NodeImage creation after Node ready

### Other

- Fix E2E for WorkloadSpread, ImagePulling, ContainerLaunchPriority

## v0.10.1

> Change log since v0.10.0

### WorkloadSpread

- Add discoveryGVK for WorkloadSpread
- Optimize webhook injection

### Kruise-daemon

- Setup generic kubeClient with Protobuf

### Other

- Fix E2E for WorkloadSpread, ImagePulling

## v1.0.0-alpha.1

> Change log since v0.10.0

### Project

- Bump CustomResourceDefinition(CRD) from v1beta1 to v1
- Bump ValidatingWebhookConfiguration/MutatingWebhookConfiguration from v1beta1 to v1
- Bump dependencies: k8s v1.18 -> v1.20, controller-runtime v0.6.5 -> v0.8.3

**So that Kruise can install into Kubernetes 1.22 and no longer support Kubernetes < 1.16.**

### New feature: in-place update with env from metadata

When update `spec.template.metadata.labels/annotations` in CloneSet or Advanced StatefulSet and there exists container env from the changed labels/annotations,
Kruise will in-place update them to renew the env value in containers.

[doc](https://openkruise.io/docs/next/core-concepts/inplace-update#understand-inplaceifpossible)

### New feature: ContainerLaunchPriority

Container Launch Priority provides a way to help users control the sequence of containers start in a Pod.

It works for Pod, no matter what kind of owner it belongs to, which means Deployment, CloneSet or any other Workloads are all supported.

[doc](https://openkruise.io/docs/next/user-manuals/containerlaunchpriority)

### WorkloadSpread

- Manage the pods that were created before WorkloadSpread.
- Optimize webhook update and retry during injection.

### PodUnavailableBudget

- Add pod no pub-protection annotation.
- PUB controller watch workload replicas changed.

### Advanced DaemonSet

- Support in-place update daemon pod.

### SidecarSet

- Fix SidecarSet filter active pods.

### Other

- Kruise-daemon watch pods using protobuf.
- Export resync seconds args.
- Fix http checker reload ca.cert.

## v0.10.0

### New feature: PodUnavailableBudget

Kubernetes offers [Pod Disruption Budget (PDB)](https://kubernetes.io/docs/tasks/run-application/configure-pdb/) to help you run highly available applications even when you introduce frequent voluntary disruptions.
PDB limits the number of Pods of a replicated application that are down simultaneously from voluntary disruptions.
However, it can only constrain the voluntary disruption triggered by the Eviction API.
For example, when you run kubectl drain, the tool tries to evict all of the Pods on the Node you're taking out of service.

PodUnavailableBudget can achieve the effect of preventing ALL application disruption or SLA degradation, including pod eviction, deletion, inplace-update, ...

[doc](https://openkruise.io/docs/user-manuals/podunavailablebudget)

### New feature: WorkloadSpread

WorkloadSpread supports to constrain the spread of stateless workload, which empowers single workload the abilities for multi-domain and elastic deployment.

It can be used with those stateless workloads, such as CloneSet, Deployment, ReplicaSet and even Job.

[doc](https://openkruise.io/docs/user-manuals/workloadspread)

### CloneSet

- Scale-down supports topology spread constraints. [doc](https://openkruise.io/docs/user-manuals/cloneset#deletion-by-spread-constraints)
- Fix in-place update pods in Updated state.

### SidecarSet

- Add imagePullSecrets field to support pull secrets for the sidecar images. [doc](https://openkruise.io/docs/user-manuals/sidecarset#imagepullsecrets)
- Add injectionStrategy.paused to stop injection temporarily. [doc](https://openkruise.io/docs/user-manuals/sidecarset#injection-pause)

### Advanced StatefulSet

- Support image pre-download for in-place update, which can accelerate the progress of applications upgrade. [doc](https://openkruise.io/docs/user-manuals/advancedstatefulset#pre-download-image-for-in-place-update)
- Support scaling with rate limit. [doc](https://openkruise.io/docs/user-manuals/advancedstatefulset#scaling-with-rate-limiting)

### Advanced DaemonSet

- Fix rolling update stuck caused by deleting terminating pods.

### Other

- Bump to Kubernetes dependency to 1.18
- Add pod informer for kruise-daemon
- More `kubectl ... -o wide` fields for kruise resources

## v0.9.0

### New feature: ContainerRecreateRequest

[[doc](https://openkruise.io/docs/user-manuals/containerrecreaterequest)]

ContainerRecreateRequest provides a way to let users **restart/recreate** one or more containers in an existing Pod.

### New feature: Deletion Protection

[[doc](https://openkruise.io/docs/user-manuals/deletionprotection)]

This feature provides a safety policy which could help users protect Kubernetes resources and
applications' availability from the cascading deletion mechanism.

### CloneSet

- Support `pod-deletion-cost` to let users set the priority of pods deletion. [[doc](https://openkruise.io/docs/user-manuals/cloneset#pod-deletion-cost)]
- Support image pre-download for in-place update, which can accelerate the progress of applications upgrade. [[doc](https://openkruise.io/docs/user-manuals/cloneset#pre-download-image-for-in-place-update)]
- Add `CloneSetShortHash` feature-gate, which solves the length limit of CloneSet name. [[doc](https://openkruise.io/docs/user-manuals/cloneset#short-hash-label)]
- Make `maxUnavailable` and `maxSurge` effective for specified deletion. [[doc](https://openkruise.io/docs/user-manuals/cloneset#maxunavailable)]
- Support efficient update and rollback using `partition`. [[doc](https://openkruise.io/docs/user-manuals/cloneset#rollback-by-partition)]

### SidecarSet

- Support sidecar container **hot upgrade**. [[doc](https://openkruise.io/docs/user-manuals/sidecarset#hot-upgrade-sidecar)]

### ImagePullJob

- Add `podSelector` to pull image on nodes of the specific pods.

## v0.8.1

### Kruise-daemon

- Optimize cri-runtime for kruise-daemon

### BroadcastJob

- Fix broadcastjob expectation observed when node assigned by scheduler

## v0.8.0

### Breaking changes

1. The flags for kruise-manager must start with `--` instead of `-`. If you install Kruise with helm chart, ignore this.
2. SidecarSet has been refactored. Make sure there is no SidecarSet being upgrading when you upgrade Kruise,
   and read [the latest doc for SidecarSet](https://openkruise.io/docs/user-manuals/sidecarset).
3. A new component named `kruise-daemon` comes in. It is deployed in kruise-system using DaemonSet, defaults on every Node.

Now Kruise includes two components:

- **kruise-controller-manager**: contains multiple controllers and webhooks, deployed using Deployment.
- **kruise-daemon**: contains bypass features like image pre-download and container restart in the future, deployed using DaemonSet.

### New CRDs: NodeImage and ImagePullJob

[Official doc](https://openkruise.io/docs/user-manuals/imagepulljob)

Kruise will create a NodeImage for each Node, and its `spec` contains the images that should be downloaded on this Node.

Also, users can create an ImagePullJob CR to declare an image should be downloaded on which nodes.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: ImagePullJob
metadata:
  name: test-imagepulljob
spec:
  image: nginx:latest
  completionPolicy:
    type: Always
  parallelism: 10
  pullPolicy:
    backoffLimit: 3
    timeoutSeconds: 300
  selector:
    matchLabels:
      node-label: xxx
```

### SidecarSet

[Official doc](https://openkruise.io/docs/user-manuals/sidecarset)

- Refactor the controller and webhook for SidecarSet:
  - For `spec`:
    - Add `namespace`: indicates this SidecarSet will only inject for Pods in this namespace.
    - For `spec.containers`:
      - Add `podInjectPolicy`: indicates this sidecar container should be injected in the front or end of `containers` list.
      - Add `upgradeStrategy`: indicates the upgrade strategy of this sidecar container (currently it only supports `ColdUpgrade`)
      - Add `shareVolumePolicy`: indicates whether to share other containers' VolumeMounts in the Pod.
      - Add `transferEnv`: can transfer the names of env shared from other containers.
    - For `spec.updateStrategy`:
      - Add `type`: contains `NotUpdate` or `RollingUpdate`.
      - Add `selector`: indicates only update Pods that matched this selector.
      - Add `partition`: indicates the desired number of Pods in old revisions.
      - Add `scatterStrategy`: defines the scatter rules to make pods been scattered during updating.

### CloneSet

- Add `currentRevision` field in status.
- Optimize CloneSet scale sequence.
- Fix condition for pod lifecycle state from Updated to Normal.
- Change annotations `inplace-update-state` => `apps.kruise.io/inplace-update-state`, `inplace-update-grace` => `apps.kruise.io/inplace-update-grace`.
- Fix `maxSurge` calculation when partition > replicas.

### UnitedDeployment

- Support Deployment as template in UnitedDeployment.

### Advanced StatefulSet

- Support lifecycle hook for in-place update and pre-delete.

### BroadcastJob

- Add PodFitsResources predicates.
- Add `--assign-bcj-pods-by-scheduler` flag to control whether to use scheduler to assign BroadcastJob's Pods.

### Others

- Add feature-gate to replace the CUSTOM_RESOURCE_ENABLE env.
- Add GetScale/UpdateScale into clientsets for scalable resources.
- Support multi-platform build in Makefile.
- Set different user-agent for controllers.

## v0.7.0

### Breaking changes

Since v0.7.0:

1. OpenKruise requires Kubernetes 1.13+ because of CRD conversion.
   Note that for Kubernetes 1.13 and 1.14, users must enable `CustomResourceWebhookConversion` feature-gate in kube-apiserver before install or upgrade Kruise.
2. OpenKruise official image supports multi-arch, by default including linux/amd64, linux/arm64, and linux/arm platforms.

### A NEW workload controller - AdvancedCronJob
Thanks for @rishi-anand contributing!

An enhanced version of CronJob, it supports multiple kind in a template:

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: AdvancedCronJob
spec:
  template:

    # Option 1: use jobTemplate, which is equivalent to original CronJob
    jobTemplate:
      # ...

    # Option 2: use broadcastJobTemplate, which will create a BroadcastJob object when cron schedule triggers
    broadcastJobTemplate:
      # ...

    # Options 3(future): ...
```

### CloneSet

- **Partition support intOrStr format**
- Warning log for expectation timeout
- Remove ownerRef when pod's labels not matched CloneSet's selector
- Allow updating revisionHistoryLimit in validation
- Fix resourceVersionExpectation race condition
- Fix overwrite gracePeriod update
- Fix webhook checking podsToDelete

### StatefulSet

- **Promote Advanced StatefulSet to v1beta1**
  - A conversion webhook will help users to transfer existing and new `v1alpha1` advanced statefulsets to `v1beta1` automatically
  - Even all advanced statefulsets have been converted to `v1beta1`, users can still get them through `v1alpha1` client and api
- **Support reserveOrdinal for Advanced StatefulSet**

### DaemonSet

- Add validation webhook for DaemonSet
- Fix pending pods created by controller

### BroadcastJob

- Optimize the way to calculate parallelism
- Check ownerReference for filtered pods
- Add pod label validation
- Add ScaleExpectation for BroadcastJob

### Others

- Initializing capabilities if allowPrivileged is true
- Support secret cert for webhook with vip
- Add rate limiter config
- Fix in-place rollback when spec image no latest tag

## v0.6.1

### CloneSet

#### Features

- Support lifecycle hooks for pre-delete and in-place update

#### Bugs

- Fix map concurrent write
- Fix current revision during rollback
- Fix update expectation for pod deletion

### SidecarSet

#### Features

- Support initContainers definition and injection

### UnitedDeployment

#### Features

- Support to define CloneSet as UnitedDeployment's subset

### StatefulSet

#### Features

- Support minReadySeconds strategy

### Others

- Add webhook controller to optimize certs and configurations generation
- Add pprof server and flag
- Optimize discovery logic in custom resource gate

## v0.6.0

### Project

- Update dependencies: k8s v1.13 -> v1.16, controller-runtime v0.1.10 -> v0.5.7
- Support multiple active webhooks
- Fix CRDs using openkruise/controller-tools

### A NEW workload controller - Advanced DaemonSet

An enhanced version of default DaemonSet with extra functionalities such as:

- **inplace update** and **surging update**
- **node selector for update**
- **partial update**

### CloneSet

#### Features

- Not create excessive pods when updating with maxSurge
- Round down maxUnavaliable when maxSurge > 0

#### Bugs

- Skip recreate when inplace update failed
- Fix scale panic when replicas < partition
- Fix CloneSet blocked by terminating PVC

---

## v0.5.0

### CloneSet

#### Features

- Support `maxSurge` strategy which could work well with `maxUnavailable` and `partition`
- Add CloneSet core interface to support multiple implementations

#### Bugs

- Fix in-place update for metadata in template

### StatefulSet

#### Bugs

- Make sure `maxUnavailable` should not be less than 1
- Fix in-place update for metadata in template

### SidecarSet

#### Bugs

- Merge volumes during injecting sidecars into Pod

### Installation

- Expose `CUSTOM_RESOURCE_ENABLE` env by chart set option

---

## v0.4.1

### CloneSet

#### Features

- Add `labelSelector` to optimize scale subresource for HPA
- Add `minReadySeconds`, `availableReplicas` fields for CloneSet
- Add `gracePeriodSeconds` for graceful in-place update

### Advanced StatefulSet

#### Features

- Support label selector in scale for HPA
- Add `gracePeriodSeconds` for graceful in-place update

#### Bugs

- Fix StatefulSet default update sequence
- Fix ControllerRevision adoption

### Installation

- Fix `check_for_installation.sh` script for k8s 1.11 to 1.13

---

## v0.4.0**

### A NEW workload controller - CloneSet

Mainly focuses on managing stateless applications. ([Concept for CloneSet](https://openkruise.io/docs/user-manuals/cloneset))

It provides full features for more efficient, deterministic and controlled deployment, such as:

- **inplace update**
- **specified pod deletion**
- **configurable priority/scatter update**
- **preUpdate/postUpdate hooks**

### Features

- UnitedDeployment supports both StatefulSet and AdvancedStatefulSet.
- UnitedDeployment supports toleration config in subset.

### Bugs

- Fix statefulset inplace update fields in pod metadata such as labels/annotations.

---

## v0.3.1**

### Installation

- Simplify installation with helm charts, one simple command to install kruise charts, instead of downloading and executing scripts.

### Advanced StatefulSet

#### Features

- Support [priority update](https://openkruise.io/docs/user-manuals/advancedstatefulset#priority-strategy), which allows users to configure the sequence for Pods updating.

#### Bugs

- Fix maxUnavailable calculation, which should not be less than 1.

### Broadcast Job

#### Bugs

- Fix BroadcastJob cleaning up after TTL.

---

## v0.3.0**

### Installation

- Provide a script to check if the K8s cluster has enabled MutatingAdmissionWebhook and ValidatingAdmissionWebhook admission plugins before installing Kruise.
- Users can now install specific controllers if they only need some of the Kruise CRD/controllers.

### Kruise-Controller-Manager

#### Bugs

- Fix a jsonpatch bug by updating the vendor code.

### Advanced StatefulSet

#### Features

- Add condition report in `status` to indicate the scaling or rollout results.

### A NEW workload controller - UnitedDeployment

#### Features

- Define a set of APIs for UnitedDeployment workload which manages multiple workloads spread over multiple domains in one cluster.
- Create one workload for each `Subset` in `Topology`.
- Manage Pod replica distribution across subset workloads.
- Rollout all subset workloads by specifying a new workload template.
- Manually manage the rollout of subset workloads by specifying the `Partition` of each workload.

### Documents

- Three blog posts are added in Kruise [website](http://openkruise.io), titled:
  1. Kruise Controller Classification Guidance.
  2. Learning Concurrent Reconciling.
  3. UnitedDeploymemt - Supporting Multi-domain Workload Management.
- New documents are added for UnitedDeployment, including a [tutorial](./docs/tutorial/uniteddeployment.md).
- Revise main README.md.

---

## v0.2.0**

### Installation

- Provide a script to generate helm charts for Kruise. User can specify the release version.
- Automatically install kubebuilder if it does not exist in the machine.
- Add Kruise uninstall script.

### Kruise-Controller-Manager

#### Bugs

- Fix a potential controller crash problem when APIServer disables MutatingAdmissionWebhook and ValidatingAdmissionWebhook admission plugins.

### Broadcast Job

#### Features

- Change the type of `Parallelism` field in BroadcastJob from `*int32` to `intOrString`.
- Support `Pause` in BroadcastJob.
- Add `FailurePolicy` in BroadcastJob, supporting `Continue`, `FastFailed`, and `Pause` polices.
- Add `Phase` in BroadcastJob `status`.

### Sidecar Set

#### Features

- Allow parallelly upgrading SidecarSet Pods by specifying `MaxUnavailable`.
- Support sidecar volumes so that user can specify volume mount in sidecar containers.

---

## v0.1.0**

### Kruise-Controller-Manager

#### Features

- Support to run kruise-controller-manager locally
- Allow selectively install required CRDs for kruise controllers

#### Bugs

- Remove `sideEffects` in kruise-manager all-in-one YAML file to avoid start failure

### Advanced Statefulset

#### Features

- Add MaxUnavailable rolling upgrade strategy
- Add In-Place pod update strategy
- Add paused functionality during rolling upgrade

### Broadcast Job

#### Features

- Add BroadcastJob that runs pods on all nodes to completion
- Add `Never` termination policy to have job running after it finishes all pods
- Add `ttlSecondsAfterFinished` to delete the job after it finishes in x seconds.

#### Bugs

- Make broadcastjob honor node unschedulable condition

### Sidecar Set

#### Features

- Add SidecarSet that automatically injects sidecar container into selected pods
- Support sidecar update functionality for SidecarSet