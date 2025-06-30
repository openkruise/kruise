# Operations Guide

This section provides comprehensive guidance for operators and platform engineers on monitoring, maintaining, and troubleshooting OpenKruise in production environments. A well-operated OpenKruise deployment ensures robust and efficient management of advanced workloads on Kubernetes.

## Contents

- **[Prometheus Metrics](./prometheus-metrics.md):**
  Learn about the metrics exposed by OpenKruise. This guide covers how to enable metrics, key metric categories, setting up Prometheus scraping and alerting, and example Grafana dashboards for effective monitoring of OpenKruise controllers and workloads.

- **[Troubleshooting Guide](./troubleshooting.md):**
  A practical guide to diagnosing and resolving common issues encountered with OpenKruise. It covers general troubleshooting steps, specific problems related to the controller manager, webhooks, and various Kruise workloads (CloneSet, AdvancedStatefulSet, SidecarSet), along with useful diagnostic commands.

## Introduction to Operating OpenKruise

Operating OpenKruise effectively involves:

1.  **Monitoring:** Continuously observing the health and performance of OpenKruise controllers and the workloads they manage using metrics. Setting up alerts for abnormal conditions is crucial.
2.  **.[Troubleshooting](./operations/troubleshooting.md)** Systematically diagnosing issues when they arise, using logs, events, metrics, and resource statuses.
3.  **Maintenance:** Understanding upgrade procedures, managing configurations, and being aware of how OpenKruise interacts with the broader Kubernetes ecosystem (e.g., API server, networking, storage).

While OpenKruise is designed for resilience, this guide aims to equip you with the knowledge to handle operational aspects confidently and maintain a stable and performant OpenKruise installation.