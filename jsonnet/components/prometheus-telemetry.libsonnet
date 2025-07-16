local withDescription = (import '../utils/add-annotations.libsonnet').withDescription;
local prometheus = import 'github.com/prometheus-operator/kube-prometheus/jsonnet/kube-prometheus/components/prometheus.libsonnet';
local generateCertInjection = import '../utils/generate-certificate-injection.libsonnet';

function(params) {
  local cfg = params,
  local prometheusTLSSecret = 'prometheus-telemetry-tls',
  local saName = "prometheus-telemetry",

  telemetryScrapeSecret: {
    apiVersion: 'v1',
    kind: 'Secret',
    metadata: {
      name: 'telemetry-scrape',
      namespace: cfg.namespace,
      labels: { 'app.kubernetes.io/name': 'prometheus-telemetry' },
    },
    type: 'Opaque',
    data: {},
  },
  serviceAccount: {
    apiVersion: 'v1',
    kind: 'ServiceAccount',
    automountServiceAccountToken: false,
    metadata: {
      labels: {
        'app.kubernetes.io/name': 'prometheus-telemetry',
        'app.kubernetes.io/component': 'prometheus-telemetry',
      } + cfg.commonLabels,
      name: saName,
      namespace: cfg.namespace,
    },
  },
  clusterRole: {
    apiVersion: 'rbac.authorization.k8s.io/v1',
    kind: 'ClusterRole',
    metadata: {
      labels: {
        'app.kubernetes.io/name': 'prometheus-telemetry',
        'app.kubernetes.io/component': 'prometheus-telemetry',
        'app.kubernetes.io/instance': 'prometheus-telemetry',
      } + cfg.commonLabels,
      name: 'system:prometheus-telemetry',
    },
    rules: [
      {
        apiGroups: ['security.openshift.io'],
	resourceNames: ['nonroot'],
        resources: ['securitycontextconstraints'],
        verbs: ['use'],
      },
      {
        nonResourceURLs: ['/federate'],
        verbs: ['get'],
      },
      {
        // By default authenticated service accounts are assigned to the `restricted` SCC which implies MustRunAsRange.
        // This is problematic with statefulsets as UIDs (and file permissions) can change if SCCs are elevated.
        // Instead, this sets the `nonroot` SCC in conjunction with a static fsGroup and runAsUser security context below
        // to be immune against UID changes.
        apiGroups: ['security.openshift.io'],
        resources: ['securitycontextconstraints'],
        resourceNames: ['nonroot'],
        verbs: ['use'],
      },
    ],
  },
  clusterRoleBinding: {
    apiVersion: 'rbac.authorization.k8s.io/v1',
    kind: 'ClusterRoleBinding',
    metadata: {
      labels: {
        'app.kubernetes.io/name': 'prometheus-telemetry',
      } + cfg.commonLabels,
      name: 'system:prometheus-telemetry',
    },
    roleRef: {
      apiGroup: 'rbac.authorization.k8s.io',
      kind: 'ClusterRole',
      name: 'system:prometheus-telemetry'
    },
    subjects: [
      {
        kind: 'ServiceAccount',
        name: 'prometheus-telemetry',
        namespace: cfg.namespace,
      },
    ],
  },
  servingCertsCaBundle+: generateCertInjection.SCOCaBundleCM(cfg.namespace, 'serving-certs-ca-bundle'),
  trustedCaBundle: generateCertInjection.trustedCNOCaBundleCM(cfg.namespace, 'prometheus-telemetry-trusted-ca-bundle'),
  prometheus: {
    apiVersion: 'monitoring.coreos.com/v1',
    kind: 'Prometheus',
    metadata: {
      name: 'telemetry',
      namespace: cfg.namespace,
      labels: {
        'app.kubernetes.io/name': 'prometheus-telemetry',
        'app.kubernetes.io/component': 'prometheus-telemetry',
      } + cfg.commonLabels,
      annotations: {
        'operator.prometheus.io/controller-id': 'openshift-monitoring/prometheus-operator',
      },
    },
    spec: {
      replicas: 1,
      serviceAccountName: saName,
      // Enable experimental delayed compaction feature.
      enableFeatures: ['delayed-compaction'],
      resources: {
        requests: {
          memory: '1Gi',
          cpu: '70m',
        },
      },
      web: {
        httpConfig: {
          headers: {
            contentSecurityPolicy: "frame-ancestors 'none'",
          },
        },
      },
      podMetadata: {
        annotations: {
          'openshift.io/required-scc': 'nonroot',
        },
      },
      securityContext: {
        fsGroup: 65534,
        runAsNonRoot: true,
        runAsUser: 65534,
      },
      serviceMonitorSelector: {
        matchLabels:
          {'app.kubernetes.io/instance': 'telemetry'},
      },
      additionalScrapeConfigs: {
        name: 'telemetry-scrape',
        key: 'telemetry-scrape.yaml',
      },
      scrapeConfigSelector: null,
      scrapeConfigNamespaceSelector: null,
      listenLocal: true,
      priorityClassName: 'system-cluster-critical',
      affinity: {
        podAntiAffinity: {
          requiredDuringSchedulingIgnoredDuringExecution: [
            {
              labelSelector: {
                matchLabels: {
                  'app.kubernetes.io/component': 'prometheus',
                  'app.kubernetes.io/instance': 'telemetry',
                  'app.kubernetes.io/name': 'prometheus',
                  'app.kubernetes.io/part-of': 'openshift-monitoring',
                },
              },
              namespaces: ['openshift-monitoring'],
              topologyKey: 'kubernetes.io/hostname',
            },
          ],
        },
      },
      additionalArgs: [
        // This aligns any scrape timestamps <= 15ms to the a multiple of
        // the scrape interval. This optmizes tsdb compression.
        // 15ms was chosen for being a conservative value given our default
        // scrape interval of 30s. Even for half the default value we only
        // move scrape interval timestamps by <= .1% of their absolute
        // length.
        {
          name: 'scrape.timestamp-tolerance',
          value: '15ms',
        },
      ],
      // Increase the startup probe timeout to 1h from 15m to avoid restart
      // failures when the WAL replay takes a long time.
      // See https://issues.redhat.com/browse/OCPBUGS-4168 for details.
      maximumStartupDurationSeconds: 3600,
      containers: [
        {
          name: 'prometheus',
          env: [{
            name: 'HTTP_PROXY',
            value: '',
          }, {
            name: 'HTTPS_PROXY',
            value: '',
          }, {
            name: 'NO_PROXY',
            value: '',
          }],
          volumeMounts: [
            {
              name: $.trustedCaBundle.metadata.name,
              mountPath: '/etc/pki/ca-trust/extracted/pem/',
            },
            // {
            //   mountPath: '/etc/tls/private',
            //   name: 'secret-' + prometheusTLSSecret,
            // },
            {
              mountPath: '/etc/tls/client',
              name: 'configmap-metrics-client-ca',
              readOnly: true,
            },
          ],
        },
      ],
      scrapeClasses: [
      ],
      configMaps: ['serving-certs-ca-bundle', 'kubelet-serving-ca-bundle', 'metrics-client-ca'],
      secrets: [
        'metrics-client-certs',
      ],
      volumes: [
        {
          name: $.trustedCaBundle.metadata.name,
          configMap: {
            name: $.trustedCaBundle.metadata.name,
            items: [{
              key: 'ca-bundle.crt',
              path: 'tls-ca-bundle.pem',
            }],
          },
        },
      ],
    },
  },
  }
