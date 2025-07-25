# Prometheus Metrics

OpenKruise exposes metrics in Prometheus format, crucial for monitoring the health and performance of its controllers and managed workloads. This guide outlines how to access, understand, and utilize these metrics.

## Enabling Metrics

By default, the OpenKruise controller manager (`kruise-controller-manager`) exposes metrics on port `8080` at the `/metrics` endpoint. This is typically enabled during installation (e.g., via Helm).

To verify:

1.  **Port-forward to the `kruise-controller-manager` service:**
    (The service port for metrics is often named `metrics` or is the main service port, e.g., 8080)
    ```bash
    # kubectl get svc -n kruise-system kruise-controller-manager -o jsonpath='{.spec.ports[?(@.name=="metrics")].port}' # to find port
    kubectl port-forward -n kruise-system svc/kruise-controller-manager 8080:8080
    ```

2.  **Query the metrics endpoint:**
    ```bash
    curl localhost:8080/metrics
    ```

## Key Metrics Categories

### 1. Controller Runtime Metrics
Standard metrics from `controller-runtime` library, offering insights into individual controller performance:

| Metric Name                                  | Type      | Description                                      | Labels       |
| -------------------------------------------- | --------- | ------------------------------------------------ | ------------ |
| `controller_runtime_reconcile_total`         | Counter   | Total reconciliations per controller.            | `controller` |
| `controller_runtime_reconcile_errors_total`  | Counter   | Total reconciliation errors per controller.      | `controller` |
| `controller_runtime_reconcile_time_seconds`  | Histogram | Reconciliation duration per controller.          | `controller` |
| `workqueue_depth`                            | Gauge     | Current workqueue depth per controller.          | `name`       |
| `workqueue_adds_total`                       | Counter   | Total items added to workqueue.                  | `name`       |
| `workqueue_retries_total`                    | Counter   | Total retries handled by workqueue.              | `name`       |

**Use to:** Identify overloaded controllers (high `workqueue_depth`, long `reconcile_time_seconds`) or persistent issues (increasing `reconcile_errors_total`).

### 2. OpenKruise Specific Metrics
Custom metrics for OpenKruise features.
**IMPORTANT: Verify exact metric names and labels against your OpenKruise `/metrics` endpoint.** The following are illustrative examples:

| Metric Name (Illustrative)                               | Type      | Description (Illustrative)                                  | Labels (Illustrative)         |
| -------------------------------------------------------- | --------- | ----------------------------------------------------------- | ----------------------------- |
| `kruise_cloneset_status_replicas`                        | Gauge     | Current number of replicas for a CloneSet.                  | `cloneset`, `namespace`       |
| `kruise_cloneset_status_updated_replicas`                | Gauge     | Number of updated replicas for a CloneSet.                  | `cloneset`, `namespace`       |
| `kruise_advancedstatefulset_update_duration_seconds`     | Histogram | Time for AdvancedStatefulSet updates.                       | `statefulset`, `namespace`    |
| `kruise_sidecarset_status_matched_pods`                  | Gauge     | Pods matched by a SidecarSet.                               | `sidecarset`                  |
| `kruise_pod_inplace_update_total`                        | Counter   | Total in-place pod updates.                                 | `controller_kind`, `namespace`|
| `kruise_webhook_admission_requests_total`                | Counter   | Admission requests to Kruise webhooks.                      | `webhook_type`, `allowed`     |
| `kruise_webhook_admission_duration_seconds`              | Histogram | Latency of Kruise webhook admission requests.               | `webhook_type`, `operation`   |

**Use to:** Monitor rollouts, feature performance (e.g., in-place updates), and webhook health.

### 3. Go Runtime and Process Metrics
Standard Go and process metrics for advanced debugging of `kruise-controller-manager` resource usage.

## Setting Up Monitoring with Prometheus

### Prometheus ServiceMonitor
For Prometheus Operator users, a `ServiceMonitor` automates scraping:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kruise-controller-manager
  namespace: monitoring # Your monitoring namespace
  labels:
    release: prometheus # Example label for Prometheus to discover this ServiceMonitor
spec:
  selector:
    matchLabels:
      # Labels of your kruise-controller-manager service
      # Verify with: kubectl get svc -n kruise-system kruise-controller-manager --show-labels
      control-plane: controller-manager # Common label, verify
  namespaceSelector:
    matchNames:
      - kruise-system # OpenKruise installation namespace
  endpoints:
  - port: metrics # Name of the metrics port in the service (e.g., 'metrics', 'http-metrics')
    interval: 30s
    # path: /metrics # Usually default


Note: Ensure selector and port match your OpenKruise service definition.

###Example Prometheus Alerting Rules

#Key alerts to consider (adapt thresholds to your environment):

apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: kruise-alerts
  namespace: monitoring # Your monitoring namespace
spec:
  groups:
  - name: kruise.rules
    rules:
    - alert: KruiseControllerReconciliationErrors
      # Alerts if any Kruise controller has a significant error rate
      expr: sum(rate(controller_runtime_reconcile_errors_total{controller=~".*kruise.*"}[5m])) by (controller) > 0.1
      for: 10m
      labels:
        severity: warning
      annotations:
        summary: "Kruise controller {{ $labels.controller }} reconciliation errors"
        description: "{{ $labels.controller }} has >0.1 reconciliation errors/sec for 10m."

    - alert: KruiseControllerHighReconciliationLatency
      # Alerts if P95 reconciliation latency for any Kruise controller is high
      expr: histogram_quantile(0.95, sum(rate(controller_runtime_reconcile_time_seconds_bucket{controller=~".*kruise.*"}[5m])) by (le, controller)) > 5
      for: 10m
      labels:
        severity: warning
      annotations:
        summary: "Kruise controller {{ $labels.controller }} high latency"
        description: "P95 reconciliation latency for {{ $labels.controller }} >5s for 10m."

    - alert: KruiseWebhookErrors
      # Alerts if Kruise webhooks are frequently denying requests (verify metric name)
      expr: (sum(rate(kruise_webhook_admission_requests_total{allowed="false"}[5m])) by (webhook_type) / sum(rate(kruise_webhook_admission_requests_total[5m])) by (webhook_type)) * 100 > 5
      for: 5m
      labels:
        severity: critical
      annotations:
        summary: "High error rate for Kruise webhook {{ $labels.webhook_type }}"
        description: "More than 5% of requests to Kruise webhook {{ $labels.webhook_type }} are failing."


### Grafana allows visualization of these metrics. You can build dashboards or import existing ones. Below is a compact example dashboard JSON focusing on key controller metrics.

# Importing to Grafana:

# Go to Dashboards -> Import.

# Paste the JSON below or upload it as a file.

# Name the dashboard and select your Prometheus datasource.

# Example Grafana Dashboard JSON (Compact):

{
  "__inputs": [],
  "__requires": [
    {"type": "grafana", "id": "grafana", "name": "Grafana", "version": "7.0.0"},
    {"type": "datasource", "id": "prometheus", "name": "Prometheus", "version": "1.0.0"}
  ],
  "annotations": {"list": [{"builtIn": 1, "datasource": {"type": "grafana", "uid": "-- Grafana --"}, "enable": true, "hide": true, "iconColor": "rgba(0, 211, 255, 1)", "name": "Annotations & Alerts", "type": "dashboard"}]},
  "editable": true, "fiscalYearStartMonth": 0, "graphTooltip": 0, "id": null, "links": [], "liveNow": false,
  "panels": [
    {
      "title": "Controller Reconciliation Rate (Overall)", "type": "timeseries", "datasource": {"type": "prometheus"}, "gridPos": {"h": 7, "w": 12, "x": 0, "y": 0},
      "targets": [{"expr": "sum(rate(controller_runtime_reconcile_total{controller=~'.*kruise.*'}[5m])) by (controller)", "legendFormat": "{{controller}}"}],
      "options": {"legend": {"displayMode": "list", "placement": "bottom"}, "tooltip": {"mode": "single"}}
    },
    {
      "title": "Controller Error Rate (Overall)", "type": "timeseries", "datasource": {"type": "prometheus"}, "gridPos": {"h": 7, "w": 12, "x": 12, "y": 0},
      "targets": [{"expr": "sum(rate(controller_runtime_reconcile_errors_total{controller=~'.*kruise.*'}[5m])) by (controller)", "legendFormat": "{{controller}}"}],
      "options": {"legend": {"displayMode": "list", "placement": "bottom"}, "tooltip": {"mode": "single"}}
    },
    {
      "title": "Controller Latency P95 (Overall)", "type": "timeseries", "datasource": {"type": "prometheus"}, "gridPos": {"h": 7, "w": 12, "x": 0, "y": 7}, "fieldConfig": {"defaults": {"unit": "s"}},
      "targets": [{"expr": "histogram_quantile(0.95, sum(rate(controller_runtime_reconcile_time_seconds_bucket{controller=~'.*kruise.*'}[5m])) by (le, controller))", "legendFormat": "{{controller}} P95"}],
      "options": {"legend": {"displayMode": "list", "placement": "bottom"}, "tooltip": {"mode": "single"}}
    },
    {
      "title": "WorkQueue Depth (Overall)", "type": "timeseries", "datasource": {"type": "prometheus"}, "gridPos": {"h": 7, "w": 12, "x": 12, "y": 7},
      "targets": [{"expr": "sum(workqueue_depth{name=~'.*kruise.*'}) by (name)", "legendFormat": "{{name}}"}],
      "options": {"legend": {"displayMode": "list", "placement": "bottom"}, "tooltip": {"mode": "single"}}
    },
    {
      "title": "Webhook Request Rate (Verify Metric)", "type": "timeseries", "datasource": {"type": "prometheus"}, "gridPos": {"h": 7, "w": 12, "x": 0, "y": 14},
      "targets": [{"expr": "sum(rate(kruise_webhook_admission_requests_total[1m])) by (webhook_type, operation, kind, allowed)", "legendFormat": "{{webhook_type}} {{operation}} {{kind}} (allowed={{allowed}})"}],
      "options": {"legend": {"displayMode": "list", "placement": "bottom"}, "tooltip": {"mode": "single"}}
    }
  ],
  "refresh": "30s", "schemaVersion": 37, "style": "dark", "tags": ["kruise", "openkruise"],
  "templating": {"list": []}, "time": {"from": "now-1h", "to": "now"}, "timepicker": {}, "timezone": "browser",
  "title": "OpenKruise Controller Health", "uid": "openkruise-health-compact", "version": 1
}


# For quick checks or debugging, query the metrics endpoint directly:

# Ensure port-forward is active as shown in "Enabling Metrics"

# Filter for OpenKruise specific metrics (verify 'kruise_' prefix and metric names)
curl -s localhost:8080/metrics | grep kruise_

# Filter for controller-runtime metrics related to Kruise controllers
# (Adjust regex if your controller names differ, e.g., "cloneset-controller")

curl -s localhost:8080/metrics | grep -E 'controller_runtime_.*(cloneset|statefulset|sidecarset|kruise)'

This helps view current metric values for targeted diagnostics.


