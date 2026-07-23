package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/OrlojHQ/orloj/resources"
)

const (
	chatInitialReconnectDelay = 500 * time.Millisecond
	chatMaxReconnectDelay     = 5 * time.Second
	chatMaxEventBytes         = 4 * 1024 * 1024
)

var errChatExit = errors.New("chat exit requested")

type chatOptions struct {
	Server             string
	Namespace          string
	System             string
	SessionName        string
	DecidedBy          string
	In                 io.Reader
	Out                io.Writer
	ErrOut             io.Writer
	Interactive        bool
	Client             *http.Client
	InitialReconnect   time.Duration
	MaxReconnect       time.Duration
	GenerateIdentifier func(string) string
}

type chatHTTPError struct {
	Operation string
	Status    int
	Body      string
}

func (e *chatHTTPError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("%s failed with HTTP %d", e.Operation, e.Status)
	}
	return fmt.Sprintf("%s failed with HTTP %d: %s", e.Operation, e.Status, body)
}

func (e *chatHTTPError) retryable() bool {
	return e.Status == http.StatusRequestTimeout ||
		e.Status == http.StatusTooManyRequests ||
		e.Status >= http.StatusInternalServerError
}

type chatTerminalError struct {
	cause error
}

func (e *chatTerminalError) Error() string {
	return e.cause.Error()
}

func (e *chatTerminalError) Unwrap() error {
	return e.cause
}

type chatInputResult struct {
	line string
	err  error
	eof  bool
}

type chatLineReader struct {
	results chan chatInputResult
	done    chan struct{}
	once    sync.Once
}

type chatTurnResponse struct {
	Turn    resources.SessionTurn `json:"turn"`
	Created bool                  `json:"created"`
}

type chatEventFrame struct {
	ID   string
	Type string
	Data string
}

type chatTurnRenderer struct {
	out             io.Writer
	errOut          io.Writer
	started         bool
	lineOpen        bool
	streamedContent strings.Builder
}

func newChatCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat <agent-system>",
		Short: "Chat interactively with an AgentSystem",
		Args:  cobra.ExactArgs(1),
		RunE:  runChatCommand,
	}
	cmd.Flags().String("session", "", "resume an existing Session by name")
	cmd.Flags().String("decided-by", "", "identity recorded for inline approval decisions")
	return cmd
}

func runChatCommand(cmd *cobra.Command, args []string) error {
	sessionName, _ := cmd.Flags().GetString("session")
	decidedBy, _ := cmd.Flags().GetString("decided-by")
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := runChat(ctx, chatOptions{
		Server:      resolveServer(cmd),
		Namespace:   resolveNamespace(cmd),
		System:      args[0],
		SessionName: sessionName,
		DecidedBy:   decidedBy,
		In:          os.Stdin,
		Out:         os.Stdout,
		ErrOut:      os.Stderr,
		Interactive: term.IsTerminal(int(os.Stdin.Fd())),
		Client:      http.DefaultClient,
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, errChatExit) {
		return nil
	}
	return err
}

func runChat(ctx context.Context, opts chatOptions) error {
	if err := normalizeChatOptions(&opts); err != nil {
		return err
	}

	session, created, err := openChatSession(ctx, opts)
	if err != nil {
		return err
	}
	opts.SessionName = session.Metadata.Name
	cursor := session.Status.LastEventSequence

	if created {
		fmt.Fprintf(opts.Out, "chat: created session/%s for agentsystem/%s\n", opts.SessionName, opts.System)
	} else {
		fmt.Fprintf(
			opts.Out,
			"chat: resumed session/%s for agentsystem/%s (%d completed turns)\n",
			opts.SessionName,
			opts.System,
			session.Status.CompletedTurns,
		)
	}
	writeChatResumeHint(opts)

	input := newChatLineReader(opts.In)
	defer input.Close()

	if session.Status.ActiveTurnID != "" &&
		(strings.EqualFold(session.Status.Phase, resources.SessionPhaseRunning) ||
			strings.EqualFold(session.Status.Phase, resources.SessionPhaseWaitingApproval)) {
		fmt.Fprintf(opts.Out, "chat: reattaching to active turn %s\n", session.Status.ActiveTurnID)
		// Replay the active turn so resumed output includes deltas emitted before
		// the client attached. Historical approval requests are display-only;
		// only the Session's current blocker can prompt during the replay.
		replayThrough := cursor
		replayApprovalName := ""
		if session.Status.BlockedOn != nil {
			replayApprovalName = strings.TrimSpace(session.Status.BlockedOn.Name)
		}
		cursor = 0
		if err := followChatTurn(
			ctx,
			opts,
			input,
			session.Status.ActiveTurnID,
			&cursor,
			replayThrough,
			replayApprovalName,
		); err != nil {
			return err
		}
		session, err = getChatSession(ctx, opts)
		if err != nil {
			return err
		}
		if resources.IsTerminalSessionPhase(session.Status.Phase) {
			fmt.Fprintf(opts.Out, "chat: session ended in phase %s\n", session.Status.Phase)
			return nil
		}
	}

	fmt.Fprintln(opts.Out, "chat: enter /help for commands")
	for {
		if err := ctx.Err(); err != nil {
			writeChatPreserved(opts)
			return err
		}
		fmt.Fprint(opts.Out, "you> ")
		line, ok, err := input.Next(ctx)
		if err != nil {
			writeChatPreserved(opts)
			return err
		}
		if !ok {
			fmt.Fprintln(opts.Out)
			writeChatPreserved(opts)
			return nil
		}

		message := strings.TrimSpace(line)
		if message == "" {
			continue
		}
		switch strings.ToLower(message) {
		case "/exit", "/quit":
			writeChatPreserved(opts)
			return nil
		case "/help":
			fmt.Fprintln(opts.Out, "chat: commands: /help, /session, /exit, /quit")
			continue
		case "/session":
			fmt.Fprintf(
				opts.Out,
				"chat: session=%s system=%s namespace=%s\n",
				opts.SessionName,
				opts.System,
				opts.Namespace,
			)
			continue
		}
		if strings.HasPrefix(message, "/") {
			fmt.Fprintf(opts.ErrOut, "error: unknown chat command %q; enter /help for commands\n", message)
			continue
		}

		turn, err := createChatTurn(ctx, opts, message)
		if err != nil {
			return err
		}
		if err := followChatTurn(ctx, opts, input, turn.ID, &cursor, 0, ""); err != nil {
			if errors.Is(err, errChatExit) {
				writeChatPreserved(opts)
			}
			return err
		}

		session, err = getChatSession(ctx, opts)
		if err != nil {
			return err
		}
		if resources.IsTerminalSessionPhase(session.Status.Phase) {
			fmt.Fprintf(opts.Out, "chat: session ended in phase %s\n", session.Status.Phase)
			return nil
		}
	}
}

func normalizeChatOptions(opts *chatOptions) error {
	if opts == nil {
		return errors.New("chat options are required")
	}
	opts.Server = strings.TrimRight(strings.TrimSpace(opts.Server), "/")
	opts.System = strings.TrimSpace(opts.System)
	opts.SessionName = strings.TrimSpace(opts.SessionName)
	opts.DecidedBy = strings.TrimSpace(opts.DecidedBy)
	opts.Namespace = strings.TrimSpace(opts.Namespace)
	if opts.Namespace == "" {
		opts.Namespace = "default"
	}
	if opts.Server == "" {
		return errors.New("chat server is required")
	}
	if opts.System == "" {
		return errors.New("agent-system name is required")
	}
	if opts.In == nil {
		opts.In = strings.NewReader("")
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.ErrOut == nil {
		opts.ErrOut = io.Discard
	}
	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}
	if opts.InitialReconnect <= 0 {
		opts.InitialReconnect = chatInitialReconnectDelay
	}
	if opts.MaxReconnect <= 0 {
		opts.MaxReconnect = chatMaxReconnectDelay
	}
	if opts.MaxReconnect < opts.InitialReconnect {
		opts.MaxReconnect = opts.InitialReconnect
	}
	if opts.GenerateIdentifier == nil {
		opts.GenerateIdentifier = randomChatIdentifier
	}
	return nil
}

func newChatLineReader(reader io.Reader) *chatLineReader {
	input := &chatLineReader{
		results: make(chan chatInputResult, 1),
		done:    make(chan struct{}),
	}
	go func() {
		defer close(input.results)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), chatMaxEventBytes)
		for scanner.Scan() {
			result := chatInputResult{line: scanner.Text()}
			select {
			case input.results <- result:
			case <-input.done:
				return
			}
		}
		result := chatInputResult{err: scanner.Err(), eof: true}
		select {
		case input.results <- result:
		case <-input.done:
		}
	}()
	return input
}

func (r *chatLineReader) Next(ctx context.Context) (string, bool, error) {
	select {
	case <-ctx.Done():
		return "", false, ctx.Err()
	case result, ok := <-r.results:
		if !ok {
			return "", false, nil
		}
		if result.err != nil {
			return "", false, result.err
		}
		if result.eof {
			return "", false, nil
		}
		return result.line, true, nil
	}
}

func (r *chatLineReader) Close() {
	r.once.Do(func() {
		close(r.done)
	})
}

func openChatSession(ctx context.Context, opts chatOptions) (resources.Session, bool, error) {
	if opts.SessionName != "" {
		session, err := getChatSession(ctx, opts)
		if err != nil {
			return resources.Session{}, false, err
		}
		if !strings.EqualFold(strings.TrimSpace(session.Spec.System), opts.System) {
			return resources.Session{}, false, fmt.Errorf(
				"session %q belongs to AgentSystem %q, not %q",
				opts.SessionName,
				session.Spec.System,
				opts.System,
			)
		}
		if resources.IsTerminalSessionPhase(session.Status.Phase) {
			return resources.Session{}, false, fmt.Errorf(
				"session %q is terminal (phase %s); start a new chat without --session",
				opts.SessionName,
				session.Status.Phase,
			)
		}
		if strings.EqualFold(session.Status.Phase, resources.SessionPhasePaused) {
			return resources.Session{}, false, fmt.Errorf(
				"session %q is paused; resume it through the Session API or Console before chatting",
				opts.SessionName,
			)
		}
		return session, false, nil
	}

	opts.SessionName = opts.GenerateIdentifier("chat-" + opts.System)
	session := resources.Session{
		APIVersion: "orloj.dev/v1",
		Kind:       "Session",
		Metadata: resources.ObjectMeta{
			Name:      opts.SessionName,
			Namespace: opts.Namespace,
		},
		Spec: resources.SessionSpec{
			System:   opts.System,
			MaxTurns: 0,
		},
	}
	var created resources.Session
	if err := doChatJSON(
		ctx,
		opts,
		http.MethodPost,
		chatAPIURL(opts, "/v1/sessions", nil),
		session,
		nil,
		&created,
		"create Session",
	); err != nil {
		return resources.Session{}, false, err
	}
	return created, true, nil
}

func getChatSession(ctx context.Context, opts chatOptions) (resources.Session, error) {
	var session resources.Session
	err := doChatJSON(
		ctx,
		opts,
		http.MethodGet,
		chatAPIURL(opts, "/v1/sessions/"+url.PathEscape(opts.SessionName), nil),
		nil,
		nil,
		&session,
		"get Session",
	)
	return session, err
}

func createChatTurn(ctx context.Context, opts chatOptions, content string) (resources.SessionTurn, error) {
	var result chatTurnResponse
	err := doChatJSON(
		ctx,
		opts,
		http.MethodPost,
		chatAPIURL(opts, "/v1/sessions/"+url.PathEscape(opts.SessionName)+"/turns", nil),
		map[string]any{"content": content},
		map[string]string{"Idempotency-Key": opts.GenerateIdentifier("turn")},
		&result,
		"create Session turn",
	)
	return result.Turn, err
}

func followChatTurn(
	ctx context.Context,
	opts chatOptions,
	input *chatLineReader,
	turnID string,
	cursor *uint64,
	replayThrough uint64,
	replayApprovalName string,
) error {
	renderer := &chatTurnRenderer{out: opts.Out, errOut: opts.ErrOut}
	delay := opts.InitialReconnect
	for {
		done, opened, err := followChatTurnOnce(
			ctx,
			opts,
			input,
			turnID,
			cursor,
			replayThrough,
			replayApprovalName,
			renderer,
		)
		if done {
			renderer.finishLine()
			return nil
		}
		if err == nil {
			err = io.EOF
		}
		if ctx.Err() != nil {
			renderer.finishLine()
			writeChatPreserved(opts)
			return ctx.Err()
		}
		var httpErr *chatHTTPError
		if errors.As(err, &httpErr) && !httpErr.retryable() {
			renderer.finishLine()
			return err
		}
		if errors.Is(err, errChatExit) {
			renderer.finishLine()
			return err
		}
		var terminalErr *chatTerminalError
		if errors.As(err, &terminalErr) {
			renderer.finishLine()
			return err
		}
		if opened {
			delay = opts.InitialReconnect
		}
		fmt.Fprintf(
			opts.ErrOut,
			"chat: stream disconnected (%s); reconnecting from event %d\n",
			terminalSafeText(err.Error()),
			*cursor,
		)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			renderer.finishLine()
			writeChatPreserved(opts)
			return ctx.Err()
		case <-timer.C:
		}
		delay *= 2
		if delay > opts.MaxReconnect {
			delay = opts.MaxReconnect
		}
	}
}

func followChatTurnOnce(
	ctx context.Context,
	opts chatOptions,
	input *chatLineReader,
	turnID string,
	cursor *uint64,
	replayThrough uint64,
	replayApprovalName string,
	renderer *chatTurnRenderer,
) (bool, bool, error) {
	query := url.Values{"after": []string{strconv.FormatUint(*cursor, 10)}}
	endpoint := chatAPIURL(
		opts,
		"/v1/sessions/"+url.PathEscape(opts.SessionName)+"/stream",
		query,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, false, fmt.Errorf("build Session stream request: %w", err)
	}
	resp, err := opts.Client.Do(req)
	if err != nil {
		return false, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, chatMaxEventBytes))
		return false, false, &chatHTTPError{
			Operation: "open Session stream",
			Status:    resp.StatusCode,
			Body:      string(body),
		}
	}

	done := false
	err = scanChatEventStream(resp.Body, func(frame chatEventFrame) error {
		var event resources.SessionEvent
		if err := json.Unmarshal([]byte(frame.Data), &event); err != nil {
			fmt.Fprintf(opts.ErrOut, "chat: ignored malformed Session event: %s\n", terminalSafeText(err.Error()))
			return nil
		}
		if event.Sequence == 0 && frame.ID != "" {
			event.Sequence, _ = strconv.ParseUint(frame.ID, 10, 64)
		}
		if event.Sequence > 0 {
			if event.Sequence <= *cursor {
				return nil
			}
			*cursor = event.Sequence
		}
		if event.TurnID != "" && event.TurnID != turnID {
			return nil
		}

		switch event.Type {
		case resources.SessionEventMessageDelta:
			renderer.delta(chatPayloadText(event.Payload, "delta"))
		case resources.SessionEventMessageReset:
			renderer.reset(chatPayloadString(event.Payload, "reason"))
		case resources.SessionEventMessageCompleted:
			renderer.complete(chatPayloadText(event.Payload, "content"))
		case resources.SessionEventToolStarted:
			renderer.status("tool", formatChatToolEvent(event, "started"))
		case resources.SessionEventToolCompleted:
			renderer.status("tool", formatChatToolEvent(event, "completed"))
		case resources.SessionEventApprovalRequested:
			approvalName := chatPayloadString(event.Payload, "name")
			if event.Sequence <= replayThrough &&
				(replayApprovalName == "" || approvalName != replayApprovalName) {
				return nil
			}
			renderer.finishLine()
			renderer.started = false
			if err := handleChatApproval(
				ctx,
				opts,
				input,
				chatPayloadString(event.Payload, "kind"),
				approvalName,
				chatPayloadString(event.Payload, "reason"),
			); err != nil {
				if errors.Is(err, errChatExit) || errors.Is(err, context.Canceled) {
					return err
				}
				return &chatTerminalError{cause: err}
			}
		case resources.SessionEventApprovalResolved:
			renderer.status("approval", "resolved")
		case resources.SessionEventTurnCompleted:
			done = true
			return errChatExit
		case resources.SessionEventTurnFailed:
			renderer.finishLine()
			message := chatPayloadString(event.Payload, "error")
			if message == "" {
				message = "Session turn failed"
			}
			return &chatTerminalError{cause: errors.New(message)}
		case resources.SessionEventTurnCancelled:
			renderer.finishLine()
			return &chatTerminalError{cause: errors.New("Session turn was cancelled")}
		case resources.SessionEventError:
			renderer.finishLine()
			message := chatPayloadString(event.Payload, "message")
			if message == "" {
				message = "Session stream reported an error"
			}
			return &chatTerminalError{cause: errors.New(message)}
		case resources.SessionEventSessionCancelled,
			resources.SessionEventSessionCompleted,
			resources.SessionEventSessionExpired:
			if !done {
				renderer.finishLine()
				return &chatTerminalError{
					cause: fmt.Errorf("Session ended while turn was active (%s)", event.Type),
				}
			}
		}
		return nil
	})
	if done && errors.Is(err, errChatExit) {
		return true, true, nil
	}
	return false, true, err
}

func scanChatEventStream(reader io.Reader, onFrame func(chatEventFrame) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), chatMaxEventBytes)
	frame := chatEventFrame{}
	data := make([]string, 0, 1)

	dispatch := func() error {
		if len(data) == 0 {
			frame = chatEventFrame{}
			return nil
		}
		frame.Data = strings.Join(data, "\n")
		if err := onFrame(frame); err != nil {
			return err
		}
		frame = chatEventFrame{}
		data = data[:0]
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if found && strings.HasPrefix(value, " ") {
			value = value[1:]
		}
		switch field {
		case "id":
			frame.ID = value
		case "event":
			frame.Type = value
		case "data":
			data = append(data, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := dispatch(); err != nil {
		return err
	}
	return io.EOF
}

func handleChatApproval(
	ctx context.Context,
	opts chatOptions,
	input *chatLineReader,
	kind string,
	name string,
	reason string,
) error {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("Session is waiting for an approval but did not identify the approval resource")
	}
	if !strings.EqualFold(kind, "ToolApproval") {
		resource := "task-approval"
		if !strings.EqualFold(kind, "TaskApproval") {
			resource = strings.ToLower(kind)
		}
		return fmt.Errorf(
			"session is waiting for %s/%s; resolve it with `orlojctl approve %s %s --namespace %s` or `orlojctl deny %s %s --namespace %s`",
			kind,
			name,
			resource,
			name,
			opts.Namespace,
			resource,
			name,
			opts.Namespace,
		)
	}

	approval, err := getChatToolApproval(ctx, opts, name)
	if err != nil {
		return err
	}
	phase := strings.TrimSpace(approval.Status.Phase)
	if phase != "" && !strings.EqualFold(phase, "Pending") {
		fmt.Fprintf(
			opts.Out,
			"approval: tool-approval/%s is already %s; waiting for Session to continue\n",
			terminalSafeText(name),
			terminalSafeText(phase),
		)
		return nil
	}
	if reason == "" {
		reason = approval.Spec.Reason
	}
	fmt.Fprintln(opts.Out)
	fmt.Fprintln(opts.Out, "approval: tool action requires review")
	fmt.Fprintf(opts.Out, "  name: %s\n", terminalSafeText(approval.Metadata.Name))
	fmt.Fprintf(opts.Out, "  tool: %s\n", terminalSafeText(approval.Spec.Tool))
	if approval.Spec.Agent != "" {
		fmt.Fprintf(opts.Out, "  agent: %s\n", terminalSafeText(approval.Spec.Agent))
	}
	if approval.Spec.OperationClass != "" {
		fmt.Fprintf(opts.Out, "  operation: %s\n", terminalSafeText(approval.Spec.OperationClass))
	}
	if reason != "" {
		fmt.Fprintf(opts.Out, "  reason: %s\n", terminalSafeText(reason))
	}
	if approval.Status.ExpiresAt != "" {
		fmt.Fprintf(opts.Out, "  expires: %s\n", terminalSafeText(approval.Status.ExpiresAt))
	}
	if approval.Spec.Input != "" {
		fmt.Fprintln(opts.Out, "  input:")
		for _, line := range strings.Split(formatChatApprovalInput(approval.Spec.Input), "\n") {
			fmt.Fprintf(opts.Out, "    %s\n", terminalSafeText(line))
		}
	}

	if !opts.Interactive {
		return fmt.Errorf(
			"tool approval %q requires an interactive decision; run `orlojctl approve tool-approval %s --namespace %s` or `orlojctl deny tool-approval %s --namespace %s`",
			name,
			name,
			opts.Namespace,
			name,
			opts.Namespace,
		)
	}

	action := ""
	for action == "" {
		fmt.Fprint(opts.Out, "approval> [a]pprove, [d]eny, or [q]uit: ")
		line, ok, err := input.Next(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return errChatExit
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "a", "approve":
			action = "approve"
		case "d", "deny":
			action = "deny"
		case "q", "quit", "abort":
			return errChatExit
		default:
			fmt.Fprintln(opts.ErrOut, "error: enter a, d, or q")
		}
	}

	fmt.Fprint(opts.Out, "comment> ")
	comment := ""
	line, ok, err := input.Next(ctx)
	if err != nil {
		return err
	}
	if ok {
		comment = strings.TrimSpace(line)
	}

	if err := decideChatToolApproval(ctx, opts, name, action, comment); err != nil {
		return err
	}
	decision := "approved"
	if action == "deny" {
		decision = "denied"
	}
	fmt.Fprintf(opts.Out, "approval: %s tool-approval/%s\n", decision, name)
	return nil
}

func getChatToolApproval(ctx context.Context, opts chatOptions, name string) (resources.ToolApproval, error) {
	var approval resources.ToolApproval
	err := doChatJSON(
		ctx,
		opts,
		http.MethodGet,
		chatAPIURL(opts, "/v1/tool-approvals/"+url.PathEscape(name), nil),
		nil,
		nil,
		&approval,
		"get ToolApproval",
	)
	return approval, err
}

func decideChatToolApproval(
	ctx context.Context,
	opts chatOptions,
	name string,
	action string,
	comment string,
) error {
	body := map[string]string{}
	if opts.DecidedBy != "" {
		body["decided_by"] = opts.DecidedBy
	}
	if comment != "" {
		body["comment"] = comment
	}
	return doChatJSON(
		ctx,
		opts,
		http.MethodPost,
		chatAPIURL(
			opts,
			"/v1/tool-approvals/"+url.PathEscape(name)+"/"+url.PathEscape(action),
			nil,
		),
		body,
		nil,
		nil,
		action+" ToolApproval",
	)
}

func doChatJSON(
	ctx context.Context,
	opts chatOptions,
	method string,
	endpoint string,
	body any,
	headers map[string]string,
	out any,
	operation string,
) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s: encode request: %w", operation, err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", operation, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := opts.Client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, chatMaxEventBytes))
	if err != nil {
		return fmt.Errorf("%s: read response: %w", operation, err)
	}
	if resp.StatusCode >= 300 {
		return &chatHTTPError{Operation: operation, Status: resp.StatusCode, Body: string(raw)}
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: decode response: %w", operation, err)
	}
	return nil
}

func chatAPIURL(opts chatOptions, path string, extra url.Values) string {
	values := url.Values{}
	for key, entries := range extra {
		for _, entry := range entries {
			values.Add(key, entry)
		}
	}
	values.Set("namespace", opts.Namespace)
	endpoint := opts.Server + path
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint
}

func randomChatIdentifier(prefix string) string {
	prefix = sanitizeChatName(prefix)
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(raw[:])
}

func sanitizeChatName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	lastDash := false
	for _, char := range value {
		valid := (char >= 'a' && char <= 'z') ||
			(char >= '0' && char <= '9') ||
			char == '.' ||
			char == '-'
		if valid {
			out.WriteRune(char)
			lastDash = char == '-'
			continue
		}
		if !lastDash {
			out.WriteByte('-')
			lastDash = true
		}
	}
	cleaned := strings.Trim(out.String(), "-.")
	if cleaned == "" {
		cleaned = "chat"
	}
	if len(cleaned) > 40 {
		cleaned = strings.TrimRight(cleaned[:40], "-.")
	}
	return cleaned
}

func (r *chatTurnRenderer) delta(content string) {
	content = terminalSafeText(content)
	if content == "" {
		return
	}
	r.ensureAssistantPrefix()
	fmt.Fprint(r.out, content)
	r.lineOpen = !strings.HasSuffix(content, "\n")
	r.streamedContent.WriteString(content)
}

func (r *chatTurnRenderer) complete(content string) {
	content = terminalSafeText(content)
	if content == "" {
		return
	}
	emitted := r.streamedContent.String()
	if emitted == "" {
		r.ensureAssistantPrefix()
		fmt.Fprint(r.out, content)
		r.lineOpen = !strings.HasSuffix(content, "\n")
		return
	}
	if strings.HasPrefix(content, emitted) {
		remaining := strings.TrimPrefix(content, emitted)
		if remaining != "" {
			r.ensureAssistantPrefix()
			fmt.Fprint(r.out, remaining)
			r.lineOpen = !strings.HasSuffix(remaining, "\n")
		}
		return
	}
	r.finishLine()
	fmt.Fprintf(r.out, "assistant (final)> %s", content)
	r.lineOpen = !strings.HasSuffix(content, "\n")
}

func (r *chatTurnRenderer) reset(reason string) {
	r.finishLine()
	if reason == "" {
		reason = "execution restarted"
	}
	fmt.Fprintf(r.out, "chat: tentative assistant output reset (%s)\n", terminalSafeText(reason))
	r.started = false
	r.streamedContent.Reset()
}

func (r *chatTurnRenderer) status(prefix string, message string) {
	r.finishLine()
	fmt.Fprintf(r.out, "%s: %s\n", terminalSafeText(prefix), terminalSafeText(message))
	r.started = false
}

func terminalSafeText(value string) string {
	var out strings.Builder
	for _, char := range value {
		switch char {
		case '\n', '\t':
			out.WriteRune(char)
		case '\r':
			out.WriteString(`\r`)
		case '\x1b':
			out.WriteString(`\x1b`)
		default:
			if char < 0x20 || (char >= 0x7f && char <= 0x9f) ||
				(char >= 0x202a && char <= 0x202e) ||
				(char >= 0x2066 && char <= 0x2069) {
				fmt.Fprintf(&out, `\u%04X`, char)
				continue
			}
			out.WriteRune(char)
		}
	}
	return out.String()
}

func (r *chatTurnRenderer) ensureAssistantPrefix() {
	if r.started {
		return
	}
	fmt.Fprint(r.out, "assistant> ")
	r.started = true
	r.lineOpen = true
}

func (r *chatTurnRenderer) finishLine() {
	if r.lineOpen {
		fmt.Fprintln(r.out)
	}
	r.lineOpen = false
}

func chatPayloadString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func chatPayloadText(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func formatChatToolEvent(event resources.SessionEvent, suffix string) string {
	tool := chatPayloadString(event.Payload, "tool")
	if tool == "" {
		tool = "tool"
	}
	return tool + " " + suffix
}

func formatChatApprovalInput(input string) string {
	var pretty bytes.Buffer
	if json.Indent(&pretty, []byte(input), "", "  ") == nil {
		return pretty.String()
	}
	return input
}

func writeChatResumeHint(opts chatOptions) {
	fmt.Fprintf(
		opts.Out,
		"chat: resume with `orlojctl chat %s --session %s --server %q --namespace %s`\n",
		opts.System,
		opts.SessionName,
		opts.Server,
		opts.Namespace,
	)
}

func writeChatPreserved(opts chatOptions) {
	fmt.Fprintf(opts.Out, "chat: session/%s preserved\n", opts.SessionName)
	writeChatResumeHint(opts)
}
