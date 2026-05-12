package cli

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/spf13/cobra"
)

func newEvalCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Agent evaluation commands",
	}
	cmd.AddCommand(
		newEvalRunCommand(),
		newEvalStartCommand(),
		newEvalListCommand(),
		newEvalGetCommand(),
		newEvalCompareCommand(),
		newEvalDatasetsCommand(),
		newEvalExportCommand(),
		newEvalAnnotateCommand(),
		newEvalImportCommand(),
		newEvalFinalizeCommand(),
	)
	return cmd
}

func newEvalRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Create and start an eval run",
		RunE:  runEvalRun,
	}
	cmd.Flags().String("dataset", "", "name of the EvalDataset to use (required)")
	cmd.Flags().String("system", "", "name of the AgentSystem to evaluate (required)")
	cmd.Flags().String("model", "", "optional model endpoint override")
	cmd.Flags().String("scoring", "", "scoring strategy: exact_match, llm_judge, manual, custom")
	cmd.Flags().String("judge-model", "", "model for llm_judge scoring")
	cmd.Flags().Int("concurrency", 5, "max parallel eval tasks")
	cmd.Flags().Bool("wait", false, "block until the run completes")
	cmd.Flags().StringP("output", "o", "table", "output format: table|json")
	_ = cmd.MarkFlagRequired("dataset")
	_ = cmd.MarkFlagRequired("system")
	return cmd
}

func runEvalRun(cmd *cobra.Command, _ []string) error {
	server := resolveServer(cmd)
	ns := resolveNamespace(cmd)

	dataset, _ := cmd.Flags().GetString("dataset")
	system, _ := cmd.Flags().GetString("system")
	model, _ := cmd.Flags().GetString("model")
	scoring, _ := cmd.Flags().GetString("scoring")
	judgeModel, _ := cmd.Flags().GetString("judge-model")
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	wait, _ := cmd.Flags().GetBool("wait")
	output, _ := cmd.Flags().GetString("output")

	runName := fmt.Sprintf("eval-%s-%s-%d", system, dataset, os.Getpid())

	run := resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata: resources.ObjectMeta{
			Name:      runName,
			Namespace: ns,
		},
		Spec: resources.EvalRunSpec{
			DatasetRef:            dataset,
			System:                system,
			ModelEndpointOverride: model,
			Concurrency:           concurrency,
		},
	}
	if scoring != "" {
		run.Spec.Scoring.Strategy = scoring
	}
	if judgeModel != "" {
		run.Spec.Scoring.ModelRef = judgeModel
	}

	body, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("failed to encode eval run: %w", err)
	}

	reqURL := strings.TrimRight(server, "/") + "/v1/eval-runs?run=true"
	resp, err := http.Post(reqURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create eval run: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create eval run: %s", bytes.TrimSpace(respBody))
	}

	var created resources.EvalRun
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("eval run %s created (phase: %s)\n", created.Metadata.Name, created.Status.Phase)

	if wait {
		fmt.Println("waiting for eval run to complete...")
		return waitAndPrintEvalRun(server, created.Metadata.Name, ns, output)
	}
	return nil
}

func newEvalStartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start a suspended eval run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			ns := resolveNamespace(cmd)
			wait, _ := cmd.Flags().GetBool("wait")
			output, _ := cmd.Flags().GetString("output")

			reqURL := strings.TrimRight(server, "/") + "/v1/eval-runs/" + url.PathEscape(args[0]) + "/start"
			if ns != "" {
				reqURL += "?namespace=" + url.QueryEscape(ns)
			}
			resp, err := http.Post(reqURL, "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to start eval run: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("start failed: %s", bytes.TrimSpace(body))
			}

			var started resources.EvalRun
			if err := json.NewDecoder(resp.Body).Decode(&started); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}

			fmt.Printf("eval run %s started (phase: %s)\n", started.Metadata.Name, started.Status.Phase)

			if wait {
				fmt.Println("waiting for eval run to complete...")
				return waitAndPrintEvalRun(server, started.Metadata.Name, ns, output)
			}
			return nil
		},
	}
	cmd.Flags().Bool("wait", false, "block until the run completes")
	cmd.Flags().StringP("output", "o", "table", "output format: table|json")
	return cmd
}

func waitAndPrintEvalRun(server, name, ns, format string) error {
	reqURL := strings.TrimRight(server, "/") + "/v1/eval-runs/" + url.PathEscape(name)
	if ns != "" {
		reqURL += "?namespace=" + url.QueryEscape(ns)
	}
	for {
		resp, err := http.Get(reqURL)
		if err != nil {
			return err
		}
		var run resources.EvalRun
		if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()

		switch run.Status.Phase {
		case resources.EvalRunPhaseSucceeded, resources.EvalRunPhaseFailed,
			resources.EvalRunPhaseCancelled, resources.EvalRunPhasePendingReview:
			return printEvalRunResult(run, format)
		}
	}
}

func printEvalRunResult(run resources.EvalRun, format string) error {
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(run)
	}

	fmt.Printf("\nEval Run: %s\n", run.Metadata.Name)
	fmt.Printf("Phase:    %s\n", run.Status.Phase)
	fmt.Printf("Samples:  %d/%d completed\n", run.Status.CompletedSamples, run.Status.TotalSamples)
	fmt.Printf("Pass Rate: %.1f%%\n", run.Status.Summary.PassRate*100)
	fmt.Printf("Mean Score: %.3f\n\n", run.Status.Summary.MeanScore)

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "SAMPLE\tSCORE\tPASS\tERROR")
	for _, r := range run.Status.Results {
		score := "-"
		if r.Score != nil {
			score = fmt.Sprintf("%.3f", *r.Score)
		}
		pass := "-"
		if r.Pass != nil {
			if *r.Pass {
				pass = "true"
			} else {
				pass = "false"
			}
		}
		errStr := r.Error
		if len(errStr) > 50 {
			errStr = errStr[:50] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.SampleName, score, pass, errStr)
	}
	return tw.Flush()
}

func newEvalListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List eval runs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			server := resolveServer(cmd)
			ns := resolveNamespace(cmd)
			reqURL := strings.TrimRight(server, "/") + "/v1/eval-runs"
			if ns != "" {
				reqURL += "?namespace=" + url.QueryEscape(ns)
			}
			resp, err := http.Get(reqURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("list failed: %s", bytes.TrimSpace(body))
			}
			var list resources.EvalRunList
			if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
				return err
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tDATASET\tSYSTEM\tPHASE\tPASS_RATE\tSAMPLES")
			for _, item := range list.Items {
				passRate := "-"
				if item.Status.Phase == resources.EvalRunPhaseSucceeded {
					passRate = fmt.Sprintf("%.0f%%", item.Status.Summary.PassRate*100)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d/%d\n",
					item.Metadata.Name, item.Spec.DatasetRef, item.Spec.System,
					item.Status.Phase, passRate,
					item.Status.CompletedSamples, item.Status.TotalSamples)
			}
			return tw.Flush()
		},
	}
}

func newEvalGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show detailed results for an eval run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			ns := resolveNamespace(cmd)
			output, _ := cmd.Flags().GetString("output")
			reqURL := strings.TrimRight(server, "/") + "/v1/eval-runs/" + url.PathEscape(args[0])
			if ns != "" {
				reqURL += "?namespace=" + url.QueryEscape(ns)
			}
			resp, err := http.Get(reqURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("get failed: %s", bytes.TrimSpace(body))
			}
			var run resources.EvalRun
			if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
				return err
			}
			return printEvalRunResult(run, output)
		},
	}
	cmd.Flags().StringP("output", "o", "table", "output format: table|json")
	return cmd
}

func newEvalCompareCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compare <run1> <run2> [runN...]",
		Short: "Compare eval runs side-by-side",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			output, _ := cmd.Flags().GetString("output")
			runsParam := strings.Join(args, ",")
			reqURL := strings.TrimRight(server, "/") + "/v1/eval-runs/compare?runs=" + url.QueryEscape(runsParam)
			resp, err := http.Get(reqURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("compare failed: %s", bytes.TrimSpace(body))
			}
			body, _ := io.ReadAll(resp.Body)
			if output == "json" {
				fmt.Println(string(body))
				return nil
			}

			var result struct {
				Runs    []string `json:"runs"`
				Summary map[string]struct {
					PassRate    float64 `json:"pass_rate"`
					MeanScore   float64 `json:"mean_score"`
					TotalTokens int     `json:"total_tokens"`
				} `json:"summary"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return err
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			header := "METRIC"
			for _, rn := range result.Runs {
				header += "\t" + rn
			}
			fmt.Fprintln(tw, header)

			passRow := "Pass Rate"
			scoreRow := "Mean Score"
			tokenRow := "Tokens"
			for _, rn := range result.Runs {
				s := result.Summary[rn]
				passRow += fmt.Sprintf("\t%.1f%%", s.PassRate*100)
				scoreRow += fmt.Sprintf("\t%.3f", s.MeanScore)
				tokenRow += fmt.Sprintf("\t%d", s.TotalTokens)
			}
			fmt.Fprintln(tw, passRow)
			fmt.Fprintln(tw, scoreRow)
			fmt.Fprintln(tw, tokenRow)
			return tw.Flush()
		},
	}
	cmd.Flags().StringP("output", "o", "table", "output format: table|json")
	return cmd
}

func newEvalDatasetsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "datasets",
		Short: "List eval datasets",
		RunE: func(cmd *cobra.Command, _ []string) error {
			server := resolveServer(cmd)
			ns := resolveNamespace(cmd)
			reqURL := strings.TrimRight(server, "/") + "/v1/eval-datasets"
			if ns != "" {
				reqURL += "?namespace=" + url.QueryEscape(ns)
			}
			resp, err := http.Get(reqURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("list failed: %s", bytes.TrimSpace(body))
			}
			var list resources.EvalDatasetList
			if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
				return err
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tSAMPLES\tSTATUS")
			for _, item := range list.Items {
				fmt.Fprintf(tw, "%s\t%d\t%s\n", item.Metadata.Name, len(item.Spec.Samples), item.Status.Phase)
			}
			return tw.Flush()
		},
	}
}

func newEvalExportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <name>",
		Short: "Export eval run results for human review",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			format, _ := cmd.Flags().GetString("format")
			reqURL := strings.TrimRight(server, "/") + "/v1/eval-runs/" + url.PathEscape(args[0]) + "/export?format=" + url.QueryEscape(format)
			resp, err := http.Get(reqURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("export failed: %s", bytes.TrimSpace(body))
			}
			_, err = io.Copy(os.Stdout, resp.Body)
			return err
		},
	}
	cmd.Flags().String("format", "csv", "export format: csv|json")
	return cmd
}

func newEvalAnnotateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "annotate <run-name>",
		Short: "Annotate a single sample result",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			sampleName, _ := cmd.Flags().GetString("sample")
			scoreStr, _ := cmd.Flags().GetString("score")
			passFlag, _ := cmd.Flags().GetBool("pass")
			failFlag, _ := cmd.Flags().GetBool("fail")
			comment, _ := cmd.Flags().GetString("comment")

			annotation := make(map[string]any)
			if scoreStr != "" {
				score, err := strconv.ParseFloat(scoreStr, 64)
				if err != nil {
					return fmt.Errorf("invalid score: %w", err)
				}
				annotation["score"] = score
			}
			if passFlag {
				annotation["pass"] = true
			} else if failFlag {
				pass := false
				annotation["pass"] = pass
			}
			if comment != "" {
				annotation["comment"] = comment
			}

			body, _ := json.Marshal(annotation)
			reqURL := strings.TrimRight(server, "/") + "/v1/eval-runs/" + url.PathEscape(args[0]) + "/results/" + url.PathEscape(sampleName)
			req, err := http.NewRequest(http.MethodPut, reqURL, bytes.NewReader(body))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				respBody, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("annotate failed: %s", bytes.TrimSpace(respBody))
			}
			fmt.Printf("annotated sample %q in eval run %s\n", sampleName, args[0])
			return nil
		},
	}
	cmd.Flags().String("sample", "", "sample name to annotate (required)")
	cmd.Flags().String("score", "", "human-assigned score (0.0-1.0)")
	cmd.Flags().Bool("pass", false, "mark sample as passed")
	cmd.Flags().Bool("fail", false, "mark sample as failed")
	cmd.Flags().String("comment", "", "reviewer notes")
	_ = cmd.MarkFlagRequired("sample")
	return cmd
}

func newEvalImportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <run-name>",
		Short: "Bulk-import annotations from a reviewed file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			filePath, _ := cmd.Flags().GetString("file")
			if filePath == "" {
				return fmt.Errorf("-f/--file is required")
			}

			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			var annotations []map[string]any
			if json.Valid(data) {
				if err := json.Unmarshal(data, &annotations); err != nil {
					return fmt.Errorf("failed to parse JSON file: %w", err)
				}
			} else {
				annotations, err = parseCSVAnnotations(data)
				if err != nil {
					return fmt.Errorf("failed to parse CSV file: %w", err)
				}
			}

			body, _ := json.Marshal(annotations)
			reqURL := strings.TrimRight(server, "/") + "/v1/eval-runs/" + url.PathEscape(args[0]) + "/results"
			resp, err := http.Post(reqURL, "application/json", bytes.NewReader(body))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				respBody, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("import failed: %s", bytes.TrimSpace(respBody))
			}
			fmt.Printf("imported %d annotations into eval run %s\n", len(annotations), args[0])
			return nil
		},
	}
	cmd.Flags().StringP("file", "f", "", "CSV or JSON file with annotations")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func parseCSVAnnotations(data []byte) ([]map[string]any, error) {
	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	header := records[0]
	colIndex := make(map[string]int, len(header))
	for i, h := range header {
		colIndex[strings.ToLower(strings.TrimSpace(h))] = i
	}

	var annotations []map[string]any
	for _, row := range records[1:] {
		ann := map[string]any{}
		if idx, ok := colIndex["sample_name"]; ok && idx < len(row) {
			ann["sample_name"] = strings.TrimSpace(row[idx])
		}
		if idx, ok := colIndex["score"]; ok && idx < len(row) {
			if s := strings.TrimSpace(row[idx]); s != "" {
				score, err := strconv.ParseFloat(s, 64)
				if err == nil {
					ann["score"] = score
				}
			}
		}
		if idx, ok := colIndex["pass"]; ok && idx < len(row) {
			if s := strings.TrimSpace(row[idx]); s != "" {
				pass, err := strconv.ParseBool(s)
				if err == nil {
					ann["pass"] = pass
				}
			}
		}
		if idx, ok := colIndex["comment"]; ok && idx < len(row) {
			ann["comment"] = strings.TrimSpace(row[idx])
		}
		if _, ok := ann["sample_name"]; ok {
			annotations = append(annotations, ann)
		}
	}
	return annotations, nil
}

func newEvalFinalizeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "finalize <run-name>",
		Short: "Transition a PendingReview eval run to Succeeded",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			reqURL := strings.TrimRight(server, "/") + "/v1/eval-runs/" + url.PathEscape(args[0]) + "/finalize"
			resp, err := http.Post(reqURL, "application/json", nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("finalize failed: %s", bytes.TrimSpace(body))
			}
			fmt.Printf("eval run %s finalized\n", args[0])
			return nil
		},
	}
}
