package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/OrlojHQ/orloj/resources"
)

const defaultDevDebounce = 400 * time.Millisecond

type devOptions struct {
	Server         string
	Namespace      string
	ManifestPath   string
	Run            bool
	TaskName       string
	TaskExplicit   bool
	Debounce       time.Duration
	FollowInterval time.Duration
	Out            io.Writer
	ErrOut         io.Writer
}

type runnableTaskManifest struct {
	Name string
	Path string
}

func newDevCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Watch manifests, apply changes, and follow a task",
		RunE:  runDev,
	}
	cmd.Flags().StringP("file", "f", "", "path to resource manifest file or directory")
	cmd.Flags().Bool("run", false, "rerun and follow a runnable Task after successful applies")
	cmd.Flags().String("task", "", "runnable Task manifest to use when more than one exists")
	cmd.Flags().Duration("debounce", defaultDevDebounce, "quiet period before applying filesystem changes")
	cmd.Flags().Duration("follow-interval", 2*time.Second, "task log polling interval")
	return cmd
}

func runDev(cmd *cobra.Command, args []string) error {
	manifestPath, _ := cmd.Flags().GetString("file")
	run, _ := cmd.Flags().GetBool("run")
	taskName, _ := cmd.Flags().GetString("task")
	debounce, _ := cmd.Flags().GetDuration("debounce")
	followInterval, _ := cmd.Flags().GetDuration("follow-interval")

	if strings.TrimSpace(manifestPath) == "" {
		return errors.New("-f is required")
	}
	if debounce <= 0 {
		return errors.New("--debounce must be > 0")
	}
	if followInterval <= 0 {
		return errors.New("--follow-interval must be > 0")
	}
	if strings.TrimSpace(taskName) != "" && !run {
		return errors.New("--task requires --run")
	}

	opts := devOptions{
		Server:         resolveServer(cmd),
		Namespace:      resolveNamespace(cmd),
		ManifestPath:   manifestPath,
		Run:            run,
		TaskName:       strings.TrimSpace(taskName),
		TaskExplicit:   strings.TrimSpace(taskName) != "",
		Debounce:       debounce,
		FollowInterval: followInterval,
		Out:            os.Stdout,
		ErrOut:         os.Stderr,
	}
	if opts.Run {
		selected, err := selectRunnableTask(opts.ManifestPath, opts.TaskName)
		if err != nil {
			return err
		}
		opts.TaskName = selected.Name
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runDevLoop(ctx, opts)
}

func runDevLoop(ctx context.Context, opts devOptions) error {
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.ErrOut == nil {
		opts.ErrOut = io.Discard
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create manifest watcher: %w", err)
	}
	defer watcher.Close()
	if err := addManifestWatches(watcher, opts.ManifestPath); err != nil {
		return err
	}

	lastHash, err := manifestContentHash(opts.ManifestPath)
	if err != nil {
		return err
	}

	var stopFollowing context.CancelFunc
	applyAndFollow := func() error {
		if opts.Run {
			requested := ""
			if opts.TaskExplicit {
				requested = opts.TaskName
			}
			selected, selectErr := selectRunnableTask(opts.ManifestPath, requested)
			if selectErr != nil {
				return selectErr
			}
			opts.TaskName = selected.Name
		}
		if err := validateDevManifests(opts.ManifestPath); err != nil {
			return err
		}
		result, applyErr := applyManifests(applyOptions{
			Server:              opts.Server,
			Namespace:           opts.Namespace,
			ManifestPath:        opts.ManifestPath,
			IncludeRunnable:     opts.Run,
			SkipRunnableAlways:  !opts.Run,
			SelectedRunnable:    opts.TaskName,
			IgnoreTaskConflicts: true,
			Out:                 opts.Out,
			Prefix:              "apply: ",
		})
		if applyErr != nil {
			return applyErr
		}
		if !opts.Run {
			return nil
		}
		followed := false
		for _, task := range result.AppliedTasks {
			if task.Runnable && strings.EqualFold(task.ManifestName, opts.TaskName) {
				if stopFollowing != nil {
					stopFollowing()
				}
				stopFollowing = startDevTaskFollower(ctx, opts, task.ActualName)
				followed = true
				break
			}
		}
		if !followed && result.TaskConflicts == 0 {
			return fmt.Errorf("selected runnable task %q was not applied", opts.TaskName)
		}
		return nil
	}

	if err := applyAndFollow(); err != nil {
		return err
	}
	fmt.Fprintf(opts.Out, "watch: watching %s (debounce %s)\n", opts.ManifestPath, opts.Debounce)

	var timer *time.Timer
	var timerC <-chan time.Time
	scheduleApply := func() {
		if timer == nil {
			timer = time.NewTimer(opts.Debounce)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(opts.Debounce)
		}
		timerC = timer.C
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
		if stopFollowing != nil {
			stopFollowing()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(opts.ErrOut, "error: manifest watcher: %v\n", watchErr)
			if errors.Is(watchErr, fsnotify.ErrEventOverflow) {
				if addErr := addManifestWatches(watcher, opts.ManifestPath); addErr != nil {
					fmt.Fprintf(opts.ErrOut, "error: restore manifest watches: %v\n", addErr)
				}
				scheduleApply()
			}
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			relevant := isRelevantManifestEvent(opts.ManifestPath, event)
			if event.Op.Has(fsnotify.Create) {
				if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
					if pathWithinRoot(opts.ManifestPath, event.Name) {
						if addErr := addManifestWatches(watcher, event.Name); addErr != nil {
							fmt.Fprintf(opts.ErrOut, "error: watch new directory %s: %v\n", event.Name, addErr)
						} else {
							relevant = true
						}
					}
				}
			}
			if relevant {
				scheduleApply()
			}
		case <-timerC:
			timerC = nil
			currentHash, hashErr := manifestContentHash(opts.ManifestPath)
			if hashErr != nil {
				fmt.Fprintf(opts.ErrOut, "error: scan manifests: %v\n", hashErr)
				continue
			}
			if currentHash == lastHash {
				continue
			}
			lastHash = currentHash
			if applyErr := applyAndFollow(); applyErr != nil {
				fmt.Fprintf(opts.ErrOut, "error: %v\n", applyErr)
			}
		}
	}
}

func validateDevManifests(manifestPath string) error {
	files, err := manifestPaths(manifestPath)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no manifest files found in %s", manifestPath)
	}
	var validationErrs []string
	for _, path := range files {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			validationErrs = append(validationErrs, fmt.Sprintf("%s: %v", path, readErr))
			continue
		}
		kind, detectErr := resources.DetectKind(raw)
		if detectErr != nil {
			validationErrs = append(validationErrs, fmt.Sprintf("%s: %v", path, detectErr))
			continue
		}
		if _, _, _, parseErr := resources.ParseManifest(kind, raw); parseErr != nil {
			validationErrs = append(validationErrs, fmt.Sprintf("%s: %v", path, parseErr))
		}
	}
	if len(validationErrs) == 0 {
		return nil
	}
	return fmt.Errorf("manifest validation failed: %s", strings.Join(validationErrs, "; "))
}

func selectRunnableTask(manifestPath, requested string) (runnableTaskManifest, error) {
	files, err := manifestPaths(manifestPath)
	if err != nil {
		return runnableTaskManifest{}, err
	}
	var tasks []runnableTaskManifest
	for _, path := range files {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return runnableTaskManifest{}, fmt.Errorf("read %s: %w", path, readErr)
		}
		kind, detectErr := resources.DetectKind(raw)
		if detectErr != nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(kind), "task") {
			continue
		}
		task, parseErr := resources.ParseTaskManifest(raw)
		if parseErr != nil {
			return runnableTaskManifest{}, fmt.Errorf("%s: %w", path, parseErr)
		}
		if strings.EqualFold(strings.TrimSpace(task.Spec.Mode), "template") {
			continue
		}
		tasks = append(tasks, runnableTaskManifest{Name: task.Metadata.Name, Path: path})
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Name < tasks[j].Name })

	requested = strings.TrimSpace(requested)
	if requested != "" {
		for _, task := range tasks {
			if strings.EqualFold(task.Name, requested) {
				return task, nil
			}
		}
		return runnableTaskManifest{}, fmt.Errorf("runnable task %q was not found under %s", requested, manifestPath)
	}
	switch len(tasks) {
	case 0:
		return runnableTaskManifest{}, fmt.Errorf("--run requires a runnable Task manifest under %s", manifestPath)
	case 1:
		return tasks[0], nil
	default:
		names := make([]string, 0, len(tasks))
		for _, task := range tasks {
			names = append(names, task.Name)
		}
		return runnableTaskManifest{}, fmt.Errorf("multiple runnable Task manifests found (%s); select one with --task", strings.Join(names, ", "))
	}
}

func addManifestWatches(watcher *fsnotify.Watcher, manifestPath string) error {
	info, err := os.Stat(manifestPath)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", manifestPath, err)
	}
	root := manifestPath
	if !info.IsDir() {
		root = filepath.Dir(manifestPath)
	} else if err := watcher.Add(filepath.Dir(filepath.Clean(root))); err != nil {
		return fmt.Errorf("watch parent of %s: %w", manifestPath, err)
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Name() == ".git" {
			return fs.SkipDir
		}
		return watcher.Add(path)
	})
	if err != nil {
		return fmt.Errorf("watch %s: %w", manifestPath, err)
	}
	return nil
}

func isRelevantManifestEvent(root string, event fsnotify.Event) bool {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	eventAbs, err := filepath.Abs(event.Name)
	if err != nil {
		return false
	}
	if filepath.Clean(rootAbs) == filepath.Clean(eventAbs) {
		return true
	}
	if !isManifestFile(event.Name) || isEditorTemporaryFile(event.Name) {
		return false
	}
	if info, statErr := os.Stat(rootAbs); statErr == nil && !info.IsDir() {
		return filepath.Clean(rootAbs) == filepath.Clean(eventAbs)
	}
	relative, err := filepath.Rel(rootAbs, eventAbs)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func pathWithinRoot(root, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(rootAbs, candidateAbs)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func isManifestFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

func isEditorTemporaryFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasPrefix(name, ".#") ||
		strings.HasSuffix(name, "~") ||
		strings.HasSuffix(name, ".swp") ||
		strings.HasSuffix(name, ".tmp")
}

func manifestContentHash(manifestPath string) (string, error) {
	files, err := manifestPaths(manifestPath)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, path := range files {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", fmt.Errorf("read %s: %w", path, readErr)
		}
		relative, relErr := filepath.Rel(manifestPath, path)
		if relErr != nil {
			relative = path
		}
		_, _ = io.WriteString(hash, filepath.ToSlash(relative))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(raw)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func startDevTaskFollower(parent context.Context, opts devOptions, taskName string) context.CancelFunc {
	ctx, cancel := context.WithCancel(parent)
	fmt.Fprintf(opts.Out, "task: following %s\n", taskName)

	_, endpoint, err := logsEndpointWithNamespace(opts.Server, "task/"+taskName, opts.Namespace)
	if err != nil {
		fmt.Fprintf(opts.ErrOut, "error: %v\n", err)
		cancel()
		return cancel
	}
	fetch := func(fetchCtx context.Context) ([]string, error) {
		return fetchLogs(fetchCtx, endpoint)
	}
	cursor := &logStreamCursor{}
	logOut := prefixWriter{out: opts.Out, prefix: "logs: "}
	logCtx, stopLogs := context.WithCancel(ctx)
	logDone := make(chan struct{})

	go func() {
		defer close(logDone)
		if err := streamLogsWithCursor(logCtx, logOut, fetch, opts.FollowInterval, cursor); err != nil && logCtx.Err() == nil {
			fmt.Fprintf(opts.ErrOut, "error: follow task logs: %v\n", err)
		}
	}()

	go func() {
		if err := followTaskPhases(ctx, opts, taskName); err != nil && ctx.Err() == nil {
			fmt.Fprintf(opts.ErrOut, "error: follow task phase: %v\n", err)
		}
		if ctx.Err() == nil {
			stopLogs()
			<-logDone
			finalCtx, finalCancel := context.WithTimeout(ctx, 2*time.Second)
			if logs, finalErr := fetch(finalCtx); finalErr == nil {
				cursor.printNew(logOut, logs)
			}
			finalCancel()
		}
		cancel()
	}()
	return cancel
}

func followTaskPhases(ctx context.Context, opts devOptions, taskName string) error {
	lastPhase := ""
	for {
		terminal, err := watchTaskPhasesOnce(ctx, opts, taskName, &lastPhase)
		if ctx.Err() != nil {
			return nil
		}
		if terminal {
			return nil
		}
		if err != nil {
			fmt.Fprintf(opts.ErrOut, "error: task phase stream disconnected: %v; reconnecting\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func watchTaskPhasesOnce(ctx context.Context, opts devOptions, taskName string, lastPhase *string) (bool, error) {
	endpoint := strings.TrimRight(opts.Server, "/") + "/v1/tasks/watch"
	query := url.Values{"name": []string{taskName}}
	if strings.TrimSpace(opts.Namespace) != "" {
		query.Set("namespace", strings.TrimSpace(opts.Namespace))
	}
	endpoint += "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("task watch failed: %s", strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var event struct {
			Resource resources.Task `json:"resource"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		phase := strings.TrimSpace(event.Resource.Status.Phase)
		if phase != "" && !strings.EqualFold(phase, *lastPhase) {
			*lastPhase = phase
			fmt.Fprintf(opts.Out, "task: %s phase=%s\n", taskName, phase)
		}
		switch strings.ToLower(phase) {
		case "succeeded", "failed", "deadletter", "cancelled":
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, io.EOF
}

type prefixWriter struct {
	out    io.Writer
	prefix string
}

func (w prefixWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if _, err := fmt.Fprint(w.out, w.prefix); err != nil {
		return 0, err
	}
	if _, err := w.out.Write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}
