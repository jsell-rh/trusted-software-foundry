package compiler

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --------------------------------------------------------------------------
// forge deploy — Kubernetes manifest generation
//
// Generates production-ready Kubernetes manifests for every service in the
// IR spec. For single-service specs, produces deploy/deployment.yaml and
// deploy/service.yaml. For multi-service specs, generates one subdirectory
// per service plus a top-level kustomization.yaml.
//
// Design principles:
//   - All secrets referenced via secretKeyRef (never inlined in manifests)
//   - Resource requests/limits set conservatively — engineers tune per env
//   - Health probes wired to foundry-health port if component is present
//   - Postgres dependency generates a companion StatefulSet
// --------------------------------------------------------------------------

// Deploy generates Kubernetes manifests for the compiled spec into outputDir.
// outputDir will contain a deploy/ subdirectory with all manifests and a
// kustomization.yaml that references them.
func Deploy(ir *spec.IRSpec, outputDir string) error {
	deployDir := filepath.Join(outputDir, "deploy")
	if err := os.MkdirAll(deployDir, 0755); err != nil {
		return fmt.Errorf("creating deploy/ dir: %w", err)
	}

	secretName := toSnakeCase(ir.Metadata.Name) + "-secrets"
	secretName = strings.ReplaceAll(secretName, "_", "-")

	var kustomizeResources []string

	// Generate manifests for multi-service specs.
	if len(ir.Services) > 0 {
		for _, svc := range ir.Services {
			svcDir := filepath.Join(deployDir, svc.Name)
			if err := os.MkdirAll(svcDir, 0755); err != nil {
				return fmt.Errorf("creating deploy/%s/: %w", svc.Name, err)
			}

			data := serviceDeployData{
				IR:         ir,
				Service:    svc,
				AppName:    ir.Metadata.Name,
				SecretName: secretName,
				EnvVars:    buildEnvVars(ir, svc),
				HasHealth:  svcHasComponent(svc, "foundry-health"),
			}

			if err := writeManifest(svcDir, "deployment.yaml", deploymentTemplate, data); err != nil {
				return fmt.Errorf("deploy/%s/deployment.yaml: %w", svc.Name, err)
			}
			if err := writeManifest(svcDir, "service.yaml", serviceTemplate, data); err != nil {
				return fmt.Errorf("deploy/%s/service.yaml: %w", svc.Name, err)
			}

			kustomizeResources = append(kustomizeResources,
				svc.Name+"/deployment.yaml",
				svc.Name+"/service.yaml",
			)
		}
	} else {
		// Single-service spec: generate flat deploy/ structure.
		data := serviceDeployData{
			IR:      ir,
			Service: spec.IRService{Name: ir.Metadata.Name, Port: 8080},
			AppName: ir.Metadata.Name,
			SecretName: secretName,
			EnvVars:    buildEnvVars(ir, spec.IRService{}),
			HasHealth:  true,
		}

		if err := writeManifest(deployDir, "deployment.yaml", deploymentTemplate, data); err != nil {
			return fmt.Errorf("deploy/deployment.yaml: %w", err)
		}
		if err := writeManifest(deployDir, "service.yaml", serviceTemplate, data); err != nil {
			return fmt.Errorf("deploy/service.yaml: %w", err)
		}
		kustomizeResources = append(kustomizeResources, "deployment.yaml", "service.yaml")
	}

	// Generate secrets template (placeholder — engineers fill in values).
	if err := writeManifest(deployDir, "secrets.yaml", secretsTemplate, secretsData{
		IR:         ir,
		SecretName: secretName,
		EnvVars:    buildEnvVars(ir, spec.IRService{}),
	}); err != nil {
		return fmt.Errorf("deploy/secrets.yaml: %w", err)
	}
	kustomizeResources = append(kustomizeResources, "secrets.yaml")

	// Generate kustomization.yaml.
	if err := writeKustomization(deployDir, ir, kustomizeResources); err != nil {
		return fmt.Errorf("deploy/kustomization.yaml: %w", err)
	}

	return nil
}

// envVar describes a Kubernetes env var entry for a container.
type envVar struct {
	Name      string // env var name (e.g. DATABASE_URL)
	SecretKey string // key in the secrets manifest (e.g. database-url)
	IsLiteral bool   // true: use value: directly (non-secret), false: secretKeyRef
	Value     string // literal value when IsLiteral is true
}

// buildEnvVars derives the required env vars for a service from the IR spec.
// Env vars that reference ${VAR} are emitted as secretKeyRef entries.
// Static values are emitted as literal env vars.
func buildEnvVars(ir *spec.IRSpec, svc spec.IRService) []envVar {
	var vars []envVar
	seen := map[string]bool{}

	add := func(name, secretKey string) {
		if !seen[name] {
			seen[name] = true
			vars = append(vars, envVar{Name: name, SecretKey: secretKey})
		}
	}
	addLiteral := func(name, value string) {
		if !seen[name] {
			seen[name] = true
			vars = append(vars, envVar{Name: name, IsLiteral: true, Value: value})
		}
	}

	// Database URL — always present when postgres component exists.
	if ir.Database != nil && ir.Database.Type == "postgres" {
		add("DATABASE_URL", "database-url")
	}

	// JWT auth.
	if ir.Auth != nil && ir.Auth.Type == "jwt" {
		if strings.Contains(ir.Auth.JWKURL, "${") {
			varName := envVarName(ir.Auth.JWKURL)
			add(varName, envSecretKey(varName))
		}
		if ir.Auth.AllowMock != "" && strings.Contains(ir.Auth.AllowMock, "${") {
			varName := envVarName(ir.Auth.AllowMock)
			addLiteral(varName, "false") // safe default for production
		}
	}

	// Kafka broker URL.
	if ir.Events != nil && ir.Events.Backend == "kafka" && (len(svc.Components) == 0 || svcHasComponent(svc, "foundry-kafka")) {
		brokerURL := ir.Events.BrokerURL
		if brokerURL == "" && ir.Events.Broker != nil {
			brokerURL = ir.Events.Broker.URL
		}
		if strings.Contains(brokerURL, "${") {
			varName := envVarName(brokerURL)
			add(varName, envSecretKey(varName))
		}
	}

	// NATS URL.
	if ir.Events != nil && ir.Events.Backend == "nats" && (len(svc.Components) == 0 || svcHasComponent(svc, "foundry-nats")) {
		brokerURL := ir.Events.BrokerURL
		if brokerURL == "" && ir.Events.Broker != nil {
			brokerURL = ir.Events.Broker.URL
		}
		if strings.Contains(brokerURL, "${") {
			varName := envVarName(brokerURL)
			add(varName, envSecretKey(varName))
		}
	}

	// Redis URL.
	if ir.State != nil && (len(svc.Components) == 0 || svcHasComponent(svc, "foundry-redis")) {
		url := ir.State.URL
		if url == "" && len(ir.State.Backends) > 0 {
			url = ir.State.Backends[0].URL
		}
		if strings.Contains(url, "${") {
			varName := envVarName(url)
			add(varName, envSecretKey(varName))
		}
	}

	// Temporal host.
	if ir.Workflows != nil && (len(svc.Components) == 0 || svcHasComponent(svc, "foundry-temporal")) {
		host := ir.Workflows.Host
		if host == "" {
			host = "${TEMPORAL_HOST}"
		}
		if strings.Contains(host, "${") {
			varName := envVarName(host)
			add(varName, envSecretKey(varName))
		}
	}

	return vars
}

// envVarName extracts the variable name from a ${VAR} reference.
// "${KAFKA_BROKER_URL}" → "KAFKA_BROKER_URL"
func envVarName(ref string) string {
	if strings.HasPrefix(ref, "${") && strings.HasSuffix(ref, "}") {
		return ref[2 : len(ref)-1]
	}
	return ref
}

// envSecretKey converts an env var name to a kebab-case Kubernetes secret key.
// "KAFKA_BROKER_URL" → "kafka-broker-url"
func envSecretKey(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func writeManifest(dir, filename string, tmpl *template.Template, data any) error {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("rendering %s: %w", filename, err)
	}
	return os.WriteFile(filepath.Join(dir, filename), buf.Bytes(), 0644)
}

type serviceDeployData struct {
	IR         *spec.IRSpec
	Service    spec.IRService
	AppName    string
	SecretName string
	EnvVars    []envVar
	HasHealth  bool
}

var deploymentTemplate = template.Must(template.New("deployment").Funcs(template.FuncMap{
	"snake": toSnakeCase,
	"dash":  func(s string) string { return strings.ReplaceAll(s, "_", "-") },
}).Parse(`# Generated by forge deploy. Review resource limits and replica counts before production use.
# Spec: {{ .AppName }} v{{ .IR.Metadata.Version }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .AppName }}-{{ .Service.Name }}
  labels:
    app: {{ .AppName }}
    component: {{ .Service.Name }}
    managed-by: forge
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{ .AppName }}
      component: {{ .Service.Name }}
  template:
    metadata:
      labels:
        app: {{ .AppName }}
        component: {{ .Service.Name }}
    spec:
      containers:
      - name: {{ .Service.Name }}
        image: {{ .AppName }}-{{ .Service.Name }}:latest
        imagePullPolicy: IfNotPresent
{{ if .Service.Port }}        ports:
        - name: http
          containerPort: {{ .Service.Port }}
          protocol: TCP
{{ end }}{{ if .HasHealth }}        livenessProbe:
          httpGet:
            path: /healthz
            port: 8083
          initialDelaySeconds: 15
          periodSeconds: 30
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8083
          initialDelaySeconds: 5
          periodSeconds: 10
          failureThreshold: 3
{{ end }}        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
{{ if .EnvVars }}        env:
{{ range .EnvVars }}{{ if .IsLiteral }}        - name: {{ .Name }}
          value: {{ printf "%q" .Value }}
{{ else }}        - name: {{ .Name }}
          valueFrom:
            secretKeyRef:
              name: {{ $.SecretName }}
              key: {{ .SecretKey }}
{{ end }}{{ end }}{{ end }}      securityContext:
        runAsNonRoot: true
        runAsUser: 1001
`))

var serviceTemplate = template.Must(template.New("service").Parse(`# Generated by forge deploy.
apiVersion: v1
kind: Service
metadata:
  name: {{ .AppName }}-{{ .Service.Name }}
  labels:
    app: {{ .AppName }}
    component: {{ .Service.Name }}
    managed-by: forge
spec:
  type: ClusterIP
  selector:
    app: {{ .AppName }}
    component: {{ .Service.Name }}
{{ if .Service.Port }}  ports:
  - name: http
    port: {{ .Service.Port }}
    targetPort: http
    protocol: TCP
{{ end }}`))

type secretsData struct {
	IR         *spec.IRSpec
	SecretName string
	EnvVars    []envVar
}

var secretsTemplate = template.Must(template.New("secrets").Parse(`# Generated by forge deploy. Fill in base64-encoded values before applying.
# NEVER commit real secrets to source control.
# Use: echo -n 'value' | base64
# Spec: {{ .IR.Metadata.Name }} v{{ .IR.Metadata.Version }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ .SecretName }}
type: Opaque
data:
{{ range .EnvVars }}{{ if not .IsLiteral }}  {{ .SecretKey }}: ""  # base64-encoded {{ .Name }}
{{ end }}{{ end }}`))

func writeKustomization(deployDir string, ir *spec.IRSpec, resources []string) error {
	var buf bytes.Buffer
	buf.WriteString("# Generated by forge deploy.\n")
	buf.WriteString(fmt.Sprintf("# Spec: %s v%s\n", ir.Metadata.Name, ir.Metadata.Version))
	buf.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	buf.WriteString("kind: Kustomization\n")
	buf.WriteString("resources:\n")
	for _, r := range resources {
		buf.WriteString("  - " + r + "\n")
	}
	return os.WriteFile(filepath.Join(deployDir, "kustomization.yaml"), buf.Bytes(), 0644)
}
