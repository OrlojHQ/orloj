package agentruntime

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// KubernetesToolConfig holds operator-level defaults for the K8s tool runtime.
type KubernetesToolConfig struct {
	Namespace      string
	ServiceAccount string
	DefaultImage   string
	JobTTLSeconds  int32
}

func (c KubernetesToolConfig) normalized() KubernetesToolConfig {
	out := c
	if strings.TrimSpace(out.Namespace) == "" {
		out.Namespace = "default"
	}
	if strings.TrimSpace(out.DefaultImage) == "" {
		out.DefaultImage = "curlimages/curl:8.8.0"
	}
	if out.JobTTLSeconds <= 0 {
		out.JobTTLSeconds = 300
	}
	return out
}

// KubernetesJobClient abstracts the Kubernetes API calls for testing.
type KubernetesJobClient interface {
	CreateJob(ctx context.Context, namespace string, job *batchv1.Job) (*batchv1.Job, error)
	WatchJob(ctx context.Context, namespace, name string) (watch.Interface, error)
	GetJob(ctx context.Context, namespace, name string) (*batchv1.Job, error)
	GetPodLogs(ctx context.Context, namespace, podName string) (string, error)
	ListPods(ctx context.Context, namespace, jobName string) ([]corev1.Pod, error)
	DeleteJob(ctx context.Context, namespace, name string) error
}

type defaultKubernetesJobClient struct {
	clientset kubernetes.Interface
}

// NewDefaultKubernetesJobClient wraps a kubernetes.Interface as a KubernetesJobClient.
func NewDefaultKubernetesJobClient(clientset kubernetes.Interface) KubernetesJobClient {
	return &defaultKubernetesJobClient{clientset: clientset}
}

func (c *defaultKubernetesJobClient) CreateJob(ctx context.Context, namespace string, job *batchv1.Job) (*batchv1.Job, error) {
	return c.clientset.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
}

func (c *defaultKubernetesJobClient) WatchJob(ctx context.Context, namespace, name string) (watch.Interface, error) {
	return c.clientset.BatchV1().Jobs(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + name,
	})
}

func (c *defaultKubernetesJobClient) GetJob(ctx context.Context, namespace, name string) (*batchv1.Job, error) {
	return c.clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *defaultKubernetesJobClient) GetPodLogs(ctx context.Context, namespace, podName string) (string, error) {
	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, int64(DefaultMaxToolOutputBytes)))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *defaultKubernetesJobClient) ListPods(ctx context.Context, namespace, jobName string) ([]corev1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *defaultKubernetesJobClient) DeleteJob(ctx context.Context, namespace, name string) error {
	propagation := metav1.DeletePropagationBackground
	return c.clientset.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
}

// KubernetesToolRuntime executes tools as ephemeral Kubernetes Jobs.
type KubernetesToolRuntime struct {
	registry     ToolCapabilityRegistry
	secrets      SecretResolver
	authInjector *AuthInjector
	client       KubernetesJobClient
	config       KubernetesToolConfig
	namespace    string
}

func NewKubernetesToolRuntime(
	clientset kubernetes.Interface,
	config KubernetesToolConfig,
	secrets SecretResolver,
) *KubernetesToolRuntime {
	var client KubernetesJobClient
	if clientset != nil {
		client = &defaultKubernetesJobClient{clientset: clientset}
	}
	return NewKubernetesToolRuntimeWithClient(client, config, secrets)
}

func NewKubernetesToolRuntimeWithClient(
	client KubernetesJobClient,
	config KubernetesToolConfig,
	secrets SecretResolver,
) *KubernetesToolRuntime {
	if secrets == nil {
		secrets = NewEnvSecretResolver("ORLOJ_SECRET_")
	}
	return &KubernetesToolRuntime{
		client:       client,
		config:       config.normalized(),
		secrets:      secrets,
		authInjector: NewAuthInjector(secrets, nil),
	}
}

func (r *KubernetesToolRuntime) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	if r == nil {
		return NewKubernetesToolRuntimeWithClient(nil, KubernetesToolConfig{}, nil)
	}
	return &KubernetesToolRuntime{
		registry:     registry,
		secrets:      r.secrets,
		authInjector: r.authInjector,
		client:       r.client,
		config:       r.config,
		namespace:    r.namespace,
	}
}

func (r *KubernetesToolRuntime) WithNamespace(namespace string) ToolRuntime {
	if r == nil {
		return NewKubernetesToolRuntimeWithClient(nil, KubernetesToolConfig{}, nil)
	}
	copy := *r
	copy.namespace = resources.NormalizeNamespace(strings.TrimSpace(namespace))
	if aware, ok := copy.secrets.(namespaceAwareSecretResolver); ok {
		copy.secrets = aware.WithNamespace(copy.namespace)
	}
	copy.authInjector = NewAuthInjector(copy.secrets, nil)
	return &copy
}

func (r *KubernetesToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return "", NewToolError(
			ToolStatusError, ToolCodeInvalidInput, ToolReasonInvalidInput, false,
			"missing tool name", ErrInvalidToolRuntimePolicy, map[string]string{"field": "tool"},
		)
	}
	if r.client == nil {
		return "", NewToolError(
			ToolStatusError, ToolCodeIsolationUnavailable, ToolReasonIsolationUnavailable, false,
			fmt.Sprintf("kubernetes runtime unavailable for tool=%s: no client configured", tool),
			ErrToolIsolationUnavailable, map[string]string{"tool": tool, "isolation_mode": "kubernetes"},
		)
	}
	if r.registry == nil {
		return "", NewToolError(
			ToolStatusError, ToolCodeRuntimePolicyInvalid, ToolReasonRuntimePolicyInvalid, false,
			"missing tool registry for kubernetes runtime",
			ErrInvalidToolRuntimePolicy, map[string]string{"tool": tool},
		)
	}
	spec, ok := r.registry.Resolve(tool)
	if !ok {
		return "", NewToolError(
			ToolStatusError, ToolCodeUnsupportedTool, ToolReasonToolUnsupported, false,
			fmt.Sprintf("unsupported tool %s", tool),
			ErrUnsupportedTool, map[string]string{"tool": tool},
		)
	}

	switch strings.ToLower(strings.TrimSpace(spec.Type)) {
	case "", "http":
		return r.callHTTP(ctx, tool, spec, input)
	case "cli":
		return r.callCLI(ctx, tool, spec, input)
	default:
		return "", NewToolError(
			ToolStatusError, ToolCodeUnsupportedTool, ToolReasonToolUnsupported, false,
			fmt.Sprintf("tool=%s type=%s unsupported by kubernetes isolation path", tool, spec.Type),
			ErrUnsupportedTool, map[string]string{"tool": tool, "type": strings.TrimSpace(spec.Type)},
		)
	}
}

func (r *KubernetesToolRuntime) callHTTP(ctx context.Context, tool string, spec resources.ToolSpec, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", mapKubernetesContextError(tool, err)
	}
	endpoint := strings.TrimSpace(spec.Endpoint)
	if endpoint == "" {
		return "", NewToolError(
			ToolStatusError, ToolCodeRuntimePolicyInvalid, ToolReasonRuntimePolicyInvalid, false,
			fmt.Sprintf("tool=%s missing endpoint", tool),
			ErrInvalidToolRuntimePolicy, map[string]string{"tool": tool},
		)
	}

	image := strings.TrimSpace(r.config.DefaultImage)
	shell := "/bin/sh"

	env := []corev1.EnvVar{{Name: "TOOL_ENDPOINT", Value: endpoint}}

	if r.authInjector != nil {
		authResult, authErr := r.authInjector.Resolve(ctx, tool, spec.Auth)
		if authErr != nil {
			return "", authErr
		}
		for k, v := range authResult.EnvVars {
			env = append(env, corev1.EnvVar{Name: k, Value: v})
		}
	}

	script := `if [ -n "$TOOL_AUTH_BEARER" ]; then HEADER="Authorization: Bearer $TOOL_AUTH_BEARER"; cat | curl -sS --fail-with-body -X POST -H "$HEADER" --data-binary @- "$TOOL_ENDPOINT"; elif [ -n "$TOOL_AUTH_BASIC" ]; then HEADER="Authorization: Basic $TOOL_AUTH_BASIC"; cat | curl -sS --fail-with-body -X POST -H "$HEADER" --data-binary @- "$TOOL_ENDPOINT"; elif [ -n "$TOOL_AUTH_HEADER_NAME" ]; then HEADER="$TOOL_AUTH_HEADER_NAME: $TOOL_AUTH_HEADER_VALUE"; cat | curl -sS --fail-with-body -X POST -H "$HEADER" --data-binary @- "$TOOL_ENDPOINT"; else cat | curl -sS --fail-with-body -X POST --data-binary @- "$TOOL_ENDPOINT"; fi`

	job := r.buildJob(tool, image, []string{shell, "-lc", script}, env, input, spec)
	return r.runJob(ctx, tool, job)
}

func (r *KubernetesToolRuntime) callCLI(ctx context.Context, tool string, spec resources.ToolSpec, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", mapKubernetesContextError(tool, err)
	}
	command := strings.TrimSpace(spec.Cli.Command)
	if command == "" {
		return "", NewToolError(
			ToolStatusError, ToolCodeRuntimePolicyInvalid, ToolReasonRuntimePolicyInvalid, false,
			fmt.Sprintf("tool=%s missing cli.command", tool),
			ErrInvalidToolRuntimePolicy, map[string]string{"tool": tool},
		)
	}
	image := strings.TrimSpace(spec.Cli.Image)
	if image == "" {
		return "", NewToolError(
			ToolStatusError, ToolCodeRuntimePolicyInvalid, ToolReasonRuntimePolicyInvalid, false,
			fmt.Sprintf("tool=%s missing cli.image for kubernetes isolation", tool),
			ErrInvalidToolRuntimePolicy, map[string]string{"tool": tool},
		)
	}

	args, err := evaluateCLIArgs(spec.Cli.Args, input)
	if err != nil {
		return "", err
	}

	var envVars []corev1.EnvVar
	for k, v := range spec.Cli.Env {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}
	for _, ref := range spec.Cli.EnvFrom {
		secretRef := ref.SecretRef
		if ref.Key != "" {
			secretRef = secretRef + ":" + ref.Key
		}
		value, resolveErr := r.secrets.Resolve(ctx, secretRef)
		if resolveErr != nil {
			return "", NewToolError(
				ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, false,
				fmt.Sprintf("failed to resolve secret for env var %s: %v", ref.Name, resolveErr),
				fmt.Errorf("%w: %v", ErrToolSecretResolution, resolveErr),
				map[string]string{"env_var": ref.Name, "secretRef": ref.SecretRef},
			)
		}
		envVars = append(envVars, corev1.EnvVar{Name: ref.Name, Value: value})
	}

	cmdArgs := append([]string{command}, args...)

	stdinData := ""
	if spec.Cli.StdinFromInput {
		stdinData = strings.TrimSpace(input)
	}

	job := r.buildJob(tool, image, cmdArgs, envVars, stdinData, spec)
	stdout, runErr := r.runJob(ctx, tool, job)
	if runErr != nil {
		return "", runErr
	}
	return stdout, nil
}

func (r *KubernetesToolRuntime) buildJob(
	tool, image string,
	command []string,
	env []corev1.EnvVar,
	stdinData string,
	spec resources.ToolSpec,
) *batchv1.Job {
	cfg := r.config
	ns := strings.TrimSpace(cfg.Namespace)
	if r.namespace != "" {
		ns = r.namespace
	}
	if ns == "" {
		ns = "default"
	}

	jobName := sanitizeK8sName("orloj-tool-" + tool + "-" + fmt.Sprintf("%d", time.Now().UnixNano()))

	var backoffLimit int32
	ttl := cfg.JobTTLSeconds

	resourceReqs := corev1.ResourceRequirements{}
	if mem := strings.TrimSpace(spec.Cli.Resources.Memory); mem != "" {
		if resourceReqs.Limits == nil {
			resourceReqs.Limits = corev1.ResourceList{}
		}
		if resourceReqs.Requests == nil {
			resourceReqs.Requests = corev1.ResourceList{}
		}
		q := resource.MustParse(mem)
		resourceReqs.Limits[corev1.ResourceMemory] = q
		resourceReqs.Requests[corev1.ResourceMemory] = q
	}
	if cpus := strings.TrimSpace(spec.Cli.Resources.CPUs); cpus != "" {
		if resourceReqs.Limits == nil {
			resourceReqs.Limits = corev1.ResourceList{}
		}
		if resourceReqs.Requests == nil {
			resourceReqs.Requests = corev1.ResourceList{}
		}
		q := resource.MustParse(cpus)
		resourceReqs.Limits[corev1.ResourceCPU] = q
		resourceReqs.Requests[corev1.ResourceCPU] = q
	}

	var activeDeadline *int64
	if timeout := strings.TrimSpace(spec.Runtime.Timeout); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil && d > 0 {
			secs := int64(d.Seconds())
			if secs < 1 {
				secs = 1
			}
			activeDeadline = &secs
		}
	}

	container := corev1.Container{
		Name:      "tool",
		Image:     image,
		Command:   command,
		Env:       env,
		Resources: resourceReqs,
		Stdin:     stdinData != "",
	}

	if stdinData != "" {
		container.Command = wrapWithStdin(command, stdinData)
		container.Stdin = false
	}

	labels := map[string]string{
		"orloj.dev/component": "tool-job",
		"orloj.dev/tool":      sanitizeK8sLabelValue(tool),
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			ActiveDeadlineSeconds:   activeDeadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{container},
				},
			},
		},
	}

	if sa := strings.TrimSpace(cfg.ServiceAccount); sa != "" {
		job.Spec.Template.Spec.ServiceAccountName = sa
	}

	return job
}

func (r *KubernetesToolRuntime) runJob(ctx context.Context, tool string, job *batchv1.Job) (string, error) {
	ns := job.Namespace

	created, err := r.client.CreateJob(ctx, ns, job)
	if err != nil {
		return "", NewToolError(
			ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, true,
			fmt.Sprintf("kubernetes job creation failed for tool=%s: %v", tool, err),
			err, map[string]string{"tool": tool, "isolation_mode": "kubernetes"},
		)
	}
	jobName := created.Name

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = r.client.DeleteJob(cleanupCtx, ns, jobName)
	}()

	if err := r.waitForJobCompletion(ctx, tool, ns, jobName); err != nil {
		return "", err
	}

	pods, err := r.client.ListPods(ctx, ns, jobName)
	if err != nil || len(pods) == 0 {
		return "", NewToolError(
			ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, true,
			fmt.Sprintf("kubernetes job pod not found for tool=%s job=%s", tool, jobName),
			err, map[string]string{"tool": tool, "job": jobName, "isolation_mode": "kubernetes"},
		)
	}

	stdout, err := r.client.GetPodLogs(ctx, ns, pods[0].Name)
	if err != nil {
		return "", NewToolError(
			ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, true,
			fmt.Sprintf("kubernetes job log retrieval failed for tool=%s pod=%s: %v", tool, pods[0].Name, err),
			err, map[string]string{"tool": tool, "pod": pods[0].Name, "isolation_mode": "kubernetes"},
		)
	}

	return strings.TrimSpace(stdout), nil
}

func (r *KubernetesToolRuntime) waitForJobCompletion(ctx context.Context, tool, namespace, jobName string) error {
	watcher, err := r.client.WatchJob(ctx, namespace, jobName)
	if err != nil {
		return NewToolError(
			ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, true,
			fmt.Sprintf("kubernetes job watch failed for tool=%s job=%s: %v", tool, jobName, err),
			err, map[string]string{"tool": tool, "job": jobName, "isolation_mode": "kubernetes"},
		)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return mapKubernetesContextError(tool, ctx.Err())
		case event, ok := <-watcher.ResultChan():
			if !ok {
				job, getErr := r.client.GetJob(ctx, namespace, jobName)
				if getErr != nil {
					return NewToolError(
						ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, true,
						fmt.Sprintf("kubernetes job watch closed and get failed for tool=%s job=%s", tool, jobName),
						getErr, map[string]string{"tool": tool, "job": jobName, "isolation_mode": "kubernetes"},
					)
				}
				return checkJobStatus(tool, jobName, job)
			}
			if event.Type == watch.Deleted {
				return NewToolError(
					ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, false,
					fmt.Sprintf("kubernetes job deleted unexpectedly for tool=%s job=%s", tool, jobName),
					nil, map[string]string{"tool": tool, "job": jobName, "isolation_mode": "kubernetes"},
				)
			}
			job, ok := event.Object.(*batchv1.Job)
			if !ok {
				continue
			}
			if result := checkJobStatus(tool, jobName, job); result != errJobStillRunning {
				return result
			}
		}
	}
}

var errJobStillRunning = fmt.Errorf("job still running")

func checkJobStatus(tool, jobName string, job *batchv1.Job) error {
	for _, cond := range job.Status.Conditions {
		switch cond.Type {
		case batchv1.JobComplete:
			if cond.Status == corev1.ConditionTrue {
				return nil
			}
		case batchv1.JobFailed:
			if cond.Status == corev1.ConditionTrue {
				return NewToolError(
					ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, true,
					fmt.Sprintf("kubernetes job failed for tool=%s job=%s reason=%s", tool, jobName, cond.Reason),
					nil, map[string]string{"tool": tool, "job": jobName, "reason": cond.Reason, "isolation_mode": "kubernetes"},
				)
			}
		}
	}
	return errJobStillRunning
}

func mapKubernetesContextError(tool string, err error) error {
	tool = strings.TrimSpace(tool)
	switch {
	case ctx_is_deadline(err):
		return NewToolError(
			ToolStatusError, ToolCodeTimeout, ToolReasonExecutionTimeout, true,
			fmt.Sprintf("kubernetes tool execution timed out for tool=%s", tool),
			err, map[string]string{"tool": tool, "isolation_mode": "kubernetes"},
		)
	case ctx_is_canceled(err):
		return NewToolError(
			ToolStatusError, ToolCodeCanceled, ToolReasonExecutionCanceled, false,
			fmt.Sprintf("kubernetes tool execution canceled for tool=%s", tool),
			err, map[string]string{"tool": tool, "isolation_mode": "kubernetes"},
		)
	default:
		return err
	}
}

func ctx_is_deadline(err error) bool {
	return err == context.DeadlineExceeded
}

func ctx_is_canceled(err error) bool {
	return err == context.Canceled
}

// wrapWithStdin builds a command that echoes stdin data into the real command via a shell heredoc.
func wrapWithStdin(command []string, stdinData string) []string {
	escaped := strings.ReplaceAll(stdinData, "'", "'\"'\"'")
	inner := strings.Join(command, " ")
	script := fmt.Sprintf("printf '%%s' '%s' | %s", escaped, inner)
	return []string{"/bin/sh", "-c", script}
}

func sanitizeK8sName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	b.Grow(len(name))
	for i, ch := range name {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch == '-' || ch == '_' || ch == '.':
			if i > 0 {
				b.WriteByte('-')
			}
		default:
			if i > 0 {
				b.WriteByte('-')
			}
		}
	}
	result := b.String()
	result = strings.TrimRight(result, "-")
	if len(result) > 63 {
		result = result[:63]
		result = strings.TrimRight(result, "-")
	}
	return result
}

func sanitizeK8sLabelValue(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, ch := range s {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9', ch == '-', ch == '_', ch == '.':
			b.WriteRune(ch)
		default:
			b.WriteByte('-')
		}
	}
	result := b.String()
	if len(result) > 63 {
		result = result[:63]
	}
	result = strings.Trim(result, "-_.")
	return result
}
